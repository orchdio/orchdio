package platforms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/teris-io/shortid"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/services/deezer"
	"orchdio/universal"
	"orchdio/util"
	"os"
	"strings"
)

// Platforms represents the structure for the platforms
type Platforms struct {
	Redis *redis.Client
	DB    *sqlx.DB
}

func NewPlatform(r *redis.Client, db *sqlx.DB) *Platforms {
	return &Platforms{Redis: r, DB: db}
}

// ConvertTrack returns the link to a track on several platforms
func (p *Platforms) ConvertTrack(ctx *fiber.Ctx) error {
	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)

	// make sure we're actually handling for track alone, not playlist.
	if !strings.Contains(linkInfo.Entity, "track") {
		log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", "Not a track URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Not a track entity")
	}

	conversion, err := universal.ConvertTrack(linkInfo, p.Redis)
	if err != nil {
		if err == blueprint.ENOTIMPLEMENTED {
			log.Printf("\n[controllers][platforms][deezer][ConvertTrack] error - %v\n", "Not implemented")
			return util.ErrorResponse(ctx, http.StatusNotImplemented, "Not implemented")
		}

		log.Printf("\n[controllers][platforms][base][ConvertTrack] - Could not convert track")
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	log.Printf("\n[controllers][platforms][ConvertTrack] - converted %v track with URL %v\n", linkInfo.Entity, linkInfo.TargetLink)

	// HACK: insert a new task in the DB directly and return the ID as part of the
	// conversion response. We are saving directly because for playlists, we run them in asynq job queue
	// and for tracks we don't. we use this task info to implement being able to share a conversion page
	// (for example on zoove) with a link because it carries data unique to the operation.
	database := db.NewDB{DB: p.DB}
	uniqueId, _ := uuid.NewUUID()
	// generate a URL friendly short ID. this is what we're going to send to the API response
	// and user can use it in the conversion URL.
	const format = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-" // 0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-
	sid, err := shortid.New(1, format, 2342)
	if err != nil {
		log.Printf("\n[controllers][platforms][ConvertTrack] - could not generate short id %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	uID, _ := sid.Generate()

	serialized, err := json.Marshal(conversion)

	// add the task ID to the conversion response
	conversion.ShortURL = uID

	log.Printf("\n[controllers][platforms][ConvertTrack] - serialized conversion %v\n", string(serialized))

	if err != nil {
		log.Printf("[db][CreateTrackTaskRecord] error serializing result. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	_, err = database.CreateTrackTaskRecord(uniqueId.String(), uID, linkInfo.EntityID, serialized)
	if err != nil {
		log.Printf("\n[controllers][platforms][ConvertTrack] - Could not create task record")
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	return util.SuccessResponse(ctx, http.StatusOK, conversion)
}

// ConvertPlaylist retrieves info about a playlist from various platforms.
func (p *Platforms) ConvertPlaylist(ctx *fiber.Ctx) error {
	// first, we want to fetch the information on the link

	linkInfo := ctx.Locals("linkInfo").(*blueprint.LinkInfo)

	// make sure we're actually handling for track alone, not playlist.
	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("\n[controllers][platforms][ConvertTrack] error - %v\n", "Not a playlist URL")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Not a playlist entity")
	}

	convertedPlaylist, err := universal.ConvertPlaylist(linkInfo, p.Redis)

	if err != nil {
		if err == blueprint.ENOTIMPLEMENTED {
			return util.ErrorResponse(ctx, http.StatusNotImplemented, err)
		}
		log.Printf("\n[controllers][platforms][ConvertPlaylist][error] could not convert playlist %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	return util.SuccessResponse(ctx, http.StatusOK, convertedPlaylist)
}

// AddPlaylistToAccount adds a playlist to a user's account
func (p *Platforms) AddPlaylistToAccount(ctx *fiber.Ctx) error {
	// get the platform they want to add the playlist to
	platform := ctx.Params("platform")
	if platform == "" {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No platform in context")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Platform not found")
	}

	//// get the playlist ID
	//playlistID := ctx.Params("playlistId")
	//if playlistID == "" {
	//	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No playlist ID in context")
	//	return util.ErrorResponse(ctx, http.StatusBadRequest, "Playlist ID not found")
	//}

	// get the playlist creation body
	var createBodyData = struct {
		User   string   `json:"user"`
		Title  string   `json:"title"`
		Tracks []string `json:"tracks"`
	}{}

	err := ctx.BodyParser(&createBodyData)
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
	}

	if len(createBodyData.Tracks) == 0 {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No tracks in playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "No tracks to insert into playlist. please add tracks to the playlist")
	}

	if createBodyData.Title == "" {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "No title in playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "No title to insert into playlist. please add title to the playlist")
	}

	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] incoming body - %v\n", createBodyData)

	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - got user %v\n", createBodyData.User)

	// find the user in the database
	database := db.NewDB{DB: p.DB}
	user, err := database.FindUserByEmail(createBodyData.User)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", "User not found")
			return util.ErrorResponse(ctx, http.StatusNotFound, "User not found")
		}
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	// get the user's access token
	t, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error decrypting user refresh token - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - user refresh token %v\n", string(t))
	//title := fmt.Sprintf("Zoove playlist: %s", createBodyData.Title)
	description := "powered by Orchdio. https://orchdio.com"
	playlistlink := ""
	switch platform {
	case "spotify":
		httpClient := spotifyauth.New().Client(context.Background(), &oauth2.Token{
			RefreshToken: string(t),
		})

		client := spotify.New(httpClient)
		createdPlaylist, err := client.CreatePlaylistForUser(context.Background(), user.PlatformID, createBodyData.Title, description, true, false)

		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error getting profile - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}

		var trackIds []spotify.ID
		for _, track := range createBodyData.Tracks {
			if track != "" {
				trackIds = append(trackIds, spotify.ID(track))
			}
		}

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - track ids %v\n", len(trackIds))
		// update playlist with the tracks
		updated, err := client.AddTracksToPlaylist(context.Background(), createdPlaylist.ID, trackIds...)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error getting profile - %v\n", err)
			if err.Error() == "No tracks specified." {
				return util.ErrorResponse(ctx, http.StatusBadRequest, err.Error())
			}
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err.Error())
		}

		playlistlink = createdPlaylist.ExternalURLs["spotify"]

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", updated)

	case "deezer":
		id, err := deezer.CreateNewPlaylist(createBodyData.Title, user.PlatformID, string(t), createBodyData.Tracks)
		if err != nil {
			log.Printf("\n[controllers][platforms][AddPlaylistToAccount] error creating new playlist - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
		}

		playlistlink = fmt.Sprintf("https://www.deezer.com/en/playlist/%s", id)

		log.Printf("\n[controllers][platforms][AddPlaylistToAccount] - created playlist %v\n", createBodyData.Title)
		// get the user, to see if our token is valid
	}

	//c := map[string]interface{}{
	//	"url": playlistlink,
	//}

	return util.SuccessResponse(ctx, http.StatusCreated, playlistlink)

}
