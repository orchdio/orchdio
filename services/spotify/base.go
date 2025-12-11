package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	svixwebhook "orchdio/webhooks/svix"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

type Service struct {
	IntegrationAppID     string
	IntegrationAppSecret string
	RedisClient          *redis.Client
	PgClient             *sqlx.DB
	App                  *blueprint.DeveloperApp
	WebhookSender        svixwebhook.SvixInterface
}

type WebhookSender interface {
	SendTrackEvent(appID string, event *blueprint.PlaylistConversionEventTrack) bool
}

func NewService(credentials *blueprint.IntegrationCredentials, pgClient *sqlx.DB, redisClient *redis.Client, devApp *blueprint.DeveloperApp, webhookSender svixwebhook.SvixInterface) *Service {
	return &Service{
		IntegrationAppID:     credentials.AppID,
		IntegrationAppSecret: credentials.AppSecret,
		// the refreshtoken is optional for this so we're not declaring it
		RedisClient:   redisClient,
		PgClient:      pgClient,
		App:           devApp,
		WebhookSender: webhookSender,
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

// extractArtiste retrieves an artiste from a passed string containing something like
// feat. Eg "Blanka Mazimela feat. Vanco"
func extractArtiste(artiste string) string {
	featIndex := strings.Index(artiste, "feat")
	if featIndex != -1 {
		return strings.Trim(artiste[:featIndex], " ")
	}
	return strings.ReplaceAll(artiste, " ", "")
}

// fetchSingleTrack returns a single track by searching with the title. This method is used when fetching a track
// using the SearchData from another service.
func (s *Service) fetchSingleTrack(searchData *blueprint.TrackSearchData) *spotify.SearchResult {
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

	results, err := client.Search(context.Background(), fmt.Sprintf("%s %s", searchData.Artists[0], searchData.Title), spotify.SearchTypeTrack)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchingSingleTrack] error - could not search for track: %v\n", err)
		return nil
	}

	return results
}

// SearchTrackWithTitle searches spotify using the title of a track
// This is typically expected to be used when the track we want to fetch is the one we just
// want to search on. That is, the other platforms that the user is trying to convert to.
func (s *Service) SearchTrackWithTitle(searchData *blueprint.TrackSearchData, requestAuthInfo blueprint.UserAuthInfoForRequests) (*blueprint.TrackSearchResult, error) {
	searchData.Artists[0] = extractArtiste(searchData.Artists[0])
	cleanedArtiste := fmt.Sprintf("spotify-%s-%s", util.NormalizeString(searchData.Artists[0]), searchData.Title)

	log.Printf("Spotify: Searching with stripped artiste: %s. Original artiste: %s", cleanedArtiste, searchData.Artists[0])
	// if we have searched for this specific track before, we return the cached result
	// And how do we know if we have cached it before?
	// We store the hash of the title and artiste of the track in redis. we check if the hash of the
	// track we want to search exist.
	if s.RedisClient.Exists(context.Background(), cleanedArtiste).Val() == 1 {
		log.Printf("Spotify: Found cached result for %s", cleanedArtiste)
		// deserialize the result from redis
		var result *blueprint.TrackSearchResult
		cachedResult, err := s.RedisClient.Get(context.Background(), cleanedArtiste).Result()
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

	spotifySearch := s.fetchSingleTrack(searchData)
	if spotifySearch == nil {
		log.Printf("\n[controllers][platforms][spotify][ConvertPlaylist] error - error fetching single track on spotify\n")
		// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		return nil, blueprint.EnoResult
	}

	// probably better to deserialize the ```spotifySearch.Tracks``` so we can check if its nil or not
	// but it seems if its nil, then the spotiufySearch.Artists is also nil so check for that for now. but if
	// a similar problem where a result is empty but not detected as omittedTrack comes up again for spotify,
	// then we should check here and do the former.
	if len(spotifySearch.Tracks.Tracks) == 0 {
		log.Printf("\n[controllers][platforms][spotify][ConvertPlaylist] error - error fetching single track on spotify\n")
		// panic for now.. at least until i figure out how to handle it if it can fail at all or not or can fail but be taken care of
		return nil, blueprint.EnoResult
	}
	log.Printf("\n[controllers][platforms][spotify][ConvertPlaylist] info - found %v tracks on spotify\n", len(spotifySearch.Tracks.Tracks))

	var fullSpotifyTrack spotify.FullTrack

	// we're extracting just the first track.
	// NB: when the time comes to properly handle the results and return the best match (sometimes its like the 2nd result)
	// then, this is where to probably start.
	if len(spotifySearch.Tracks.Tracks) > 0 {
		fullSpotifyTrack = spotifySearch.Tracks.Tracks[0]
	}

	var cover string
	// fetch the spotify image preview.
	if len(fullSpotifyTrack.Album.Images) > 0 {
		cover = fullSpotifyTrack.Album.Images[0].URL
	}

	// fetch all the tracks from the contributors.
	var spTrackContributors []string
	// reminder: for now, i'm just returning the name of the artiste
	for _, contributor := range fullSpotifyTrack.Artists {
		spTrackContributors = append(spTrackContributors, contributor.Name)
	}

	fetchedSpotifyTrack := blueprint.TrackSearchResult{
		Released:      fullSpotifyTrack.Album.ReleaseDate,
		URL:           fullSpotifyTrack.SimpleTrack.ExternalURLs["spotify"],
		Artists:       spTrackContributors,
		Duration:      util.GetFormattedDuration(int(fullSpotifyTrack.Duration) / 1000),
		DurationMilli: int(fullSpotifyTrack.Duration),
		Explicit:      fullSpotifyTrack.Explicit,
		Title:         fullSpotifyTrack.Name,
		Preview:       fullSpotifyTrack.PreviewURL,
		Album:         fullSpotifyTrack.Album.Name,
		ID:            fullSpotifyTrack.SimpleTrack.ID.String(),
		Cover:         cover,
	}

	ok := util.CacheTrackByID(&fetchedSpotifyTrack, s.RedisClient, IDENTIFIER)
	if !ok {
		log.Printf("[services][platforms][spotify][ConvertPlaylist] error - could not save cached result")
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

	if err != nil && !errors.Is(err, redis.Nil) {
		log.Printf("\n[services][SearchTrackWithID] error - Could not fetch record from cache. This is an unexpected error\n")
		return nil, err
	}

	// we have not cached this track before
	if err != nil && errors.Is(err, redis.Nil) {
		log.Printf("\n[services][SearchTrackWithID] function track has not been cached")
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
			Duration:      util.GetFormattedDuration(int(results.Duration) / 1000),
			DurationMilli: int(results.Duration),
			Explicit:      results.Explicit,
			Title:         results.Name,
			Preview:       results.PreviewURL,
			Album:         results.Album.Name,
			ID:            results.ID.String(),
			Cover:         results.Album.Images[0].URL,
		}

		ok := s.WebhookSender.SendTrackEvent(s.App.WebhookAppID, &blueprint.PlaylistConversionEventTrack{
			EventType: blueprint.PlaylistConversionTrackEvent,
			Platform:  IDENTIFIER,
			TaskId:    info.EntityID,
			Track:     &out,
		})

		if !ok {
			log.Print("[services][platforms][spotify][base][SearchTrackWithTitle] error - could not send track webhook event")
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

// FetchPlaylistMetaInfo fetches metadata for a playlist. It'll always return the latest metadata as we hit the Spotify API.
func (s *Service) FetchPlaylistMetaInfo(info *blueprint.LinkInfo) (*blueprint.PlaylistMetadata, error) {
	token := s.NewAuthToken()
	if token == nil {
		log.Printf("\n[services][spotify][base][SearchPlaylistWithID] error - could not fetch token\n")
		return nil, errors.New("could not fetch token")
	}

	ctx := context.Background()
	client := s.NewClient(ctx, token)
	options := spotify.Fields("description,uri,external_urls,snapshot_id,name,images,owner,tracks(total,items(track))")

	cacheKey := util.FormatPlatformConversionCacheKey(info.EntityID, IDENTIFIER)
	_, cacheErr := s.RedisClient.Get(context.Background(), cacheKey).Result()

	if cacheErr != nil && !errors.Is(cacheErr, redis.Nil) {
		log.Printf("\n[services][SearchPlaylistWithID] error - Could not fetch snapshot id from cache\n")
		return nil, cacheErr
	}

	_, snapshotErr := s.RedisClient.Get(context.Background(), util.FormatPlatformPlaylistSnapshotID(IDENTIFIER, info.EntityID)).Result()
	if snapshotErr != nil && !errors.Is(snapshotErr, redis.Nil) {
		log.Printf("\n[services][SearchPlaylistWithID] error - Could not fetch snapshot id from cache\n")
		return nil, snapshotErr
	}

	playlistInfo, err := client.GetPlaylist(context.Background(), spotify.ID(info.EntityID), options)
	if err != nil {
		return nil, err
	}

	playlistMeta := &blueprint.PlaylistMetadata{
		// no length yet. its calculated by adding up all the track lengths in the playlist
		Length: "",
		Title:  playlistInfo.SimplePlaylist.Name,
		// no preview. might be an abstracted implementation in the future.
		Preview: "",

		// todo: fetch user's orchdio @ and/or id and enrich. might need a specific owner object
		Owner: playlistInfo.Owner.DisplayName,
		// fixme: possible nil pointer
		Cover:       playlistInfo.SimplePlaylist.Images[0].URL,
		Entity:      "playlist",
		URL:         playlistInfo.ExternalURLs["spotify"],
		NBTracks:    int(playlistInfo.Tracks.Total),
		Description: playlistInfo.Description,
		// similar to deezer, spotify does not have a field that specifies last updated, so we'll use snapshotid
		// (deezer calls it checksum) to know if the version of the playlist has changed since last we fetched & cached.
		Checksum: playlistInfo.SnapshotID,
		// no short url here.
		// todo: try to pass the entity id here
		//ShortURL: info.TaskID,
		ID: string(playlistInfo.ID),
	}

	return playlistMeta, nil
}

// FetchTracksForSourcePlatform fetches the tracks for a given playlist (with playlistID). Its the method used
// to fetch the tracks in a playlist if the user is trying to convert from spotify to another platform.
func (s *Service) FetchTracksForSourcePlatform(info *blueprint.LinkInfo, playlistMeta *blueprint.PlaylistMetadata, resultChan chan blueprint.TrackSearchResult) error {
	token := s.NewAuthToken()
	if token == nil {
		log.Printf("\n[services][spotify][base][SearchPlaylistWithID] error - could not fetch token\n")
		return errors.New("could not fetch token")
	}

	ctx := context.Background()
	client := s.NewClient(ctx, token)

	cacheKey := util.FormatPlatformConversionCacheKey(info.EntityID, IDENTIFIER)
	cachedSnapshot, cacheErr := s.RedisClient.Get(context.Background(), cacheKey).Result()

	if cacheErr != nil && !errors.Is(cacheErr, redis.Nil) {
		log.Printf("\n[services][SearchPlaylistWithID] error - Could not fetch snapshot id from cache\n")
		return cacheErr
	}

	cachedSnapshotID, snapshotErr := s.RedisClient.Get(context.Background(), util.FormatPlatformPlaylistSnapshotID(IDENTIFIER, info.EntityID)).Result()
	if snapshotErr != nil && !errors.Is(snapshotErr, redis.Nil) {
		log.Printf("\n[services][SearchPlaylistWithID] error - Could not fetch snapshot id from cache\n")
		return snapshotErr
	}

	if cacheErr != nil && errors.Is(cacheErr, redis.Nil) || cachedSnapshotID != playlistMeta.Checksum {

		// playlist, cErr := client.GetPlaylistItems(ctx, spotify.ID(info.EntityID), spotify.Fields("tracks.items, album"))

		playlist, cErr := client.GetPlaylistItems(ctx, spotify.ID(info.EntityID))
		if cErr != nil {
			log.Printf("\n[services][spotify][base][FetchPlaylistWithID] - Could not fetch playlist from spotify: %v\n", cErr)
			return cErr
		}

		var outItems []spotify.PlaylistItem

		// make a copy of the value of the playlist pointer.
		out := *playlist
		// fetch all pages and their tracks, we'll later append, loop over and send each track to the channel
		for {
			log.Printf("Page has %d tracks", len(out.Items))
			err := client.NextPage(ctx, &out)
			if err == spotify.ErrNoMorePages {
				break
			}
			// outt = append(outt, playlist.Items...)
			if err != nil {
				log.Fatal(err)
			}
			outItems = append(outItems, out.Items...)
		}

		playlist.Items = append(playlist.Items, outItems...)
		for _, track := range playlist.Items {
			var artistes []string
			for _, artist := range track.Track.Track.Artists {
				artistes = append(artistes, artist.Name)
			}

			// todo: move this to the point where we can update the playlistmeta (in the case of spotify conversion) in the service platform
			// playlistLength += track.Track.Track.Duration / 1000

			var cover string
			if len(track.Track.Track.Album.Images) > 0 {
				cover = track.Track.Track.Album.Images[0].URL
			}

			trackCopy := blueprint.TrackSearchResult{
				URL:           track.Track.Track.ExternalURLs["spotify"],
				Artists:       artistes,
				Released:      track.Track.Track.Album.ReleaseDate,
				Duration:      util.GetFormattedDuration(int(track.Track.Track.Duration) / 1000),
				DurationMilli: int(track.Track.Track.Duration),
				Explicit:      track.Track.Track.Explicit,
				Title:         track.Track.Track.Name,
				Preview:       track.Track.Track.PreviewURL,
				Album:         track.Track.Track.Album.Name,
				ID:            track.Track.Track.ID.String(),
				Cover:         cover,
			}
			resultChan <- trackCopy
		}
		return nil
	}

	playlistResult := blueprint.PlaylistSearchResult{}
	err := json.Unmarshal([]byte(cachedSnapshot), &playlistResult)
	if err != nil {
		log.Printf("\n[services][spotify][base][FetchPlaylistWithID] error - could not unmarshal playlist: %v\n", err)
		return err
	}

	for _, track := range playlistResult.Tracks {
		resultChan <- track
	}
	return nil
}

// FetchPlaylistHash fetches the hash of a playlist.
// DEPRECATED: remove reference to this function in (services/HasPlaylistBeenUpdated)
func (s *Service) FetchPlaylistHash(token, playlistId string) []byte {
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

func (s *Service) FetchUserArtists(refreshToken string) (*blueprint.UserLibraryArtists, error) {
	log.Printf("\n[services][spotify][base][FetchUserArtists] - fetching user's libraryArtists\n")
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: refreshToken})
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
		Total:   int(libraryArtists.Total),
	}
	return &response, nil
}

func (s *Service) FetchListeningHistory(token string) ([]blueprint.TrackSearchResult, error) {
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: token})
	recentlyPlayed, err := client.PlayerRecentlyPlayedOpt(context.Background(), &spotify.RecentlyPlayedOptions{
		Limit: 50,
	})
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

	var tracks []blueprint.TrackSearchResult
	for _, track := range recentlyPlayed {
		var artists []string
		for _, artist := range track.Track.Artists {
			artists = append(artists, artist.Name)
		}

		cover := ""
		if len(track.Track.Album.Images) > 0 {
			cover = track.Track.Album.Images[0].URL
		}

		tracks = append(tracks, blueprint.TrackSearchResult{
			URL:           track.Track.Album.ExternalURLs["spotify"],
			Artists:       artists,
			Released:      track.Track.Album.ReleaseDate,
			Duration:      util.GetFormattedDuration(int(track.Track.Duration) / 1000),
			DurationMilli: int(track.Track.Duration),
			Explicit:      track.Track.Explicit,
			Title:         track.Track.Name,
			Preview:       track.Track.PreviewURL,
			Album:         track.Track.Album.Name,
			ID:            track.Track.ID.String(),
			Cover:         cover,
		})
	}
	return tracks, nil
}

// FetchUserInfo fetches a user's profile information from spotify. This involves private information like the user's email so its not
// for cases where public information is needed.
func (s *Service) FetchUserInfo(authInfo blueprint.UserAuthInfoForRequests) (*blueprint.UserPlatformInfo, error) {
	log.Printf("\n[services][spotify][base][FetchUserInfo] - fetching user's info\n")

	// first, we want to create the endpoint to fetch the user info
	//httpClient := spotifyauth.New(spotifyauth.WithClientID(s.IntegrationAppID), spotifyauth.WithClientSecret(s.IntegrationAppSecret)).Client(context.Background(), &oauth2.MusicToken{RefreshToken: token})
	//client := spotify.New(httpClient)
	//
	//
	// todo: use refreshing accessToken using the refreshToken if its expired or use accessToken if it hasnt.
	client := s.NewClient(context.Background(), &oauth2.Token{RefreshToken: authInfo.RefreshToken})
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
