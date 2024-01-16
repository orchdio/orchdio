package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"log"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	"strings"
	"sync"
	"time"
)

// ExtractArtiste retrieves an artiste from a passed string containing something like
// feat.
func ExtractArtiste(artiste string) string {
	featIndex := strings.Index(artiste, "feat")
	if featIndex != -1 {
		return strings.Trim(artiste[:featIndex], " ")
	}
	return strings.ReplaceAll(artiste, " ", "")
}

type Service struct {
	IntegrationAppID     string
	IntegrationAppSecret string
	RedisClient          *redis.Client
	PgClient             *sqlx.DB
}

func NewService(credentials *blueprint.IntegrationCredentials, pgClient *sqlx.DB, redisClient *redis.Client) *Service {
	return &Service{
		IntegrationAppID:     credentials.AppID,
		IntegrationAppSecret: credentials.AppSecret,
		// the refreshtoken is optional for this so we're not declaring it
		RedisClient: redisClient,
		PgClient:    pgClient,
	}
}

// NewAuthToken returns a new auth token for the spotify integration. This is to be called
// everytime we make a call that requires user authentication or authorization or uses a specific scope.
func (s *Service) NewAuthToken() *oauth2.Token {
	config := &clientcredentials.Config{
		ClientID:     s.IntegrationAppID,
		ClientSecret: s.IntegrationAppSecret,
		TokenURL:     spotifyauth.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		if err.Error() == "oauth2: cannot fetch token: 503 Service Unavailable" {
			log.Printf("\n[services][spotify][base][SearchTrackWithID] error - 503 Service Unavailable\n")
			return nil
		}
		log.Printf("\n[services][spotify][base][SearchTrackWithID] error  - could not fetch spotify token: %v\n", err)
		return nil
	}
	return token
}

// NewClient returns a new spotify client
func (s *Service) NewClient(ctx context.Context, token *oauth2.Token) *spotify.Client {
	httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(ctx, token)
	return spotify.New(httpClient)
}

// FetchSingleTrack returns a single track by searching with the title
func (s *Service) FetchSingleTrack(searchData *blueprint.TrackSearchData) *spotify.SearchResult {
	config := &clientcredentials.Config{
		ClientID:     s.IntegrationAppID,
		ClientSecret: s.IntegrationAppSecret,
		TokenURL:     spotifyauth.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		log.Printf("\n[services][spotify][base][SearchTrackWithID] error  - could not fetch spotify token: %v\n", err)
		return nil
	}

	httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), token)
	client := spotify.New(httpClient)

	results, err := client.Search(context.Background(),
		fmt.Sprintf("artist:%s track:%s album:%s", searchData.Artists[0], searchData.Title, searchData.Album), spotify.SearchTypeTrack)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchingSingleTrack] error - could not search for track: %v\n", err)
		return nil
	}

	return results
}

// SearchTrackWithTitleChan searches a for a track using the title and channel
func (s *Service) SearchTrackWithTitleChan(searchData *blueprint.TrackSearchData, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup) {
	result, err := s.SearchTrackWithTitle(searchData)
	if err != nil {
		log.Printf("\nError fetching track %s with channels\n. Error: %v", searchData.Title, err)
		defer wg.Done()
		c <- nil
		wg.Add(1)

		return
	}
	defer wg.Done()
	c <- result
	wg.Add(1)

	return
}

// SearchTrackWithTitle searches spotify using the title of a track
// This is typically expected to be used when the track we want to fetch is the one we just
// want to search on. That is, the other platforms that the user is trying to convert to.
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData) (*blueprint.TrackSearchResult, error) {
	normalizedSearchData := fmt.Sprintf("spotify-%s-%s-%s", util.NormalizeString(searchData.Artists[0]), searchData.Title, searchData.Album)

	// if we have searched for this specific track before, we return the cached result
	// And how do we know if we have cached it before?
	// We store the hash of the title and artiste of the track in redis. we check if the hash of the
	// track we want to search exist.
	if s.RedisClient.Exists(context.Background(), normalizedSearchData).Val() == 1 {
		log.Printf("Spotify: Found cached result for %s", normalizedSearchData)
		// deserialize the result from redis
		var result *blueprint.TrackSearchResult
		cachedResult, err := s.RedisClient.Get(context.Background(), normalizedSearchData).Result()
		if err != nil {
			log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not get cached result for track. This is an unexpected error: %v\n", err)
			return nil, err
		}
		err = json.Unmarshal([]byte(cachedResult), &result)
		if err != nil {
			log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not unmarshal cached result: %v\n", err)
			return nil, err
		}
		return result, nil
	}

	spotifySearch := s.FetchSingleTrack(searchData)
	if spotifySearch == nil {
		log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - error fetching single track on spotify\n")
		// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		return nil, blueprint.ENORESULT
	}

	// probably better to deserialize the ```spotifySearch.Tracks``` so we can check if its nil or not
	// but it seems if its nil, then the spotiufySearch.Artists is also nil so check for that for now. but if
	// a similar problem where a result is empty but not detected as omittedTrack comes up again for spotify,
	// then we should check here and do the former.
	if len(spotifySearch.Tracks.Tracks) == 0 {
		log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - error fetching single track on spotify\n")
		// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		return nil, blueprint.ENORESULT
	}

	//if spotifySearch.Artists == nil {
	//	log.Printf("\n[controllers][platforms][spotify][ConvertEntity] error - error fetching single track on spotify\n")
	//	// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
	//	return nil, blueprint.ENORESULT
	//}

	log.Printf("\n[controllers][platforms][spotify][ConvertEntity] info - found %v tracks on spotify\n", len(spotifySearch.Tracks.Tracks))

	var spSingleTrack spotify.FullTrack

	// we're extracting just the first track.
	// NB: when the time comes to properly handle the results and return the best match (sometimes its like the 2nd result)
	// then, this is where to probably start.
	if len(spotifySearch.Tracks.Tracks) > 0 {
		spSingleTrack = spotifySearch.Tracks.Tracks[0]
	}

	var cover string
	// fetch the spotify image preview.
	if len(spSingleTrack.Album.Images) > 0 {
		cover = spSingleTrack.Album.Images[0].URL
	}

	// fetch all the tracks from the contributors.
	var spTrackContributors []string
	// reminder: for now, i'm just returning the name of the artiste
	for _, contributor := range spSingleTrack.Artists {
		spTrackContributors = append(spTrackContributors, contributor.Name)
	}

	fetchedSpotifyTrack := blueprint.TrackSearchResult{
		Released:      spSingleTrack.Album.ReleaseDate,
		URL:           spSingleTrack.SimpleTrack.ExternalURLs["spotify"],
		Artists:       spTrackContributors,
		Duration:      util.GetFormattedDuration(spSingleTrack.Duration / 1000),
		DurationMilli: spSingleTrack.Duration,
		Explicit:      spSingleTrack.Explicit,
		Title:         spSingleTrack.Name,
		Preview:       spSingleTrack.PreviewURL,
		Album:         spSingleTrack.Album.Name,
		ID:            spSingleTrack.SimpleTrack.ID.String(),
		Cover:         cover,
	}

	// serialize the track
	serializedTrack, err := json.Marshal(fetchedSpotifyTrack)
	trackCacheKey := "spotify:track:" + fetchedSpotifyTrack.ID

	if lo.Contains(fetchedSpotifyTrack.Artists, searchData.Artists[0]) {
		err = s.RedisClient.MSet(context.Background(), map[string]interface{}{
			normalizedSearchData: string(serializedTrack),
		}).Err()
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error caching track - %v\n", err)
		} else {
			log.Printf("\n[controllers][platforms][spotify][SearchTrackWithTitle] Track %s has been cached\n", fetchedSpotifyTrack.Title)
		}
	}

	//newIdentifier := util.HashIdentifier(fmt.Sprintf("spotify-%s-%s", _artiste, fetchedSpotifyTrack.Title))

	//if err != nil {
	//	log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not marshal track: %v\n", err)
	//	return nil, err
	//}
	err = s.RedisClient.Set(context.Background(), trackCacheKey, serializedTrack, time.Hour*24).Err()
	// cache in redis. since this is for a track that we're just searching by the title and artiste,
	// we're saving the hash with a scheme of: "spotify-artist-title". e.g "spotify-taylor-swift-blink-182"
	// so we save the fetched track under that hash and assumed that was what the user searched for and wanted.
	//err = red.Set(context.Background(), newIdentifier, serializedTrack, 0).Err()

	if err != nil {
		log.Printf("\n[services][spotify][base][SearchTrackWithTitle] error - could not cache spotify track: %v\n", err)
	} else {
		log.Printf("\n[services][spotify][base][SearchTrackWithTitle] success - cached track: %v\n", fetchedSpotifyTrack.Title)
	}

	return &fetchedSpotifyTrack, nil
}

// SearchTrackWithID fetches a track using a track (entityID) and return a spotify track.
// this is typically expected to be used when the track we want to convert is the one
// the user wants to convert. i.e the track is what the user wants to convert
// and from the link, we can get the trackID.
// Basically, the platform the user is trying to convert from.
func (s *Service) SearchTrackWithID(info *blueprint.LinkInfo) (*blueprint.TrackSearchResult, error) {
	// the cacheKey. scheme is "spotify:track_id"
	cacheKey := "spotify:track:" + info.EntityID
	cachedTrack, err := s.RedisClient.Get(context.Background(), cacheKey).Result()

	if err != nil && err != redis.Nil {
		log.Printf("\n[services][SearchTrackWithID] error - Could not fetch record from cache. This is an unexpected error\n")
		return nil, err
	}

	// we have not cached this track before
	if err != nil && err == redis.Nil {
		log.Printf("\n[services][SearchTrackWithID] function track has not been cached")
		//config := &clientcredentials.Config{
		//	ClientID:     s.IntegrationAppID,
		//	ClientSecret: s.IntegrationAppSecret,
		//	TokenURL:     spotifyauth.TokenURL,
		//}
		//
		//token, err := config.MusicToken(context.Background())
		//if err != nil {
		//	log.Printf("\n[services][spotify][base][SearchTrackWithID] error  - could not fetch spotify token: %v\n", err)
		//	return nil, err
		//}
		//
		//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), token)
		//client := spotify.New(httpClient)
		token := s.NewAuthToken()
		client := s.NewClient(context.Background(), token)
		results, err := client.GetTrack(context.Background(), spotify.ID(info.EntityID))
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchingSingleTrack] error - could not search for track: %v\n", err)
			return nil, err
		}

		var artistes []string
		strippedTitleInfo := util.ExtractTitle(results.Name)

		for _, artiste := range results.Album.Artists {
			artistes = append(artistes, artiste.Name)
		}

		str := strippedTitleInfo.Artists
		if len(str) > 0 {
			artistes = append(artistes, str...)
		}

		out := blueprint.TrackSearchResult{
			URL:           results.ExternalURLs["spotify"],
			Artists:       lo.Uniq(artistes),
			Released:      results.Album.ReleaseDate,
			Duration:      util.GetFormattedDuration(results.Duration / 1000),
			DurationMilli: results.Duration,
			Explicit:      results.Explicit,
			Title:         results.Name,
			Preview:       results.PreviewURL,
			Album:         results.Album.Name,
			ID:            results.ID.String(),
			Cover:         results.Album.Images[0].URL,
		}

		serialized, err := json.Marshal(out)
		if err != nil {
			log.Printf("\n[services][spotify][base][SearchTrackWithID] error - could not serialize track: %v\n", err)
		}

		err = s.RedisClient.Set(context.Background(), cacheKey, serialized, time.Hour*24).Err()
		if err != nil {
			log.Printf("\n[services][spotify][base][SearchTrackWithID] error - could not cache track: %v\n", err)
		} else {
			log.Printf("\n[services][spotify][base][SearchTrackWithID] success - track cached\n")
		}
		return &out, nil
	}

	var deserializedTrack blueprint.TrackSearchResult
	err = json.Unmarshal([]byte(cachedTrack), &deserializedTrack)
	if err != nil {
		log.Printf("\n[services][SearchTrackWithID] error - Could not deserialize track from cache\n")
		return nil, err
	}
	return &deserializedTrack, nil
}

// SearchPlaylistWithID fetches a playlist and returns a list of tracks and the playlist info with pagination info
// This function caches each of the tracks in the playlist, the playlist snapshop id in the scheme: "spotify:snapshot:id"
// and the playlist id in the scheme: "spotify:playlist:id"
func (s *Service) SearchPlaylistWithID(id string, webhookId, taskId string) (*blueprint.PlaylistSearchResult, error) {
	token := s.NewAuthToken()
	if token == nil {
		log.Printf("\n[services][spotify][base][SearchPlaylistWithID] error - could not fetch token\n")
		return nil, errors.New("could not fetch token")
	}
	ctx := context.Background()
	if token == nil {
		log.Printf("\n[services][spotify][base][SearchPlaylistWithID] error - could not fetch token\n")
		return nil, errors.New("could not fetch token")
	}
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID),
	//	spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(ctx, token)
	//client := spotify.New(httpClient)
	client := s.NewClient(ctx, token)
	options := spotify.Fields("description,uri,external_urls,snapshot_id,name,images")

	// --id--: 55JFgMW6BkDzIIHA7D3Wwo
	// --snapshotID--: MixiMzRkNTFkNDJhZTIyOGQ1ZWViZTFjYWI4OTIxMDdiNWE2ZTA5OGVm

	// first get the snapshot id from cache. if it exists and its the same as the one in the playlist,
	// then we want to return the cached data. however, if the snapshot id is different, we want to
	// fetch the data from the spotify api and cache it.
	cachedSnapshot, cacheErr := s.RedisClient.Get(context.Background(), "spotify:playlist:"+id).Result()
	if cacheErr != nil && cacheErr != redis.Nil {
		log.Printf("\n[services][SearchPlaylistWithID] error - Could not fetch snapshot id from cache\n")
		return nil, cacheErr
	}

	cachedSnapshotID, snapshotErr := s.RedisClient.Get(context.Background(), "spotify:snapshot:"+id).Result()
	if snapshotErr != nil && snapshotErr != redis.Nil {
		log.Printf("\n[services][SearchPlaylistWithID] error - Could not fetch snapshot id from cache\n")
		return nil, snapshotErr
	}

	info, err := client.GetPlaylist(context.Background(), spotify.ID(id), options)

	// if we have not cached this playlist before or the snapshot id has changed (i.e. the playlist has been updated)
	// then we want to fetch the tracks and cache them.
	if cacheErr != nil && cacheErr == redis.Nil || cachedSnapshotID != info.SnapshotID {

		playlist, cErr := client.GetPlaylistItems(ctx, spotify.ID(id))
		if cErr != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Could not fetch playlist from spotify: %v\n", cErr)
			return nil, cErr
		}
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - playlist fetched from spotify: %v\n", len(playlist.Items))

		// fetch ALL the pages
		for page := 1; ; page++ {
			out := &spotify.PlaylistItemPage{}
			paginationErr := client.NextPage(ctx, out)
			if errors.Is(paginationErr, spotify.ErrNoMorePages) {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - No more pages for playlist\n")
				break
			}
			if paginationErr != nil {
				log.Printf("\n[services][spotify][base][SearchPlaylistWithID] error - could not fetch playlist: %v\n", err)
				return nil, paginationErr
			}
			playlist.Items = append(playlist.Items, out.Items...)
		}

		var tracks []blueprint.TrackSearchResult

		// calculate the duration of the playlist
		playlistLength := 0
		for _, track := range playlist.Items {
			var artistes []string
			for _, artist := range track.Track.Track.Artists {
				artistes = append(artistes, artist.Name)
			}

			playlistLength += track.Track.Track.Duration / 1000

			var cover string
			if len(track.Track.Track.Album.Images) > 0 {
				cover = track.Track.Track.Album.Images[0].URL
			}

			trackCopy := blueprint.TrackSearchResult{
				URL:           track.Track.Track.ExternalURLs["spotify"],
				Artists:       artistes,
				Released:      track.Track.Track.Album.ReleaseDate,
				Duration:      util.GetFormattedDuration(track.Track.Track.Duration / 1000),
				DurationMilli: track.Track.Track.Duration,
				Explicit:      track.Track.Track.Explicit,
				Title:         track.Track.Track.Name,
				Preview:       track.Track.Track.PreviewURL,
				Album:         track.Track.Track.Album.Name,
				ID:            track.Track.Track.ID.String(),
				Cover:         cover,
			}
			tracks = append(tracks, trackCopy)
			// cache the track. the scheme is: "spotify:track_id"
			cacheKey := "spotify:track:" + track.Track.Track.ID.String()
			serialized, err := json.Marshal(trackCopy)
			if err != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not serialize track: %v\n", err)
			}
			err = s.RedisClient.Set(context.Background(), cacheKey, string(serialized), time.Hour*24).Err()
			if err != nil {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not cache track: %v\n", err)
			} else {
				log.Printf("\n[services][spotify][base][FetchPlaylistWithID] success - track %s by %s has been cached\n", trackCopy.Title, trackCopy.Artists[0])
			}
		}

		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - playlist trcaks length: %v\n", len(tracks))

		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Owner info is: %v\n", info.Owner)

		playlistResult := blueprint.PlaylistSearchResult{
			URL:    info.ExternalURLs["spotify"],
			Tracks: tracks,
			Title:  info.Name,
			Length: util.GetFormattedDuration(playlistLength),
			Owner:  info.Owner.DisplayName,
			Cover:  info.Images[0].URL,
		}

		// update the snapshotID in the cache
		err = s.RedisClient.Set(context.Background(), "spotify:snapshot:"+id, info.SnapshotID, 0).Err()
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not cache snapshot id: %v\n", err)
		} else {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] success - snapshot id has been cached %s\n", info.SnapshotID)
		}

		// cache the whole playlist
		serializedPlaylist, err := json.Marshal(playlistResult)
		if err != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not serialize playlist: %v\n", err)
		}

		err = s.RedisClient.Set(context.Background(), "spotify:playlist:"+id, string(serializedPlaylist), 0).Err()
		return &playlistResult, nil
	}

	// if we get here, then the snapshot id is the same as the one in the cache.
	// we can return the cached data
	playlistResult := blueprint.PlaylistSearchResult{}
	err = json.Unmarshal([]byte(cachedSnapshot), &playlistResult)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not unmarshal playlist: %v\n", err)
		return nil, err
	}

	return &playlistResult, nil
}

// FetchTracks fetches tracks in a playlist concurrently using channels
func (s *Service) FetchTracks(tracks []blueprint.PlatformSearchTrack, webhookId string) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var fetchedTracks []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var omittedTracks []blueprint.OmittedTracks
	var wg sync.WaitGroup
	for _, t := range tracks {
		// FIXME: unhandled slice index
		searchData := blueprint.TrackSearchData{
			Title:   t.Title,
			Artists: t.Artistes,
			Album:   t.Album,
		}
		go s.SearchTrackWithTitleChan(&searchData, ch, &wg)
		outputTrack := <-ch
		// for some reason, there is no spotify url which means could not fetch track, we
		// want to add to the list of "not found" tracks.
		if outputTrack == nil || outputTrack.URL == "" {
			// log info about empty track
			log.Printf("\n[services][spotify][base][SearchPlaylistWithTracks][warn] - Could not find track for %s\n", t.Title)
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Title: t.Title,
				URL:   t.URL,
				// WARNING: unhandled slice index
				Artistes: t.Artistes,
			})
			continue
		}

		fetchedTracks = append(fetchedTracks, *outputTrack)
	}

	wg.Wait()
	return &fetchedTracks, &omittedTracks
}

// SearchPlaylistWithTracks fetches the track for a playlist based on the search result
// from another platform
func (s *Service) SearchPlaylistWithTracks(p *blueprint.PlaylistSearchResult, webhookId, taskId string) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Artistes: track.Artists,
			Title:    track.Title,
			ID:       track.ID,
			URL:      track.URL,
		})
	}
	track, omittedTracks := s.FetchTracks(trackSearch, webhookId)
	return track, omittedTracks
}

func (s *Service) FetchPlaylistHash(token, playlistId string) []byte {
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), token)
	//client := spotify.New(httpClient)
	t := s.NewAuthToken()
	client := s.NewClient(context.Background(), t)
	opts := spotify.Fields("snapshot_id")

	info, err := client.GetPlaylist(context.Background(), spotify.ID(playlistId), opts)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistHash] error - could not fetch playlist: %v\n", err)
		return nil
	}

	return []byte(info.SnapshotID)
}

// FetchUserPlaylist fetches the user's playlist
func (s *Service) FetchUserPlaylist(token string) (*spotify.SimplePlaylistPage, error) {
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: token})
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), &oauth2.MusicToken{RefreshToken: token})
	//client := spotify.New(httpClient)
	playlists, err := client.CurrentUsersPlaylists(context.Background())
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchUserPlaylist] error - could not fetch playlist: %v\n", err)
		return nil, err
	}
	for {
		out := spotify.SimplePlaylistPage{}
		paginationErr := client.NextPage(context.Background(), &out)
		if paginationErr == spotify.ErrNoMorePages {
			log.Printf("\n[services][spotify][base][FetchUserPlaylist] - no more pages. User's full playlist retrieved\n")
			break
		}
		if paginationErr != nil {
			log.Printf("\n[services][spotify][base][FetchUserPlaylist] error - could not fetch playlist: %v\n", err)
			return nil, paginationErr
		}
		playlists.Playlists = append(playlists.Playlists, out.Playlists...)
	}

	return playlists, nil
}

func (s *Service) FetchUserArtists(token string) (*blueprint.UserLibraryArtists, error) {
	log.Printf("\n[services][spotify][base][FetchUserArtists] - fetching user's libraryArtists\n")
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), &oauth2.MusicToken{RefreshToken: token})
	//client := spotify.New(httpClient)
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: token})
	values := url.Values{}
	values.Set("limit", "50")
	libraryArtists, err := client.CurrentUsersFollowedArtists(context.Background(), spotify.Limit(50))
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchUserArtists] error - could not fetch libraryArtists: %v\n", err)
		return nil, err
	}
	for {
		if libraryArtists.Next == "" {
			log.Printf("\n[services][spotify][base][FetchUserArtists][warning] - no more pages. User's full artist list retrieved\n")
			break
		}
		out := spotify.FullArtistPage{}
		paginationErr := client.NextPage(context.Background(), &out)
		if paginationErr == spotify.ErrNoMorePages {
			log.Printf("\n[services][spotify][base][FetchUserArtists] - no more pages. User's full artist list retrieved\n")
			break
		}
		if paginationErr != nil {
			log.Printf("\n[services][spotify][base][FetchUserArtists] error - could not fetch libraryArtists: %v\n", err)
			return nil, paginationErr
		}
		libraryArtists.Artists = append(libraryArtists.Artists, out.Artists...)
		libraryArtists.Next = out.Next
	}

	var artists []blueprint.UserArtist
	for _, artist := range libraryArtists.Artists {
		pix := ""
		if len(artist.Images) > 0 {
			pix = artist.Images[0].URL
		}
		artists = append(artists, blueprint.UserArtist{
			ID:    string(artist.ID),
			Name:  artist.Name,
			Cover: pix,
			URL:   artist.ExternalURLs["spotify"],
		})
	}

	response := blueprint.UserLibraryArtists{
		Payload: artists,
		Total:   libraryArtists.Total,
	}
	return &response, nil
}

func (s *Service) FetchTrackListeningHistory(token string) ([]blueprint.TrackSearchResult, error) {
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), &oauth2.MusicToken{RefreshToken: token})
	//client := spotify.New(httpClient)
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: token})
	values := url.Values{}
	values.Set("limit", "50")
	libraryArtists, err := client.CurrentUsersTopTracks(context.Background(), spotify.Limit(50), spotify.Timerange("short_term"))
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchUserArtists] error - could not fetch libraryArtists: %v\n", err)
		if strings.Contains(err.Error(), "401") {
			return nil, errors.New("unauthorized")
		}

		if strings.Contains(err.Error(), "403") {
			return nil, errors.New("forbidden")
		}

		return nil, err
	}

	// each page returns 50 items. so theoretically, this should return the 250 tracks. this is similar to deezer's implementation so
	// 250 can be considered the default limit for this endpoint
	for i := 0; i < 5; i++ {
		out := spotify.FullTrackPage{}
		paginationErr := client.NextPage(context.Background(), &out)
		if paginationErr == spotify.ErrNoMorePages {
			log.Printf("\n[services][spotify][base][FetchUserArtists] - no more pages. User's full artist list retrieved\n")
			break
		}
		if paginationErr != nil {
			log.Printf("\n[services][spotify][base][FetchUserArtists] error - could not fetch libraryArtists: %v\n", err)
			return nil, err
		}
		libraryArtists.Tracks = append(libraryArtists.Tracks, out.Tracks...)
	}

	var tracks []blueprint.TrackSearchResult
	for _, track := range libraryArtists.Tracks {
		var artists []string
		for _, artist := range track.Artists {
			artists = append(artists, artist.Name)
		}

		cover := ""
		if len(track.Album.Images) > 0 {
			cover = track.Album.Images[0].URL
		}

		tracks = append(tracks, blueprint.TrackSearchResult{
			URL:           track.ExternalURLs["spotify"],
			Artists:       artists,
			Released:      track.Album.ReleaseDate,
			Duration:      util.GetFormattedDuration(track.Duration / 1000),
			DurationMilli: track.Duration,
			Explicit:      track.Explicit,
			Title:         track.Name,
			Preview:       track.PreviewURL,
			Album:         track.Album.Name,
			ID:            track.ID.String(),
			Cover:         cover,
		})
	}
	return tracks, nil
}

// FetchUserInfo fetches a user's profile information from spotify. This involves private information like the user's email so its not
// for cases where public information is needed.
func (s *Service) FetchUserInfo(token string) (*blueprint.UserPlatformInfo, error) {
	log.Printf("\n[services][spotify][base][FetchUserInfo] - fetching user's info\n")

	// first, we want to create the endpoint to fetch the user info
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), &oauth2.MusicToken{RefreshToken: token})
	//client := spotify.New(httpClient)
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: token})
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchUserInfo] error - could not fetch user info: %v\n", err)
		return nil, err
	}

	var profilePicture = ""
	if len(user.Images) > 0 {
		profilePicture = user.Images[0].URL
	}
	// todo: add support for explicit content level in the spotify library
	var profileInfo = blueprint.UserPlatformInfo{
		Platform:        "spotify",
		Username:        user.DisplayName,
		ProfilePicture:  profilePicture,
		Followers:       int(user.Followers.Count),
		PlatformID:      user.ID,
		PlatformSubPlan: user.Product,
		Url:             fmt.Sprintf("https://open.spotify.com/user/%s", user.ID),
	}
	log.Printf("\n[services][spotify][base][FetchUserInfo] - user spotify info fetched successfully\n")
	return &profileInfo, nil
}
