package account

import (
	"context"
	"database/sql"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
	"strings"
)

type UserController struct {
	DB *sql.DB
}

func NewUserController(db *sql.DB) *UserController {
	return &UserController{
		DB: db,
	}
}

// RedirectAuth returns the authorization URL when a user wants to connect their platform.
func (c *UserController) RedirectAuth(ctx *fiber.Ctx) error {
	var uniqueID, _ = uuid.NewUUID()
	dz := &deezer.Deezer{
		ClientID:     os.Getenv("DEEZER_ID"),
		ClientSecret: os.Getenv("DEEZER_SECRET"),
		RedirectURI:  os.Getenv("DEEZER_REDIRECT_URI"),
	}

	platform := strings.ToLower(ctx.Params("platform"))

	if platform == "spotify" {
		// now do spotify things here.
		url := spotify.FetchAuthURL(uniqueID.String())
		if url == nil {
			log.Printf("[account][auth] error - Could return URL for user")
			return util.ErrorResponse(ctx, http.StatusOK, "Error creating auth URL")
		}

		return util.SuccessResponse(ctx, http.StatusOK, fiber.Map{
			"url": string(url),
		})
	}

	if platform == "deezer" {
		url := dz.FetchAuthURL()
		return util.SuccessResponse(ctx, http.StatusOK, url)
	}
	return util.ErrorResponse(ctx, http.StatusNotImplemented, "Other Platforms have not been implemented")
}

// AuthSpotifyUser authorizes a user with spotify account. It generates a JWT token for
// a new user
func (c *UserController) AuthSpotifyUser(ctx *fiber.Ctx) error {
	var uniqueID, _ = uuid.NewUUID()
	state := ctx.Query("state")
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "State is not present")
	}

	// create a new net/http instance since *fasthttp.Request() cannot be passed
	r, err := http.NewRequest("GET", string(ctx.Request().RequestURI()), nil)

	if err != nil {
		log.Printf("[controllers][account][user] Error - error creating a new http request - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	client, refreshToken := spotify.CompleteUserAuth(context.Background(), r)
	_, encErr := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
	if encErr != nil {
		log.Printf("\n[controllers][account][user] Error - could not encrypt refreshToken - %v\n", encErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, encErr)
	}

	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Printf("\n[controllers][account][user] Error - could not fetch current spotify user- %v\n", encErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	log.Printf("%v", user)

	query := queries.CreateUserQuery
	_, dbErr := c.DB.Exec(query,
		user.Email,
		user.DisplayName,
		uniqueID,
	)

	if dbErr != nil {
		log.Printf("\n[controller][account][user] : [AuthUser] Error executing query: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	log.Printf("\n[user][controller][AuthUser] Method - User with the email %s just signed up or logged in with their Spotify account.\n", user.Email)
	// create a jwt
	claim := &blueprint.ZooveUserToken{
		//Role:       "user",
		Email:    user.Email,
		Username: user.DisplayName,
		UUID:     uniqueID,
	}
	token, err := util.SignJwt(claim)
	return util.SuccessResponse(ctx, http.StatusOK, string(token))
}

// AuthDeezerUser authorizes a user with deezer account. It generates a JWT token for
// a new user
func (c *UserController) AuthDeezerUser(ctx *fiber.Ctx) error {
	var uniqueID, _ = uuid.NewUUID()
	code := ctx.Query("code")
	state := ctx.Query("state")
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "State is not present")
	}

	if code == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Code wasn't passed")
	}

	dz := deezer.Deezer{
		ClientID:     os.Getenv("DEEZER_ID"),
		ClientSecret: os.Getenv("DEEZER_SECRET"),
		RedirectURI:  os.Getenv("DEEZER_REDIRECT_URI"),
	}

	token := dz.FetchAccessToken(code)
	if token == nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error fetching token: No deezer token fetched")
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "Could not fetch deezer token")
	}

	_, err := util.Encrypt(token, []byte(encryptionSecretKey))
	if err != nil {
		log.Printf("\n[controllers][account][users][AuthDeezerUser] Method - Error encrypting deezer token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	user, err := dz.CompleteUserAuth(token)
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error fetching deezer user: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	_, err = c.DB.Exec(queries.CreateUserQuery,
		user.Email,
		user.Name,
		uniqueID,
	)

	// now create a token
	claims := &blueprint.ZooveUserToken{
		Email:    user.Email,
		Username: user.Name,
		UUID:     uniqueID,
	}
	jToken, err := util.SignJwt(claims)
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error signing token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	return util.SuccessResponse(ctx, http.StatusOK, string(jToken))
}

// FetchProfile fetches the playlist of the person, on the platform
func (c *UserController) FetchProfile(ctx *fiber.Ctx) error {
	claims := ctx.Locals("claims").(*blueprint.ZooveUserToken)

	database := db.NewDB{
		DB: c.DB,
	}

	user, err := database.FindUserByEmail(claims.Email)
	if err != nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
	}
	return util.SuccessResponse(ctx, http.StatusOK, user)
}

// GenerateAPIKeys generates API keys for users
func (c *UserController) GenerateAPIKeys(ctx *fiber.Ctx) error {
	/**
	  SPEC
	=====================================================================================================
	  When a user wants to generate keys, first they obviously must have an account
	  At the moment, there shall be no rate limit on the APIs.

	  The API key would be like so: "xxx-xxx-xxx-xxx". A UUID v4 seems to fit this the most
	  but if there are other ways to generate an ID similar to that, then its okay. Specific way/tool to
	  arrive at the solution is up to be decided when implementing.

	  The API key shall keep count of how many requests have been made. This is to ensure that there is
	  good tracking of requests per app since there are no specific rate-limiting yet.

	  The API key shall be used in the header like: "x-orchdio-key".

	  There shall be just one key allowed per user for the moment.
	  =====================================================================================================


	  IMPLEMENTATION NOTES
	  Create a new table called apiKeys
	  Create a 1-1 (for now) relationship for apiKeys to users


	  First, check if the access token is valid. An api key is valid for indefinite time (for now)
	  If its valid, then the user can make calls. If not, they need to auth again.
	*/

	return nil
}
