package account

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/iris-contrib/middleware/jwt"
	"github.com/kataras/iris/v12"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"zoove/queries"
	"zoove/services/deezer"
	"zoove/services/spotify"
	"zoove/types"
	"zoove/util"
)

type UserController struct {
	DB *sql.DB
}

func NewUserController(db *sql.DB) *UserController {
	return &UserController{
		DB: db,
	}
}
// RedirectAuth returns the authorization URL when a user wants to connect their platform
func (c *UserController) RedirectAuth(ctx iris.Context) {
var uniqueID, _ = uuid.NewUUID()
dz := &deezer.Deezer{
	ClientID: os.Getenv("DEEZER_ID"),
	ClientSecret: os.Getenv("DEEZER_SECRET"),
	RedirectURI: os.Getenv("DEEZER_REDIRECT_URI"),
}
	// first, get the platform that the user wants to log in/signup with
	platform := strings.ToLower(ctx.Params().Get("platform"))

	if platform == "spotify" {
		// now do spotify things here.
		url := spotify.FetchAuthURL(uniqueID.String())
		if url == nil {
			log.Printf("[account][auth] error - Could return URL for user")
			ctx.StopWithJSON(http.StatusInternalServerError, types.ControllerError{
				Message: "An error occurred while getting spotify auth URL for user",
				Status: http.StatusInternalServerError,
			})
			return
		}

		log.Printf("\nState is: %s\n", uniqueID.String())
		ctx.StopWithJSON(http.StatusOK, types.ControllerResult{
			Message: "Redirect URL fetched successfully",
			Status: http.StatusOK,
			Data: map[string]string{
				"url": string(url),
			},
		})
		return
	}

	if platform == "deezer" {
		// FIXME: handle errors properly here if any
		url := dz.FetchAuthURL()
		ctx.StopWithJSON(http.StatusOK, types.ControllerResult{
			Message: "Redirect URL fetched successfully",
			Status: http.StatusOK,
			Data: map[string]string{
				"url": url,
			},
		})
		return
	}

	// no other platforms for now
	// TODO: handle for other platforms
	ctx.StopWithJSON(http.StatusNotImplemented, types.ControllerError{
		Message: "Other platforms havent been implemented yet",
		Status: http.StatusNotImplemented,
	})
	return
}

func (c *UserController) AuthSpotifyUser(ctx iris.Context) {
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	state := ctx.URLParam("state")
	if state == "" {
		ctx.StopWithJSON(http.StatusBadRequest, types.ControllerError{
			Message: "State is not present in the URL",
			Status: http.StatusBadRequest,
		})
		return
	}

	log.Printf("\nState is: %s\n", state)

	client, refreshToken := spotify.CompleteUserAuth(context.Background(), ctx.Request())
	user, err := client.CurrentUser(context.Background())

	encrypedToken, err := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
	if err != nil {
		log.Printf("[account][auth][spotify] error - Error encrypting secretToken %v\n", err)
		ctx.StopWithJSON(http.StatusInternalServerError, types.ControllerError{
			Message: "Error encrypting the secret token",
			Status: http.StatusInternalServerError,
			Error: err,
		})
		return
	}

	if err != nil {
		log.Println("Error retrieving user")
	}

	query := queries.CreateUserQuery
	_, err = c.DB.Exec(query,
		user.Email,
		encrypedToken,
		user.ID,
		user.DisplayName,
		user.ExternalURLs["spotify"], // FIXME: handle this properly
		spotify.IDENTIFIER,
		encrypedToken,
	)
	if err != nil {
		log.Printf("\n[user][controller][AuthUser] Error executing query: %v\n", err)
	}
	log.Printf("\n[user][controller][AuthUser] Method - User with the email %s just signed up or logged in with their Spotify account.\n", user.Email)
	// create a jwt
	token := jwt.NewTokenWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"platform": "deezer",
		"platform_id": user.ID,
		"email": user.Email,
	})
	signedToken, jwtErr := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if jwtErr != nil {
		log.Printf("\n[user][controller][AuthUser] Method - An error occured while signing token: %v\n", jwtErr)
		ctx.StopWithJSON(http.StatusInternalServerError, types.ControllerError{
			Message: "Error authing user",
			Status: http.StatusInternalServerError,
			Error: jwtErr,
		})
		return
	}
	ctx.StopWithJSON(http.StatusOK, types.ControllerResult{
		Message: "here is your response",
		Status: http.StatusOK,
		Data: map[string]string{
			"token": signedToken,
		},
	})
	return
}

func (c *UserController) AuthDeezerUser(ctx iris.Context) {
	code := ctx.URLParam("code")
	dz := &deezer.Deezer{
		ClientID: os.Getenv("DEEZER_ID"),
		ClientSecret: os.Getenv("DEEZER_SECRET"),
		RedirectURI: os.Getenv("DEEZER_REDIRECT_URI"),
	}
	if code == "" {
		log.Printf("\n[user][controller][AuthDeezerUser] method - Error authorizing deezer. - No auth code\n")
		ctx.StopWithJSON(http.StatusBadRequest, types.ControllerError{
			Message: "State is not present in the URL",
			Status: http.StatusBadRequest,
		})
		return
	}

	authToken := dz.FetchAccessToken(code)
	if authToken == nil {
		log.Printf("\n[user][controller][AuthDeezerUser] method - could not fetch the token")
	}

	dzUser, err := dz.CompleteUserAuth(authToken)
	if err != nil {
		log.Printf("\n[user][controller][AuthDeezerUser] method - error retrieving user: %v \n", err)
		ctx.StopWithJSON(http.StatusInternalServerError, types.ControllerError{
			Message: "Error retrieving deezer user",
			Status: http.StatusInternalServerError,
			Error: err,
		})
	}

	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	encrypedToken, err := util.Encrypt(authToken, []byte(encryptionSecretKey))
	if err != nil {
		log.Printf("[account][auth][deezer] error - Error encrypting acesss token %v\n", err)
		ctx.StopWithJSON(http.StatusInternalServerError, types.ControllerError{
			Message: "Error encrypting the secret token",
			Status: http.StatusInternalServerError,
			Error: err,
		})
		return
	}
	_, err = c.DB.Exec(queries.CreateUserQuery,
		dzUser.Email,
		// it seems to work when i dont convert to string.
		// postgres still saves as string but I am not taking chances
		strconv.Itoa(dzUser.ID),
		dzUser.Name,
		dzUser.Link,
		deezer.IDENTIFIER,
		encrypedToken,
		)
	if err != nil {
		log.Printf("\n[user][controller][AuthUser] Error executing query: %v\n", err)
		ctx.StopWithJSON(http.StatusInternalServerError, types.ControllerError{
			Message: "Error creating a new user",
			Status: http.StatusInternalServerError,
			Error: err,
		})
		return
	}
	log.Printf("\n[user][controller][AuthUser] Method - User with the email: %s just signed up or logged in with their Deezer account.\n", dzUser.Email)

	// create a jwt
	token := jwt.NewTokenWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"platform": "deezer",
		"platform_id": strconv.Itoa(dzUser.ID),
		"email": dzUser.Email,
	})
	signedToken, jwtErr := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if jwtErr != nil {
		log.Printf("\n[user][controller][AuthUser] Method - An error occured while signing token: %v\n", jwtErr)
		ctx.StopWithJSON(http.StatusInternalServerError, types.ControllerError{
			Message: "Error authing user",
			Status: http.StatusInternalServerError,
			Error: jwtErr,
		})
		return
	}
	ctx.StopWithJSON(http.StatusOK, types.ControllerResult{
		Message: "here is your response",
		Status: http.StatusOK,
		Data: map[string]string{
			"token": signedToken,
		},
	})
	return
}