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
func TrackConversion(payload *ikisocket.EventPayload) {
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
			log.Printf("\n[main][SocketEvent][EventMessage][error] %v - could not extract playlist", err.Error())
			// note: feels hacky, but it should work. Not sure how the error looks (is it "Not found." or "Not Found" or "Not Found")
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				log.Printf("\n[main][SocketEvent][EventMessage][error] %v - Playlist not found. It might be private.")
				errorResponse := util.SerializeWebsocketMessage(&blueprint.WebsocketErrorMessage{
					Message:   "Playlist not found",
					Error:     blueprint.ENORESULT.Error(),
					EventName: "error",
				})
				payload.Kws.Emit(errorResponse)
				return
			}

			errorResponse := util.SerializeWebsocketMessage(&blueprint.WebsocketErrorMessage{
				Message:   "Could not convert playlist",
				EventName: "error",
				Error:     blueprint.EGENERAL.Error(),
			})
			payload.Kws.Emit(errorResponse)
			return
		}

		serializeResponse := util.SerializeWebsocketMessage(&blueprint.WebsocketMessage{
			Event:   "playlist",
			Payload: playlist,
		})
		payload.Kws.Emit(serializeResponse)
		return
	}
}
