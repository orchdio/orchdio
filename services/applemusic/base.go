package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-redis/redis/v8"
	"github.com/minchao/go-apple-music"
	"github.com/samber/lo"
	"github.com/vicanso/go-axios"
	"log"
	"net/url"
	"orchdio/blueprint"
	"orchdio/util"
	"os"
	"strings"
	"sync"
	"time"
)

// SearchTrackWithLink fetches a track from the ID using the link.
func SearchTrackWithLink(info *blueprint.LinkInfo, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	cacheKey := "applemusic:track:" + info.EntityID
	_, err := red.Get(context.Background(), cacheKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from cache: %v\n", err)
		return nil, err
	}

	log.Printf("[services][applemusic][SearchTrackWithLink] Track not found in cache, fetching from Apple Music: %v\n", info.EntityID)
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY")}
	client := applemusic.NewClient(tp.Client())
	tracks, response, err := client.Catalog.GetSong(context.Background(), "us", info.EntityID, nil)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}
	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}

	if len(tracks.Data) == 0 {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error fetching track from Apple Music: %v\n", err)
		return nil, blueprint.ENORESULT
	}

	previewURL := ""
	t := tracks.Data[0]
	previews := *t.Attributes.Previews
	if len(previews) > 0 {
		previewURL = previews[0].Url
	}
	//if t.Attributes.Previews != nil {
	//
	//}

	// replace the cover url with the 150x150 version using regex. the original url is in the format of https://is5-ssl.mzstatic.com/image/thumb/Music124/v4/f8/0d/17/f80d17a1-c1c8-1f3d-6797-8d7e9a98539b/8720205201379.png/{w}x{h}bb.jpg
	// where {w} and {h} are the width and height of the image. we replace it with 150x150
	coverURL := strings.ReplaceAll(t.Attributes.Artwork.URL, "{w}x{h}bb.jpg", "150x150bb.jpg")

	track := &blueprint.TrackSearchResult{
		URL:           info.TargetLink,
		Artists:       []string{t.Attributes.ArtistName},
		Released:      t.Attributes.ReleaseDate,
		Duration:      util.GetFormattedDuration(int(t.Attributes.DurationInMillis / 1000)),
		DurationMilli: int(t.Attributes.DurationInMillis),
		Explicit:      false,
		Title:         t.Attributes.Name,
		Preview:       previewURL,
		Album:         t.Attributes.AlbumName,
		ID:            t.Id,
		Cover:         coverURL,
	}

	serializeTrack, err := json.Marshal(track)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error serializing track: %v\n", err)
		return nil, err
	}
	err = red.Set(context.Background(), cacheKey, serializeTrack, time.Hour*24).Err()
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithLink] Error caching track: %v\n", err)
		return nil, err
	}
	return track, nil

}

// SearchTrackWithTitle searches for a track using the query.
func SearchTrackWithTitle(title, artiste string, red *redis.Client) (*blueprint.TrackSearchResult, error) {
	cleanedArtiste := fmt.Sprintf("applemusic-%s-%s", util.NormalizeString(artiste), title)
	log.Printf("Apple music: Searching with stripped artiste: %s. Original artiste: %s", cleanedArtiste, artiste)
	if red.Exists(context.Background(), cleanedArtiste).Val() == 1 {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Track found in cache: %v\n", cleanedArtiste)
		track, err := red.Get(context.Background(), cleanedArtiste).Result()
		if err != nil {
			log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from cache: %v\n", err)
			return nil, err
		}
		var result *blueprint.TrackSearchResult
		err = json.Unmarshal([]byte(track), &result)
		if err != nil {
			log.Printf("[services][applemusic][SearchTrackWithTitle] Error unmarshalling track from cache: %v\n", err)
			return nil, err
		}
		return result, nil
	}

	log.Printf("[services][applemusic][SearchTrackWithTitle] Track not found in cache, fetching from Apple Music: %v\n", title)
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY")}
	client := applemusic.NewClient(tp.Client())
	results, response, err := client.Catalog.Search(context.Background(), "us", &applemusic.SearchOptions{
		Term: fmt.Sprintf("%s+%s", artiste, title),
	})
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}

	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, err
	}

	if results.Results.Songs == nil {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, blueprint.ENORESULT
	}

	if len(results.Results.Songs.Data) == 0 {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error fetching track from Apple Music: %v\n", err)
		return nil, blueprint.ENORESULT
	}

	t := results.Results.Songs.Data[0]
	previewURL := ""
	previews := *t.Attributes.Previews
	if len(previews) > 0 {
		previewURL = previews[0].Url
	}

	coverURL := strings.ReplaceAll(t.Attributes.Artwork.URL, "{w}x{h}bb.jpg", "150x150bb.jpg")

	track := &blueprint.TrackSearchResult{
		Artists:       []string{t.Attributes.ArtistName},
		Released:      t.Attributes.ReleaseDate,
		Duration:      util.GetFormattedDuration(int(t.Attributes.DurationInMillis / 1000)),
		DurationMilli: int(t.Attributes.DurationInMillis),
		Explicit:      false, // apple doesnt seem to return explicit content value for songs
		Title:         t.Attributes.Name,
		Preview:       previewURL,
		Album:         t.Attributes.AlbumName,
		ID:            t.Id,
		Cover:         coverURL,
		URL:           t.Attributes.URL,
	}
	serializedTrack, err := json.Marshal(track)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitle] Error serializing track: %v\n", err)
		return nil, err
	}

	if lo.Contains(track.Artists, artiste) {
		err = red.MSet(context.Background(), map[string]interface{}{
			cleanedArtiste: string(serializedTrack),
		}).Err()
		if err != nil {
			log.Printf("\n[controllers][platforms][deezer][SearchTrackWithTitle] error caching track - %v\n", err)
		} else {
			log.Printf("\n[controllers][platforms][applemusic][SearchTrackWithTitle] Track %s has been cached\n", track.Title)
		}
	}

	return track, nil
}

// SearchTrackWithTitleChan searches for tracks using title and artistes but do so asynchronously.
func SearchTrackWithTitleChan(title, artiste string, c chan *blueprint.TrackSearchResult, wg *sync.WaitGroup, red *redis.Client) {
	track, err := SearchTrackWithTitle(title, artiste, red)
	if err != nil {
		log.Printf("[services][applemusic][SearchTrackWithTitleChan] Error fetching track: %v\n", err)
		defer wg.Done()
		c <- nil
		wg.Add(1)
		return
	}
	defer wg.Done()
	c <- track
	wg.Add(1)
	return
}

// FetchTracks asynchronously fetches a list of tracks using the track id
func FetchTracks(tracks []blueprint.PlatformSearchTrack, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks, error) {
	var omittedTracks []blueprint.OmittedTracks
	var results []blueprint.TrackSearchResult
	var ch = make(chan *blueprint.TrackSearchResult, len(tracks))
	var wg sync.WaitGroup
	for _, track := range tracks {
		identifier := fmt.Sprintf("applemusic-%s-%s", track.Artistes[0], track.Title)
		if red.Exists(context.Background(), identifier).Val() == 1 {
			var deserializedTrack *blueprint.TrackSearchResult
			track, err := red.Get(context.Background(), identifier).Result()
			if err != nil {
				log.Printf("[services][applemusic][FetchTracks] Error fetching track from cache: %v\n", err)
				return nil, nil, err
			}
			err = json.Unmarshal([]byte(track), &deserializedTrack)
			if err != nil {
				log.Printf("[services][applemusic][FetchTracks] Error unmarshalling track from cache: %v\n", err)
				return nil, nil, err
			}
			results = append(results, *deserializedTrack)
			continue
		}
		go SearchTrackWithTitleChan(track.Title, track.Artistes[0], ch, &wg, red)
		chTracks := <-ch
		if chTracks == nil {
			omittedTracks = append(omittedTracks, blueprint.OmittedTracks{
				Artistes: []string{track.Artistes[0]},
				Title:    track.Title,
			})
			continue
		}
		results = append(results, *chTracks)
	}
	wg.Wait()
	return &results, &omittedTracks, nil
}

// FetchPlaylistTrackList fetches a list of tracks for a playlist and saves the last modified date to redis
func FetchPlaylistTrackList(id string, red *redis.Client) (*blueprint.PlaylistSearchResult, error) {
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY")}
	client := applemusic.NewClient(tp.Client())
	playlist := &blueprint.PlaylistSearchResult{}
	duration := 0

	var tracks []blueprint.TrackSearchResult

	//for page = 1; ; page++ {
	log.Printf("[services][applemusic][FetchPlaylistTrackList] Fetching playlist tracks: %v\n", id)
	//id = strings.ReplaceAll(id, "/", "")
	playlistId := strings.ReplaceAll(id, "/", "")
	log.Printf("[services][applemusic][FetchPlaylistTrackList] Playlist id: %v\n", playlistId)
	results, response, err := client.Catalog.GetPlaylist(context.Background(), "us", playlistId, nil)

	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistTrackList][error] - could not fetch playlist tracks:")
		spew.Dump(response.StatusCode)
		return nil, err
	}

	if response.StatusCode != 200 {
		log.Printf("[services][applemusic][FetchPlaylistTrackList][GetPlaylist] Status - %v could not fetch playlist tracks: %v\n", response.StatusCode, err)
		return nil, blueprint.EUNKNOWN
	}

	if len(results.Data) == 0 {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] result data is empty. Could not fetch playlist tracks: %v\n", err)
		return nil, err
	}

	playlistData := results.Data[0]

	for _, t := range playlistData.Relationships.Tracks.Data {
		tr, err := t.Parse()
		track := tr.(*applemusic.Song)
		if err != nil {
			log.Printf("[services][applemusic][FetchPlaylistTrackList] Error parsing track: %v\n", err)
			return nil, err
		}

		previewURL := ""
		var previewStruct []struct {
			Url string `json:"url"`
		}

		trackAttr := track.Attributes
		duration += int(track.Attributes.DurationInMillis)
		tribute := trackAttr.Previews

		if tribute != nil {
			r, err := json.Marshal(tribute)
			if err != nil {
				log.Printf("[services][applemusic][FetchPlaylistTrackList] Error serializing preview url: %v\n", err)
				return nil, err
			}
			err = json.Unmarshal(r, &previewStruct)
			if err != nil {
				log.Printf("[services][applemusic][FetchPlaylistTrackList] Error deserializing preview url: %v\n", err)
				return nil, err
			}
			previewURL = previewStruct[0].Url
		}

		cover := ""
		if playlistData.Attributes.Artwork != nil {
			cover = strings.ReplaceAll(playlistData.Attributes.Artwork.URL, "{w}x{h}bb.jpg", "300x300bb.jpg")
		}

		tracks = append(tracks, blueprint.TrackSearchResult{
			URL:           trackAttr.URL,
			Artists:       []string{trackAttr.ArtistName},
			Released:      trackAttr.ReleaseDate,
			Duration:      util.GetFormattedDuration(int(trackAttr.DurationInMillis / 1000)),
			DurationMilli: int(trackAttr.DurationInMillis),
			Explicit:      false,
			Title:         trackAttr.Name,
			Preview:       previewURL,
			Album:         trackAttr.AlbumName,
			ID:            track.Id,
			Cover:         cover,
		})
	}
	playlistCover := ""
	if playlistData.Attributes.Artwork != nil {
		playlistCover = strings.ReplaceAll(playlistData.Attributes.Artwork.URL, "{w}x{h}cc.jpg", "300x300cc.jpg")
	}

	// in order to add support for paginated playlists, we need to somehow get the total number of tracks in the playlist and or get the next page of tracks
	// if there is next page, we keep fetching (in a loop) until we get all the tracks and break out of the loop. This is similar to how we fetch paginated playlists
	// for spotify. However, the difference is that the library we use for Apple Music does not support pagination. So in order to get paginated tracks, for now, we first make
	// a GetPlaylist call to get playlist tracks (and data) and first 100 tracks (if playlist is more than 100 tracks). Then we make another call that fetches the tracks for the playlists, offset by 100 or length of the first call.
	// but this time we specify the limit of 700. Downside is that we may not have playlists that have less than 300 tracks. THIS IS AN EDGE/UNLIKELY CASE. GARDEN THIS FOR NOW.
	// TODO: refactor this when patch to library is implemented and merged (or switch to implementation in our fork) until the PR from the fork is merged.
	p := url.Values{}
	p.Add("limit", "300")
	p.Add("offset", fmt.Sprintf("%d", len(playlistData.Relationships.Tracks.Data)))
	ax := axios.NewInstance(&axios.InstanceConfig{
		BaseURL: "https://api.music.apple.com",
		Headers: map[string][]string{
			"Authorization": {fmt.Sprintf("Bearer %s", os.Getenv("APPLE_MUSIC_API_KEY"))},
		},
	})

	log.Printf("[services][applemusic][FetchPlaylistTrackList] Request configs ")

	_allTracksRes, tErr := ax.Get(fmt.Sprintf("/v1/catalog/us/playlists/%s/tracks", playlistId), p)

	if tErr != nil {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error fetching playlist tracks from apple music %v\n", tErr.Error())
		return nil, tErr
	}

	// if the response is a 404 and the length of the tracks we got earlier is 0, that means we really cant get the playlist tracks
	if _allTracksRes.Status == 404 {
		if len(tracks) == 0 {
			log.Printf("[services][applemusic][FetchPlaylistTrackList] Could not fetch playlist tracks: %v\n", err)
			return nil, blueprint.EUNKNOWN
		}
		result := blueprint.PlaylistSearchResult{
			Title:   playlistData.Attributes.Name,
			Tracks:  tracks,
			URL:     playlistData.Attributes.URL,
			Length:  util.GetFormattedDuration(duration / 1000),
			Preview: "",
			Owner:   playlistData.Attributes.CuratorName,
			Cover:   playlistCover,
		}
		return &result, nil
	}

	if _allTracksRes.Status != 200 {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error fetching playlist tracks: %v\n", string(_allTracksRes.Data))
		log.Printf("original req url %v", _allTracksRes.Request.URL)
		return nil, err
	}

	var allTracksRes UnlimitedPlaylist
	err = json.Unmarshal(_allTracksRes.Data, &allTracksRes)

	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error fetching playlist tracks: %v\n", err)
		return nil, err
	}

	for _, d := range allTracksRes.Data {
		singleTrack := d
		previewURL := ""
		tribute := d.Attributes.Previews
		if len(tribute) > 0 {
			previewURL = tribute[0].Url
		}
		duration += d.Attributes.DurationInMillis
		coverURL := ""
		if singleTrack.Attributes.Artwork.Height > 0 {
			coverURL = singleTrack.Attributes.Artwork.Url
		}

		coverURL = strings.ReplaceAll(singleTrack.Attributes.Artwork.Url, "{w}x{h}bb.jpg", "300x300bb.jpg")
		track := &blueprint.TrackSearchResult{
			URL:           singleTrack.Attributes.Url,
			Artists:       []string{singleTrack.Attributes.ArtistName},
			Released:      singleTrack.Attributes.ReleaseDate,
			Duration:      util.GetFormattedDuration(singleTrack.Attributes.DurationInMillis / 1000),
			DurationMilli: singleTrack.Attributes.DurationInMillis,
			Explicit:      false,
			Title:         singleTrack.Attributes.Name,
			Preview:       previewURL,
			Album:         singleTrack.Attributes.AlbumName,
			ID:            singleTrack.Id,
			Cover:         coverURL,
		}
		tracks = append(tracks, *track)
	}

	// save the last updated at to redis under the key "applemusic:playlist:<id>"
	err = red.Set(context.Background(), fmt.Sprintf("applemusic:playlist:%s", id), playlistData.Attributes.LastModifiedDate, 0).Err()
	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistTrackList] Error setting last updated at: %v\n", err)
		return nil, err
	}

	playlist = &blueprint.PlaylistSearchResult{
		Title:   playlistData.Attributes.Name,
		Tracks:  tracks,
		URL:     playlistData.Attributes.URL,
		Length:  util.GetFormattedDuration(duration / 1000),
		Preview: "",
		Owner:   playlistData.Attributes.CuratorName,
		Cover:   playlistCover,
	}

	log.Printf("[services][applemusic][FetchPlaylistTrackList] Done fetching playlist tracks: %v\n", playlist)
	return playlist, nil
}

// FetchPlaylistSearchResult fetches the tracks for a playlist based on the search result
// from another platform
func FetchPlaylistSearchResult(p *blueprint.PlaylistSearchResult, red *redis.Client) (*[]blueprint.TrackSearchResult, *[]blueprint.OmittedTracks) {
	var trackSearch []blueprint.PlatformSearchTrack
	for _, track := range p.Tracks {
		trackSearch = append(trackSearch, blueprint.PlatformSearchTrack{
			Title:    track.Title,
			Artistes: track.Artists,
		})
	}
	tracks, omittedTracks, err := FetchTracks(trackSearch, red)
	if err != nil {
		log.Printf("[services][applemusic][FetchPlaylistSearchResult] Error fetching tracks: %v\n", err)
		return nil, nil
	}
	return tracks, omittedTracks
}

func CreateNewPlaylist(title, description, musicToken string, tracks []string) ([]byte, error) {
	log.Printf("[services][applemusic][CreateNewPlaylist] Creating new playlist: %v\n", title)
	log.Printf("User Applemusic token is: %v\n", musicToken)
	tp := applemusic.Transport{Token: os.Getenv("APPLE_MUSIC_API_KEY"), MusicUserToken: musicToken}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[services][applemusic][CreateNewPlaylist] TP client creation here %v\n", r)
		}
	}()

	client := applemusic.NewClient(tp.Client())
	defer func() {
		if err := recover(); err != nil {
			log.Printf("[services][applemusic][CreateNewPlaylist] Error creating playlist: %v\n", err)
			return
		}
	}()
	playlist, response, err := client.Me.CreateLibraryPlaylist(context.Background(), applemusic.CreateLibraryPlaylist{
		Attributes: applemusic.CreateLibraryPlaylistAttributes{
			Name:        title,
			Description: description,
		},
		Relationships: nil,
	}, nil)

	if err != nil {
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - could not create new playlist: %v\n", err)
	}

	if response.Response.StatusCode == 403 {
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - unauthorized: %v\n", err)
		return nil, blueprint.EFORBIDDEN
	}

	if response.Response.StatusCode == 401 {
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - unauthorized: %v\n", err)
		return nil, blueprint.EUNAUTHORIZED
	}

	if response.Response.StatusCode == 400 {
		log.Printf("[services][applemusic][CreateNewPlaylist][error] - bad request: %v\n", err)
		return nil, blueprint.EBADREQUEST
	}

	// add the tracks to the playlist
	var playlistTracks []applemusic.CreateLibraryPlaylistTrack
	for _, track := range tracks {
		playlistTracks = append(playlistTracks, applemusic.CreateLibraryPlaylistTrack{
			Id:   track,
			Type: "songs",
		})
	}

	playlistData := applemusic.CreateLibraryPlaylistTrackData{
		Data: playlistTracks,
	}

	response, err = client.Me.AddLibraryTracksToPlaylist(context.Background(), playlist.Data[0].Id, playlistData)
	if err != nil {
		log.Printf("[services][applemusic][CreateNewPlaylist] Error adding tracks to playlist: %v\n", err)
		return nil, err
	}

	if response.StatusCode >= 400 {
		log.Printf("[services][applemusic][CreateNewPlaylist] Error adding tracks to playlist: %v\n", err)
		return nil, err
	}

	log.Printf("[services][applemusic][CreateNewPlaylist] Successfully created playlist: %v\n", playlist.Data[0].Href)

	return []byte(fmt.Sprintf("https://music.apple.com/us/playlist/%s", playlist.Data[0].Id)), nil

}
