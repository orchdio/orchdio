package universal

import (
	"encoding/json"
	"github.com/antoniodipinto/ikisocket"
	"github.com/go-redis/redis/v8"
	"log"
	"strings"
	"zoove/blueprint"
	"zoove/services"
	"zoove/util"
)

// TrackConversion Listener listens for track conversion events and converts the track
func TrackConversion(payload *ikisocket.EventPayload, red *redis.Client) {
	message := util.GetWSMessagePayload(payload.Data, payload.Kws)
	if message == nil {
		log.Printf(`[main][Socket][TrackConversion] Unable to get message payload`)
		return
	}
	linkInfo, err := services.ExtractLinkInfo(message.Link)

	if err != nil {
		log.Println("Oops! Could not extract information about the link. Is it valid?")
		payload.Kws.Emit([]byte("Invalid link event here."))
		return
	}
	// now fetch the info
	if strings.Contains(linkInfo.Entity, "track") {
		log.Printf("\n[universal][SocketEvent][EventMessage][ConvertTrack] - Message is a track conversion\n")
		// TODO: pass the proper redis client here.
		conversion, err := ConvertTrack(linkInfo, red)
		if err != nil {
			log.Printf("\n[main][SocketEvent][EventMessage][ConvertTrack][error] - error converting track %v\n", err)
			payload.Kws.Emit([]byte("Could not convert track error event"))
			return
		}
		var convertedSpotify []blueprint.TrackSearchResult
		var convertedDeezer []blueprint.TrackSearchResult

		convertedSpotify = append(convertedSpotify, *conversion.Platforms.Spotify)
		convertedDeezer = append(convertedDeezer, *conversion.Platforms.Deezer)

		socketMessage := blueprint.SocketMessage{
			Entity: "track",
			Platforms: struct {
				Deezer  *[]blueprint.TrackSearchResult `json:"deezer"`
				Spotify *[]blueprint.TrackSearchResult `json:"spotify"`
			}(struct {
				Deezer  *[]blueprint.TrackSearchResult
				Spotify *[]blueprint.TrackSearchResult
			}{Deezer: &convertedDeezer, Spotify: &convertedSpotify}),
			Meta: nil,
		}
		conversionBytes, mErr := json.Marshal(socketMessage)
		if mErr != nil {
			log.Printf("\n[main][SocketEvent][EventMessage][ConvertTrack][error] - error converting track %v\n", mErr)
			payload.Kws.Emit([]byte("Could not convert track error event"))
			return
		}
		payload.Kws.Emit(conversionBytes)
		return
	}
}

// PlaylistConversion listens for a playlist conversion event and converts the playlist
func PlaylistConversion(payload *ikisocket.EventPayload, red *redis.Client) {
	message := util.GetWSMessagePayload(payload.Data, payload.Kws)
	if message == nil {
		return
	}

	linkInfo, err := services.ExtractLinkInfo(message.Link)
	if err != nil {
		log.Println("Oops! Could not extract information about the link. Is it valid?")
		payload.Kws.Emit([]byte("Invalid link event here."))
		return
	}

	if strings.Contains(linkInfo.Entity, "playlist") {
		playlist, err := ConvertPlaylist(linkInfo, red)
		if err != nil {
			log.Printf("\n[main][SocketEvent][EventMessage][error] - could not extract playlist")
			payload.Kws.Emit([]byte(blueprint.EEPLAYLISTCONVERSION))
			return
		}

		socketMessage := blueprint.SocketMessage{
			Entity: "playlist",
			Platforms: struct {
				Deezer  *[]blueprint.TrackSearchResult `json:"deezer"`
				Spotify *[]blueprint.TrackSearchResult `json:"spotify"`
			}{
				Deezer:  playlist.Tracks.Deezer,
				Spotify: playlist.Tracks.Spotify,
			},
			Meta: map[string]interface{}{
				"length":         playlist.Length,
				"title":          playlist.Title,
				"preview":        playlist.Preview,
				"omitted_tracks": playlist.OmittedTracks,
				"url":            playlist.URL,
			},
		}
		playlistBytes, mErr := json.Marshal(socketMessage)
		if mErr != nil {
			log.Printf("\n[main][SocketEvent][EventMessage][error] - could not extract playlist")
			payload.Kws.Emit([]byte(blueprint.EEPLAYLISTCONVERSION))
			return
		}
		payload.Kws.Emit(playlistBytes)
		return
	}
}
