package universal

import (
	"encoding/json"
	"github.com/antoniodipinto/ikisocket"
	"github.com/go-redis/redis/v8"
	"log"
	"strings"
	"zoove/blueprint"
	"zoove/services"
)

// TrackConversion Listener listens for track conversion events and converts the track
func TrackConversion(payload *ikisocket.EventPayload) {
	var message blueprint.Message
	err := json.Unmarshal(payload.Data, &message)
	if err != nil {
		log.Printf("\n[main][SocketEvent][EventMessage] - error deserializing incoming message %v\n", err)
		payload.Kws.Emit([]byte(blueprint.EEDESERIALIZE))
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
		log.Printf("\n[universal][SocketEvent][EventMessage][ConvertTrack - Message is a track conversion\n")
		// TODO: pass the proper redis client here.
		conversion, err := ConvertTrack(linkInfo, nil)
		if err != nil {
			log.Printf("\n[main][SocketEvent][EventMessage][ConvertTrack][error] - error converting track %v\n", err)
			payload.Kws.Emit([]byte("Could not convert track error event"))
			return
		}
		conversionBytes, mErr := json.Marshal(conversion)
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
	var message blueprint.Message
	err := json.Unmarshal(payload.Data, &message)
	if err != nil {
		log.Printf("\n[main][SocketEvent][EventMessage] - error deserializing incoming message %v\n", err)
		payload.Kws.Emit([]byte(blueprint.EEDESERIALIZE))
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
		playlistBytes, mErr := json.Marshal(playlist)
		if mErr != nil {
			log.Printf("\n[main][SocketEvent][EventMessage][error] - could not extract playlist")
			payload.Kws.Emit([]byte(blueprint.EEPLAYLISTCONVERSION))
			return
		}
		payload.Kws.Emit(playlistBytes)
		return
	}
}
