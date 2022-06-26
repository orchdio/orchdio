package account

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
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
	DB *sqlx.DB
}

func NewUserController(db *sqlx.DB) *UserController {
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
	claim := &blueprint.OrchdioUserToken{
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

	// for lack of better naming. thisn is the "temp" struct that we're scanning the result of the db upsert into
	profScan := struct {
		Email    string
		Username string
		UUID     uuid.UUID
	}{}

	userProfile := c.DB.QueryRowx(queries.CreateUserQuery,
		user.Email,
		user.Name,
		uniqueID,
	)

	scanErr := userProfile.StructScan(&profScan)

	if scanErr != nil {
		log.Printf("[user][controller][AuthDeezerUser] could not upsert createUserQuery. %v\n", scanErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error occurred")
	}

	// now create a token
	claims := &blueprint.OrchdioUserToken{
		Email:    user.Email,
		Username: user.Name,
		UUID:     profScan.UUID,
	}

	jToken, err := util.SignJwt(claims)
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error signing token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	log.Printf("[user][controller][AuthDeezerUser] new user login/signup. Created new login.")

	return util.SuccessResponse(ctx, http.StatusOK, string(jToken))
}

// FetchProfile fetches the playlist of the person, on the platform
func (c *UserController) FetchProfile(ctx *fiber.Ctx) error {
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

	database := db.NewDB{
		DB: c.DB,
	}

	user, err := database.FindUserByEmail(claims.Email)
	if err != nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
	}
	return util.SuccessResponse(ctx, http.StatusOK, user)
}

// GenerateAPIKey generates API key for users
func (c *UserController) GenerateAPIKey(ctx *fiber.Ctx) error {
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

	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

	apiToken, _ := uuid.NewUUID()

	database := db.NewDB{
		DB: c.DB,
	}

	// first fetch user
	user, err := database.FindUserByEmail(claims.Email)
	existingKey, err := database.FetchUserApikey(user.UUID)
	if err != nil {
		log.Printf("[controller][user][GenerateApiKey] could not fetch api key from db. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	// first check if the user already has an api key. if they do, return a
	// http conflict status
	if existingKey != nil {
		log.Printf("[controller][user][Generate] warning - user already has key")
		errResponse := "You already have a key"
		return util.ErrorResponse(ctx, http.StatusConflict, errResponse)
	}

	// save into db
	query := queries.CreateNewKey
	_, dbErr := c.DB.Exec(query,
		apiToken.String(),
		user.UUID,
	)

	if dbErr != nil {
		log.Printf("\n[controller][account][user] : [AuthUser] Error executing query: %v\n. Could not create new key", dbErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	response := map[string]string{
		"key": apiToken.String(),
	}
	log.Printf("[controller][accounnt][user]: Created a new api key for user\n")
	return util.SuccessResponse(ctx, http.StatusCreated, response)

}

// RevokeKey revokes an api key.
func (c *UserController) RevokeKey(ctx *fiber.Ctx) error {
	// get the current user
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")
	// we want to set the value of revoked to true
	database := db.NewDB{DB: c.DB}

	err := database.RevokeApiKey(apiKey, claims.UUID.String())
	if err != nil {
		log.Printf("[controller][user][RevokeKey] error revoking key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error occured")
	}

	return util.SuccessResponse(ctx, http.StatusOK, nil)
}

// UnRevokeKey unrevokes an api key.
func (c *UserController) UnRevokeKey(ctx *fiber.Ctx) error {

	// get the current user
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")
	// we want to set the value of revoked to true
	database := db.NewDB{DB: c.DB}

	err := database.UnRevokeApiKey(apiKey, claims.UUID.String())
	if err != nil {
		log.Printf("[controller][user][RevokeKey] error revoking key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error occured")
	}
	log.Printf("[controller][user][UnRevokeKey] UnRevoked key")
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}

// RetrieveKey retrieves an API key associated with the user
func (c *UserController) RetrieveKey(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][RetrieveKey] - Retrieving API key")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
	database := db.NewDB{
		DB: c.DB,
	}

	key, err := database.FetchUserApikey(claims.UUID)
	if err != nil {
		log.Printf("[controller][user][RetrieveKey] - Could not retrieve user key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}
	log.Printf("[controller][user][RetrieveKey] - Retrieved apikey for user %s\n", claims)
	return util.SuccessResponse(ctx, http.StatusOK, key.Key)
}

// DeleteKey deletes a user's api key
func (c *UserController) DeleteKey(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][DeleteKey] - deleting key")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
	apiKey := ctx.Get("x-orchdio-key")
	database := db.NewDB{DB: c.DB}

	deletedKey, err := database.DeleteApiKey(apiKey, claims.UUID.String())
	if err != nil {
		log.Printf("[controller][user][DeleteKey] - error deleting Key from database %s\n", err.Error())
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}

	if len(deletedKey) == 0 {
		log.Printf("[controller][user][DeleteKey] - key already deleted")
		return util.ErrorResponse(ctx, http.StatusNotFound, "Key not found. You already deleted this key")
	}

	log.Printf("[controller][user][DeleteKey] - deleted key for user %v\n", claims)
	return util.SuccessResponse(ctx, http.StatusOK, string(deletedKey))
}
