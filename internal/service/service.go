package service

import (
	"errors"
	"fmt"
	"github.com/samber/lo"
	"log"
	"orchdio/blueprint"
	platforminternal "orchdio/internal/platform"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"os"
	"sort"
	"sync"
)

type Service struct {
	factory *platforminternal.PlatformServiceFactory
}

func NewServiceFactory(factory *platforminternal.PlatformServiceFactory) *Service {
	return &Service{
		factory: factory,
	}
}

type trackJob struct {
	track  *blueprint.PlatformSearchTrack
	result *blueprint.TrackSearchResult
	err    error
	index  int
}

func (pc *Service) ConvertTrack(info *blueprint.LinkInfo) (*blueprint.TrackConversion, error) {
	srcPlatformService, sErr := pc.factory.GetPlatformService(info.Platform)
	if sErr != nil {
		log.Println(sErr)
		return nil, sErr
	}

	srcTrackResult, stErr := srcPlatformService.SearchTrackWithID(info)
	if stErr != nil {
		log.Println(stErr)
		return nil, stErr
	}

	trackConversion := &blueprint.TrackConversion{
		Entity: "track",
	}

	uErr := pc.updatePlatformTracks(info.Platform, trackConversion, srcTrackResult)
	if uErr != nil {
		log.Println(uErr)
		return nil, uErr
	}

	searchData := &blueprint.TrackSearchData{
		Title:   srcTrackResult.Title,
		Artists: srcTrackResult.Artists,
	}

	if info.TargetPlatform == "all" {
		// get all targetPlatforms services apart from the current "from"
		validPlatforms := []string{applemusic.IDENTIFIER, deezer.IDENTIFIER,
			spotify.IDENTIFIER, tidal.IDENTIFIER, ytmusic.IDENTIFIER}

		targetPlats := lo.Filter(validPlatforms, func(s string, i int) bool {
			return validPlatforms[i] != info.Platform
		})

		allTargetPlatformServiceFactories, pErr := pc.factory.GetPlatformServices(targetPlats)
		if pErr != nil {
			log.Println(pErr)
			return nil, pErr
		}

		var toResult []blueprint.TrackSearchResult
		// todo: run this concurrently.
		for i := range targetPlats {
			instance := allTargetPlatformServiceFactories[i]
			platformSearchResult, spErr := instance.SearchTrackWithTitle(searchData)
			if spErr != nil {
				// note: for some reason, the platform could not convert the track.
				// in the final result, this will be nil and the platform would simply be
				// omitted in the response
				continue
			}

			toResult = append(toResult, *platformSearchResult)
			upErr := pc.updatePlatformTracks(targetPlats[i], trackConversion, platformSearchResult)
			if upErr != nil {
				log.Println(upErr)
				return nil, upErr
			}
		}

		return trackConversion, nil
	}

	// now do for non-all target platform
	targetPlatformService, tErr := pc.factory.GetPlatformService(info.TargetPlatform)
	if tErr != nil {
		log.Println(tErr)
		return nil, tErr
	}

	targetTrackResult, ssErr := targetPlatformService.SearchTrackWithTitle(searchData)
	if ssErr != nil {
		log.Println(ssErr)
		return nil, ssErr
	}

	ppErr := pc.updatePlatformTracks(info.TargetPlatform, trackConversion, targetTrackResult)
	if ppErr != nil {
		log.Println(ppErr)
		return nil, ppErr
	}

	return trackConversion, nil
}

func (pc *Service) AsyncConvertPlaylist(info *blueprint.LinkInfo) (*blueprint.PlaylistConversion, error) {
	if info.TargetPlatform == "" {
		return nil, errors.New("target platform is required")
	}

	fromService, fErr := pc.factory.GetPlatformService(info.Platform)
	if fErr != nil {
		log.Printf("DEBUG: error getting platform service: %v", fErr)
		return nil, fErr
	}

	toService, tErr := pc.factory.GetPlatformService(info.TargetPlatform)
	if tErr != nil {
		log.Printf("DEBUG: error getting platform service: %v", tErr)
	}

	idSearchResult, sErr := fromService.SearchPlaylistWithID(info.EntityID)
	if sErr != nil {
		log.Printf("[internal][platforms][platform_factory]: %v", sErr)
		return nil, fmt.Errorf("error searching playlist: %v", sErr)
	}

	// fixme: pay attention here
	if len(idSearchResult.Tracks) == 0 {
		log.Printf("[internal][platforms][platform_factory] DEBUG(todo): no tracks found")
		return nil, nil
	}

	// todo: check this, dynamically set it perhaps or remove if negligible advantage
	result := &blueprint.PlaylistConversion{
		Meta: blueprint.PlaylistMetadata{
			Entity: "playlist",
			URL:    idSearchResult.URL,
			Title:  idSearchResult.Title,
			Length: idSearchResult.Length,
			Owner:  idSearchResult.Owner,
			Cover:  idSearchResult.Cover,
		},
	}

	// todo: send playlist conversion metadata event here

	workerCount := 10
	jobs := make(chan trackJob, workerCount)
	results := make(chan trackJob, workerCount)
	var resultsByIndex []trackJob
	var wg sync.WaitGroup

	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go pc.asyncPlaylistConversionWorker(&toService, jobs, results, &wg)
	}

	go func() {
		for i := range idSearchResult.Tracks {
			track := idSearchResult.Tracks[i]
			jobs <- trackJob{track: &blueprint.PlatformSearchTrack{
				Title:    track.Title,
				Artistes: track.Artists,
				URL:      track.URL,
				ID:       track.ID,
			}, index: i}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var fetchedResults []blueprint.TrackSearchResult
	var omittedResults []blueprint.OmittedTracks
	var mu sync.Mutex

	for searchResults := range results {
		mu.Lock()
		resultsByIndex = append(resultsByIndex, searchResults)
		mu.Unlock()
	}

	sort.SliceStable(resultsByIndex, func(i, j int) bool {
		return resultsByIndex[i].index < resultsByIndex[j].index
	})

	for i := 0; i < len(resultsByIndex); i++ {
		resultByIndex := resultsByIndex[i]
		if resultByIndex.err != nil || resultByIndex.result == nil {
			omittedResults = append(omittedResults, blueprint.OmittedTracks{
				Title:    idSearchResult.Tracks[i].Title,
				URL:      idSearchResult.Tracks[i].URL,
				Artistes: idSearchResult.Tracks[i].Artists,
				Platform: spotify.IDENTIFIER,
				Index:    i + 1,
			})
			continue
		}

		fetchedResults = append(fetchedResults, *resultByIndex.result)
	}

	srcPlatform := blueprint.PlatformPlaylistTrackResult{
		Tracks: &idSearchResult.Tracks,
		Length: util.SumUpResultLength(&idSearchResult.Tracks),
	}

	targetPlatform := blueprint.PlatformPlaylistTrackResult{
		Tracks: &fetchedResults,
		Length: util.SumUpResultLength(&fetchedResults),
	}

	uErr := pc.updatePlatformPlaylistTracks(info.Platform, result, &srcPlatform)
	if uErr != nil {
		return nil, fmt.Errorf("error updating platform tracks: %v", uErr)
	}

	uErr2 := pc.updatePlatformPlaylistTracks(info.TargetPlatform, result, &targetPlatform)
	if uErr2 != nil {
		log.Printf("DEBUG: error updating platform source playlist '%s' tracks: %v", info.TargetPlatform, uErr2)
	}

	result.OmittedTracks = &omittedResults
	return result, nil
}

// note: results is a send-only channel while jobs is a receive only channel

func (pc *Service) asyncPlaylistConversionWorker(sv *platforminternal.PlatformService, jobs <-chan trackJob, results chan<- trackJob, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		result := trackJob{
			track: job.track,
			index: job.index,
		}

		searchData := blueprint.TrackSearchData{
			Title:   job.track.Title,
			Artists: job.track.Artistes,
		}

		vrr := *sv

		trackR, sErr := vrr.SearchTrackWithTitle(&searchData)
		if sErr != nil {
			log.Printf("[internal][platforms][platform_factory]: could not call 'SearchTrackWithTitle' method on target platform service:, %v", sErr)
			result.err = sErr
			results <- result
			continue
		}

		result.result = trackR
		results <- result
	}
}

func (pc *Service) updatePlatformPlaylistTracks(
	platform string,
	conversion *blueprint.PlaylistConversion,
	tracks *blueprint.PlatformPlaylistTrackResult,
) error {
	switch platform {
	case deezer.IDENTIFIER:
		conversion.Platforms.Deezer = tracks
	case spotify.IDENTIFIER:
		conversion.Platforms.Spotify = tracks
	case applemusic.IDENTIFIER:
		conversion.Platforms.AppleMusic = tracks
	case tidal.IDENTIFIER:
		conversion.Platforms.Tidal = tracks
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
	return nil
}

func (pc *Service) updatePlatformTracks(platform string, conversion *blueprint.TrackConversion, tracks *blueprint.TrackSearchResult) error {
	switch platform {
	case deezer.IDENTIFIER:
		conversion.Platforms.Deezer = tracks
	case spotify.IDENTIFIER:
		conversion.Platforms.Spotify = tracks
	case applemusic.IDENTIFIER:
		conversion.Platforms.AppleMusic = tracks
	case tidal.IDENTIFIER:
		conversion.Platforms.Tidal = tracks
	case ytmusic.IDENTIFIER:
		conversion.Platforms.YTMusic = tracks

	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}

	return nil
}

func (pc *Service) sendWebhookPlaylistMetadataEvent(info *blueprint.LinkInfo, conversion *blueprint.PlaylistConversion) error {
	svixInstance := svixwebhook.New(os.Getenv("SVIX_API_KEY"), true)
	_, err := svixInstance.SendEvent(pc.factory.App.WebhookAppID, blueprint.PlaylistConversionMetadataEvent, &blueprint.PlaylistConversionEventMetadata{
		Platform:  info.Platform,
		Meta:      &conversion.Meta,
		EventType: blueprint.PlaylistConversionMetadataEvent,
		TaskId:    info.EntityID,
	})
	return err
}
