package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/badoux/goscraper"
	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"log"
	"net/url"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/services/tidal"
	"orchdio/services/ytmusic"
	"orchdio/util"
	"os"
	"strings"
)

// ExtractLinkInfo extracts a URL from a URL.
func ExtractLinkInfo(t string) (*blueprint.LinkInfo, error) {
	// first, check if the link is a shortlink

	/**
	* first, get the host of the URL. Typically, its https://www.deezer.com/en/artist/4037971
	* with the format in the way of: https://www.deezer.com/:country_locale_shortcode/:entity/:id
	* where country_locale_shortcode is the two-letter shortcode for the country/region the account
	* or resource is located, entity being either of artist, user or track. The link can also come with
	* some tracking parameters/queries but not exactly a major focus with Deezer, for the most part at least
	* for now.
	* Also, the URL can also come in the form of a shortlink. This shortlink (typically) changes with each
	* time a person copies it from Deezer.
	*
	* The URL for spotify is like: https://open.spotify.com/track/2I3dW2dCBZAJGj5X21E53k?si=ae80d72a4cc149ad
	* with the format in the way of: https://open.spotify.com/:entity/:id?:tracking_params
	* where entity is either of artiste, user or track (or something like that), the id being the ID of the
	* entity and tracking_params for tracking.
	 */

	song, escapeErr := url.QueryUnescape(t)
	if escapeErr != nil {
		log.Printf("\n[services][ExtractLinkInfo][error] Error escaping URL: %v\n", escapeErr)
		return nil, escapeErr
	}
	// TODO: before parsing, check if it looks like a valid track/playlist url on supported services
	// check if the "song" is a url
	contains := strings.Contains(song, "https://")
	if !contains {
		log.Printf("[services][ExtractLinkInfo][warning] link doesnt seem to be https.")
		return nil, blueprint.EINVALIDLINK
	}

	if len([]byte(song)) > 200 {
		log.Printf("[services][ExtractLinkInfo][warning] link is larger than 100 bytes")
		return nil, errors.New("too large")
	}

	parsedURL, parseErr := url.Parse(strings.TrimSpace(song))
	if parseErr != nil {
		log.Printf("\n[services][ExtractLinkInfo][error] Error parsing escaped URL: %v\n", parseErr)
		return nil, parseErr
	}

	log.Printf("[services][ExtractLinkInfo][info] Parsed URL: %v\n", parsedURL)
	// index of the beginning of useless params/query (aka tracking links) in our link
	// if there are, then we want to remove them.
	// however for ytmusic music, the link is actually part of a query param, so we need to
	// make an exception for that. the link is in the form of: https://music.youtube.com/watch?v=2I3dW2dCBZAJGj5X21E53k&feature=share
	trailingCharIndex := strings.Index(song, "?")
	// HACK: check if the link is a ytmusic music link
	isYT := strings.Contains(song, "music.ytmusic.com")
	if trailingCharIndex != -1 && !isYT {
		song = song[:trailingCharIndex]
	}

	host := parsedURL.Host
	var (
		entityID string
		entity   = "track"
	)
	playlistIndex := strings.Index(song, "playlist")
	trackIndex := strings.Index(song, "track")

	switch host {
	case util.Find(blueprint.DeezerHost, host):
		// first, check the type of URL it is. for now, only track.
		if strings.Contains(song, "deezer.page.link") {
			// it contains a shortlink.
			previewResult, err := goscraper.Scrape(song, 10)
			if err != nil {
				log.Printf("\n[services][ExtractLinkInfo][error] could not retrieve preview of link: %v", previewResult)
				return nil, err
			}

			playlistIndex = strings.Index(previewResult.Preview.Link, "playlist")
			if playlistIndex != -1 {
				entityID = previewResult.Preview.Link[playlistIndex+9:]
				entity = "playlist"
			} else {
				trackIndex = strings.Index(previewResult.Preview.Link, "track")
				entityID = previewResult.Preview.Link[trackIndex+6:]
			}
		} else {
			// it doesnt contain a preview URL and its a deezer track
			if playlistIndex != -1 {
				entityID = song[playlistIndex+9:]
				entity = "playlist"
			} else {
				entityID = song[trackIndex+6:]
			}
		}

		// then we want to return the real URL.
		linkInfo := &blueprint.LinkInfo{
			Platform:   deezer.IDENTIFIER,
			TargetLink: fmt.Sprintf("%s/%s/%s", os.Getenv("DEEZER_API_BASE"), entity, entityID),
			Entity:     entity,
			EntityID:   entityID,
		}
		return linkInfo, nil
	case blueprint.SpotifyHost:
		if playlistIndex != -1 {
			entityID = song[34:]
			entity = "playlists"
		} else {
			// then we rename the default entity to tracks, for spotify. because that's what
			// the URL scheme for spotify uses.
			entity = "tracks"
			entityID = song[31:]
		}

		// then we want to return the real URL.
		linkInfo := &blueprint.LinkInfo{
			Platform:   spotify.IDENTIFIER,
			TargetLink: fmt.Sprintf("%s/%s/%s", os.Getenv("SPOTIFY_API_BASE"), entity, entityID),
			Entity:     entity,
			EntityID:   entityID,
		}

		return linkInfo, nil

	case blueprint.TidalHost:
		playlistIndex = strings.Index(song, "playlist")
		if playlistIndex != -1 {
			entityID = song[playlistIndex+9:]
			entity = "playlists"
		} else {
			trackIndex = strings.Index(song, "track")
			entityID = song[trackIndex+6:]
			entity = "tracks"
		}

		linkInfo := blueprint.LinkInfo{
			Platform:   tidal.IDENTIFIER,
			TargetLink: fmt.Sprintf("%s/%s/%s", os.Getenv("TIDAL_API_BASE"), entity, entityID),
			Entity:     entity,
			EntityID:   entityID,
		}

		log.Printf("[services][ExtractLinkInfo][info] LinkInfo: %v", linkInfo)

		return &linkInfo, nil

	case blueprint.YoutubeHost:
		// handle tracks first.
		// in the original link, the track ID is in the form of: https://www.youtube.com/watch?v=2I3dW2dCBZAJGj5X21E53k&list=RDAMVM2I3dW2dCBZAJGj5X21E53k
		// but the preference will depend on whichever comes first —— v= or list=
		trackParam := parsedURL.Query().Get("v")
		playlistParam := parsedURL.Query().Get("list")

		if trackParam == "" && playlistParam == "" {
			log.Printf("[services][ExtractLinkInfo][error] Youtube link does not contain a track or playlist ID.")
			return nil, blueprint.EINVALIDLINK
		}

		if playlistParam == "" && trackParam != "" {
			log.Printf("[services][ExtractLinkInfo][info] Youtube link is a track.")
			linkInfo := blueprint.LinkInfo{
				Platform: ytmusic.IDENTIFIER,
				// we're doing sprintf manually instead of the original url because the original url might have tracking links attached.cd ..
				TargetLink: fmt.Sprintf("%s/watch?=%s", os.Getenv("YTMUSIC_API_BASE"), trackParam),
				Entity:     "track",
				EntityID:   trackParam,
			}
			log.Printf("[services][ExtractLinkInfo][info] LinkInfo: %v", linkInfo)
			return &linkInfo, nil
		}

		if playlistParam != "" && trackParam == "" {
			log.Printf("[services][ExtractLinkInfo][info] Youtube link is a playlist.")
			linkInfo := blueprint.LinkInfo{
				Platform:   ytmusic.IDENTIFIER,
				TargetLink: fmt.Sprintf("%s/playlist?list=%s", os.Getenv("YTMUSIC_API_BASE"), playlistParam),
				Entity:     "playlist",
				EntityID:   playlistParam,
			}
			log.Printf("[services][ExtractLinkInfo][info] LinkInfo: %v", linkInfo)
			return &linkInfo, nil
		}

	case blueprint.AppleMusicHost:
		// https://music.apple.com/ng/album/one-of-them-feat-big-sean/1544326461?i=1544326471 - track
		// https://music.apple.com/ng/playlist/eazy/pl.u-AkAmPlyUxJ6xEl7 -- playlist
		trackID := parsedURL.Query().Get("i")
		p := strings.LastIndex(song, "/")
		playlistID := song[p:]
		log.Printf("[services][ExtractLinkInfo][info] AppleMusic trackID, playistID: %s %s", trackID, playlistID)
		// strip away query params
		if trackID == "" && playlistID == "" {
			return nil, blueprint.EINVALIDLINK
		}

		if trackID != "" {
			entityID = trackID
			entity = "track"
		}

		if trackID == "" && playlistID != "" {
			entityID = playlistID
			entity = "playlist"
		}

		linkInfo := &blueprint.LinkInfo{
			Platform:   "applemusic",
			TargetLink: song,
			Entity:     entity,
			EntityID:   entityID,
		}

		return linkInfo, nil

		// to handle pagination.
		// TODO: create magic string for these

		//----------------------------------------------------------------------------------
		// :⚠️ DOES NOT SEEM TO BE IN USE. KEEP AROUND FOR FUTURE REFERENCE.
		// PAGINATION SUPPORT.

	case "api.spotify.com":
		log.Printf("\n[servies][s: Track] URL info looks like a playlist pagination.")
		linkInfo := blueprint.LinkInfo{
			Platform:   spotify.IDENTIFIER,
			TargetLink: t,
			Entity:     "playlists",
			EntityID:   util.ExtractSpotifyID(t),
		}
		log.Printf("\n[debug][🔔] linkfo is: %v\n", linkInfo)
		return &linkInfo, nil

	case "api.deezer.com":
		log.Printf("\n[services][Playlists] URL info looks like a deezer playlist pagination")
		eID := util.ExtractDeezerID(t)
		log.Printf("\nExtractedID for deezer is: %v\n", eID)
		linkInfo := blueprint.LinkInfo{
			Platform:   deezer.IDENTIFIER,
			TargetLink: t,
			Entity:     "playlists",
			EntityID:   eID,
		}
		return &linkInfo, nil
	default:
		log.Printf("\n[servies][s: Track][error] URL info could not be processed. Might be an invalid link")
		log.Printf(host)
		return nil, blueprint.EHOSTUNSUPPORTED
	}
	return nil, nil
}

type SyncFollowTask struct {
	DB  *sqlx.DB
	Red *redis.Client
}

func NewFollowTask(db *sqlx.DB, red *redis.Client) *SyncFollowTask {
	return &SyncFollowTask{
		DB:  db,
		Red: red,
	}
}

// HasPlaylistBeenUpdated checks if the playlist has been updated. However, there is more that it does and it returns quite
// the number of information.
//  - is the playlist updated? For tidal, this is done by checking the last time it was updated. For other platforms, we check the checksum.
// It returns the following:
//  - the latest hash of the playlist. It makes calls to the endpoints so we know we always get the latest hash.
//  - a boolean indicating if the playlist has been updated.
//  - a slice of byte representing the platform which we checked for the update.
//  - an error, if any
func (s *SyncFollowTask) HasPlaylistBeenUpdated(platform, entity, entityId, appID string) ([]byte, bool, []byte, error) {
	// first check the hash in redis
	snapshotID := fmt.Sprintf("%s:snapshot:%s", platform, entityId)
	log.Printf("\n[services][SyncFollowTask][info] Checking if playlist has been updated with snapshot id: %s\n", snapshotID)

	cachedSnapshot, cacheErr := s.Red.Get(context.Background(), snapshotID).Result()
	// entity comes from the link info. so we need to check if the entity is a playlist or a track and for some
	// platforms, the entity can be returned in plural, i.e playlists or tracks.
	supportedEntities := []string{"playlists", "tracks"}

	log.Printf("\n[services][SyncFollowTask][info] entity is: %v\n", entity)
	if cacheErr != nil {
		// TODO: handle possible redis.Nil error
		if cacheErr == redis.Nil {
			log.Printf("[follow][FetchPlaylistHash] - Entity hasnt been cached: %v", cacheErr)
			return nil, false, nil, cacheErr
		}
		log.Printf("[follow][FetchPlaylistHash] - error getting playlist hash from redis: %v", cacheErr)
		return nil, false, nil, cacheErr
	}
	database := db.NewDB{DB: s.DB}
	// fetch the app that owns this request
	app, err := database.FetchAppByAppIdWithoutDevId(appID)
	if err != nil {
		log.Printf("[follow][FetchPlaylistHash] - error fetching app: could not fetch app with the ID - %s %v", err, appID)
		return nil, false, nil, err
	}

	var outBytes []byte
	switch platform {
	case spotify.IDENTIFIER:
		outBytes = app.SpotifyCredentials
	case tidal.IDENTIFIER:
		outBytes = app.TidalCredentials
	case deezer.IDENTIFIER:
		outBytes = app.DeezerCredentials
	}

	var creds blueprint.IntegrationCredentials
	err = json.Unmarshal(outBytes, &creds)
	if err != nil {
		log.Printf("[follow][FetchPlaylistHash] - error deserializing platform credentials: %v", err)
		return nil, false, nil, err
	}

	// if we got a cached snapshot, we want to get the latest snapshot for a playlist by making a request
	// to the platform's api
	var entitySnapshot string
	var platformBytes []byte
	if lo.Contains(supportedEntities, entity) {
		switch platform {
		// TODO: implement other platforms
		case "spotify":
			log.Printf("[follow][FetchPlaylistHash] - checking if playlist has been updated")
			spotifyService := spotify.NewService(&creds, s.DB, s.Red)
			// fixme: there is a bug here. we need to pass the user's auth token to the fetchplaylisthash function
			// 		question is: how do we get the user's auth token in this case? unless whenever we run this function,
			// 		we let it run in the context of an authed user request, so that way we can always get the user's auth token
			//      or we pass the developer credentials (use NewAuthToken on service) and see if it works
			// 		since it works for normal conversions.
			ent := string(spotifyService.FetchPlaylistHash("", entityId))
			if ent == "" {
				return nil, false, nil, fmt.Errorf("could not get playlist hash")
			}
			log.Printf("[follow][FetchPlaylistHash] - playlist hash is: %v", ent)
			entitySnapshot = ent
			log.Printf("[follow][FetchPlaylistHash] - fetched playlist hash from spotify: %v", entitySnapshot)
			//case "tidal":
			//	log.Printf("[follow][FetchPlaylistHash] - checking if playlist has been updated")
			//	platform = "tidal"
			//	tidalService := tidal.NewService(&creds, s.DB, s.Red)
			//	info, _, ok, err := tidalService.SearchPlaylistWithID(entityId)
			//	if err != nil {
			//		log.Printf("[follow][FetchPlaylistHash] - error fetching playlist from tidal: %v", err)
			//		return nil, false, nil, err
			//	}
			//	if !ok {
			//		log.Printf("[follow][FetchPlaylistHash] - playlist has not been updated")
			//		return nil, false, nil, nil
			//	}
			//	platformBytes, _ = json.Marshal(platform)
			//	log.Printf("[follow][FetchPlaylistHash] - playlist has been updated")
			//	// TODO: return the timestamp instead of the hash, for tidal
			//	return []byte(info.LastUpdated), true, platformBytes, nil
		}
	}
	platformBytes, _ = json.Marshal(platform)

	serializesHash, err := json.Marshal(entitySnapshot)
	if err != nil {
		log.Printf("[follow][FetchPlaylistHash] - error marshalling playlist hash: %v", err)
		return nil, false, nil, err
	}

	sanitizedCachedSnapshot := strings.Replace(string(cachedSnapshot), "\"", "", -1)

	if sanitizedCachedSnapshot == entitySnapshot {
		return serializesHash, false, nil, nil
	}
	return serializesHash, true, platformBytes, nil
}

// BuildScopesExplanation builds a string that explains the scopes that the user is granting access to
func BuildScopesExplanation(scopes []string, platform string) string {
	var spotifyScopes = map[string]string{
		spotifyauth.ScopeUserLibraryRead:           "Access your saved content",
		spotifyauth.ScopePlaylistReadPrivate:       "Access your private playlists",
		spotifyauth.ScopePlaylistReadCollaborative: "Access your collaborative playlists",
		spotifyauth.ScopeUserFollowRead:            "Access your followers and who you follow",
		spotifyauth.ScopePlaylistModifyPublic:      "Manage your public playlists",
		spotifyauth.ScopePlaylistModifyPrivate:     "Manage your private playlists",
		spotifyauth.ScopeUserTopRead:               "Read your top artists and tracks",
		spotifyauth.ScopeUserReadEmail:             "Get your real email address",
	}

	var deezScopes = map[string]string{
		"basic_access":      "Access your basic information",
		"email":             "Access your email address",
		"offline_access":    "Access your user data on Deezer anytime",
		"manage_library":    "Manage your library",
		"listening_history": "Access your listening history",
		"delete_library":    "Delete your library",
	}

	platScopes := map[string]map[string]string{
		spotify.IDENTIFIER: spotifyScopes,
		deezer.IDENTIFIER:  deezScopes,
	}

	var explanation string
	scopes = deleteEmpty(scopes)

	platformScopes := platScopes[platform]
	for i, scope := range scopes {
		if i == len(scopes)-1 {
			explanation += "and lastly, " + strings.ToLower(platformScopes[scope]) + "."
		} else if i == len(scopes)-2 {
			explanation += strings.ToLower(platformScopes[scope]) + " "
		} else {
			if i == 0 {
				explanation += platformScopes[scope] + ", "
			} else {
				explanation += strings.ToLower(platformScopes[scope]) + ", "
			}
		}
	}
	return explanation
}

// deleteEmpty removes empty strings from a slice of strings
func deleteEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}
