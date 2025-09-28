package service

import (
	"errors"
	"fmt"
	"log"
	"orchdio/blueprint"
	platforminternal "orchdio/internal/platform"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	"sync"

	"github.com/samber/lo"
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
	track          *blueprint.PlatformSearchTrack
	result         *blueprint.TrackSearchResult
	err            error
	index          int
	platform       string
	targetPlatform string
	// original link info. contains entity id and things like that.
	info *blueprint.LinkInfo
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
		Entity:         "track",
		UniqueID:       info.TaskID,
		ShortURL:       info.UniqueID,
		SourcePlatform: info.Platform,
		TargetPlatform: info.TargetPlatform,
	}

	uErr := pc.updatePlatformTracks(info.Platform, trackConversion, srcTrackResult)
	if uErr != nil {
		log.Println(uErr)
		return nil, uErr
	}

	searchData := &blueprint.TrackSearchData{
		Title:   srcTrackResult.Title,
		Artists: srcTrackResult.Artists,
		Meta: &blueprint.TrackSearchMeta{
			TaskID: info.EntityID,
		},
	}

	// fixme: magic string? — improve this.
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

// AsynqConvertPlaylist
func (pc *Service) AsynqConvertPlaylist(info *blueprint.LinkInfo) (*blueprint.PlaylistConversion, error) {
	if info.TargetPlatform == "" {
		return nil, errors.New("target platform is required")
	}

	var finalResult = &blueprint.PlaylistConversion{}

	fromService, fErr := pc.factory.GetPlatformService(info.Platform)
	if fErr != nil {
		log.Printf("DEBUG: error getting platform service: %v", fErr)
		return nil, fErr
	}

	toService, tErr := pc.factory.GetPlatformService(info.TargetPlatform)
	if tErr != nil {
		log.Printf("DEBUG: error getting platform service: %v", tErr)
	}

	// idSearchResult, sErr := fromService.SearchPlaylistWithID(info)
	playlistMeta, sErr := fromService.FetchPlaylistMetaInfo(info)
	if sErr != nil {
		log.Printf("[internal][platforms][platform_factory]: %v", sErr)
		return nil, fmt.Errorf("error searching playlist: %v", sErr)
	}

	ok := pc.factory.WebhookSender.SendPlaylistMetadataEvent(&blueprint.LinkInfo{
		Platform: info.Platform,
		EntityID: info.EntityID,
		App:      pc.factory.App.WebhookAppID,
	}, &blueprint.PlaylistConversionEventMetadata{
		Platform:  info.Platform,
		Meta:      playlistMeta,
		EventType: blueprint.PlaylistConversionMetadataEvent,
		TaskId:    info.TaskID,
		UniqueID:  info.UniqueID,
	})

	if !ok {
		log.Printf("[internal][platforms][platform_factory]: Could not send playlist conversion metadata event")
	} else {
		log.Printf("Sent playlist metadata conversion event to webhook provider for %s", info.Platform)
	}

	resultChan := make(chan blueprint.TrackSearchResult)

	var srcPlaylistTracks []blueprint.TrackSearchResult
	var targetPlaylistTracks []blueprint.TrackSearchResult

	var omittedTracksMeta []blueprint.MissingTrackEventPayload
	var omittedTracks []blueprint.OmittedTracks

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(resultChan)

		fErr := fromService.FetchTracksForSourcePlatform(info, playlistMeta, resultChan)
		if fErr != nil {
			log.Printf("Error fetching tracks... %v\n\n", fErr)
		}
	}()

	go func() {
		for result := range resultChan {

			// cache source track
			ok := util.CacheTrackByArtistTitle(&result, pc.factory.Red, info.Platform)
			if !ok {
				log.Printf("[service][AsynqConvertPlaylist][track-result-cache-error] Error caching source playlist track")
			}

			ok2 := util.CacheTrackByID(&result, pc.factory.Red, info.Platform)
			if !ok2 {
				log.Printf("[service][AsynqConvertPlaylist][track-result-cache-error] Error caching source playlist track")
			}

			searchData := &blueprint.TrackSearchData{
				Platform: info.TargetPlatform,
				Title:    result.Title,
				Artists:  result.Artists,
				Meta: &blueprint.TrackSearchMeta{
					TaskID: info.TaskID,
				},
			}

			targetPlatformTrack, sErr := toService.SearchTrackWithTitle(searchData)
			if sErr == blueprint.EnoResult {
				log.Printf("Error searching track... %v\n\n", sErr)
				log.Printf("Should add to omitted track here, send omitted track event & then send webhook event for the available track")

				meta := &blueprint.MissingTrackEventPayload{
					EventType: blueprint.PlaylistConversionMissingTrackEvent,
					TaskID:    info.TaskID,
					TrackMeta: blueprint.MissingTrackMeta{
						Platform:        info.Platform,
						MissingPlatform: info.TargetPlatform,
						Item:            result,
					},
				}

				mRes, missingWhErr := pc.factory.WebhookSender.SendEvent(pc.factory.App.WebhookAppID, blueprint.PlaylistConversionMissingTrackEvent, meta)

				if missingWhErr != nil {
					log.Printf("Error sending missing track webhook event... %v\n\n", missingWhErr)
				}

				log.Printf("Missing playlist track webhook event response is: %v", mRes)
				omittedTracksMeta = append(omittedTracksMeta, *meta)

				omittedTrack := &blueprint.OmittedTracks{
					Title:    result.Title,
					Artistes: result.Artists,
					// todo: remove this (and other fields not added here) from the result after confirming its ok.
					Platform: info.Platform,
					URL:      result.URL,
				}

				omittedTracks = append(omittedTracks, *omittedTrack)
				continue
			}

			// cache target track result
			ok3 := util.CacheTrackByArtistTitle(targetPlatformTrack, pc.factory.Red, info.TargetPlatform)
			if !ok3 {
				log.Printf("[service][AsynqConvertPlaylist][track-result-cache-error] Error caching target playlist track")
			}

			ok4 := util.CacheTrackByID(targetPlatformTrack, pc.factory.Red, info.TargetPlatform)
			if !ok4 {
				log.Printf("[service][AsynqConvertPlaylist][track-result-cache-error] Error caching target playlist track")
			}

			targetPlaylistTracks = append(targetPlaylistTracks, *targetPlatformTrack)
			playlistTrackConversionEventData := &blueprint.PlaylistTrackConversionEventResponse{
				EventType: blueprint.PlaylistConversionTrackEvent,
				TaskID:    info.TaskID,
				Tracks: []blueprint.PlaylistTrackConversionEventPayload{
					{
						Platform: info.Platform,
						Track:    &result,
					},
					{
						Platform: info.TargetPlatform,
						Track:    targetPlatformTrack,
					},
				},
			}

			_, whErr := pc.factory.WebhookSender.SendEvent(pc.factory.App.WebhookAppID, blueprint.PlaylistConversionTrackEvent, playlistTrackConversionEventData)
			if whErr != nil {
				log.Printf("Error sending playlist track conversion webhook: %v", whErr)
			}

			srcPlaylistTracks = append(srcPlaylistTracks, result)
		}
		wg.Done()
	}()

	wg.Add(1)
	wg.Wait()

	_, whErr := pc.factory.WebhookSender.SendEvent(pc.factory.App.WebhookAppID, blueprint.PlaylistConversionDoneEvent, &blueprint.PlaylistConversionDoneEventMetadata{
		EventType:      blueprint.PlaylistConversionDoneEvent,
		TaskID:         info.TaskID,
		PlaylistID:     info.EntityID,
		SourcePlatform: info.Platform,
		TargetPlatform: info.TargetPlatform,
		UniqueID:       info.UniqueID,
	})

	if whErr != nil {
		log.Printf("[service][AsynqConvertPlaylist] - Error sending playlist conversion done webhook: %v", whErr)
	}

	srcPlatformResultsErr := pc.updatePlatformPlaylistTracks(info.Platform, finalResult, &blueprint.PlatformPlaylistTrackResult{
		Tracks: &srcPlaylistTracks,
		Length: util.SumUpResultLength(&srcPlaylistTracks),
	})

	if srcPlatformResultsErr != nil {
		log.Printf("[service][AsynqConvertPlaylist] - FATAL: could not build src platform result struct")
		return nil, srcPlatformResultsErr
	}

	targetPlatformResultsErr := pc.updatePlatformPlaylistTracks(info.TargetPlatform, finalResult, &blueprint.PlatformPlaylistTrackResult{
		Tracks: &targetPlaylistTracks,
		Length: util.SumUpResultLength(&targetPlaylistTracks),
	})

	if targetPlatformResultsErr != nil {
		log.Printf("[service][AsynqConvertPlaylist] - FATAL: could not build target platform result struct")
		return nil, targetPlatformResultsErr
	}

	finalResult.OmittedTracks = &omittedTracks
	finalResult.Meta = *playlistMeta
	finalResult.Status = blueprint.TaskStatusCompleted

	finalResult.Platform = info.Platform
	finalResult.TargetPlatform = info.TargetPlatform
	// fixme: magiclink
	finalResult.Entity = "playlist"
	finalResult.UniqueID = info.UniqueID

	return finalResult, nil
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
