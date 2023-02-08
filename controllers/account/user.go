package account

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"net/mail"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
	"strconv"
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
	// swagger:route GET /:platform/connect RedirectAuth
	//
	// Redirects the user to the authorization URL for the platform.
	//
	// ---
	// Consumes:
	//  - application/json
	//
	// Produces:
	//  - application/json
	//
	// Schemes: https
	//
	// Parameters:
	//  + name: platform
	//    in: path
	//    description: The platform to connect to.
	//    required: true
	//    type: string
	//
	// Responses:
	//  200: redirectAuthResponse

	deezerSecret := os.Getenv("DEEZER_SECRET")
	deezerRedirectURL := os.Getenv("DEEZER_REDIRECT_URL")

	var uniqueID, _ = uuid.NewUUID()
	dz := &deezer.Deezer{
		ClientID:     os.Getenv("DEEZER_ID"),
		ClientSecret: deezerSecret,
		RedirectURI:  deezerRedirectURL,
	}

	platform := strings.ToLower(ctx.Params("platform"))
	var url string

	if platform == "spotify" {
		// now do spotify things here.
		_url := spotify.FetchAuthURL(uniqueID.String())
		if _url == nil {
			log.Printf("[account][auth] error - Could return URL for user")
			return util.ErrorResponse(ctx, http.StatusOK, "auth error", "Error creating auth URL")
		}

		url = string(_url)
	}

	if platform == "deezer" {
		url = dz.FetchAuthURL(uniqueID.String())
	}

	if platform == "applemusic" {
		log.Printf("[account][auth] trying to connect to apple music")
		return ctx.Render("auth", fiber.Map{
			"Token": os.Getenv("APPLE_MUSIC_API_KEY"),
		})
	}

	u := map[string]string{
		"url": url,
	}

	return util.SuccessResponse(ctx, http.StatusOK, u)
}

// AuthSpotifyUser authorizes a user with spotify account. It generates a JWT token for
// a new user
func (c *UserController) AuthSpotifyUser(ctx *fiber.Ctx) error {
	// swagger:route GET /spotify/auth AuthSpotifyUser
	//
	// Authorizes a user with spotify account. This is connects a user with a spotify account, with Orchdio.
	//
	// ---
	// Consumes:
	//  - application/json
	//
	// Produces:
	//  - application/json
	//
	// Schemes: https
	//
	// Responses:
	//  200: redirectAuthResponse
	var uniqueID, _ = uuid.NewUUID()
	state := ctx.Query("state")
	errorCode := ctx.Query("error")

	if errorCode == "access_denied" {
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "access denied", "App denied access")
	}

	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "State is not present")
	}

	// create a new net/http instance since *fasthttp.Request() cannot be passed
	r, err := http.NewRequest("GET", string(ctx.Request().RequestURI()), nil)

	if err != nil {
		log.Printf("[controllers][account][user] Error - error creating a new http request - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An unexpected error occured")
	}
	// recover from panic
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[controller][account][user][error] %v\n", r)
		}
	}()

	client, refreshToken, err := spotify.CompleteUserAuth(context.Background(), r)
	if err != nil {
		log.Printf("[controllers][account][user] Error - error completing user auth - %v\n", err)
		if err == blueprint.EINVALIDAUTHCODE {
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid auth code")
		}
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An unexpected error occured")
	}
	encryptedRefreshToken, encErr := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
	if encErr != nil {
		log.Printf("\n[controllers][account][user] Error - could not encrypt refreshToken - %v\n", encErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, encErr.Error(), "An unexpected error occurred")
	}

	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Printf("\n[controllers][account][user] Error - could not fetch current spotify user- %v\n", err)

		// THIS IS THE BEGINING OF A SUPPOSEDLY CURSED IMPLEMENTATION
		// since we might need to request token quota extension for users
		// AQB7csFtf_58P-Rq-jqrfFMhBXDJnC2xwFjLMwXr439vxbXCZdFxKpwTrnDLzJvFrY3nc2B4YeCRLOs5zgrMA4zwWZROc4P7qPt_ySlTi-qHM5w5y_eQ27PUJzLKQae5SJs
		// when the user just auths for the first time, it seems that the refresh token is gotten (for some reason, during dev)

		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An unexpected error occurred.")
	}
	log.Printf("%v", user)

	userProfile := &blueprint.User{}

	query := queries.CreateUserQuery
	newUser := c.DB.QueryRowx(
		query,
		user.Email,
		user.DisplayName,
		uniqueID,
		encryptedRefreshToken,
		user.ID,
	)

	dbErr := newUser.StructScan(userProfile)

	if dbErr != nil {
		log.Printf("\n[controller][account][user][spotify]: [AuthUser] Error executing query: %v\n", dbErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}

	serialized, err := json.Marshal(map[string]string{
		"spotify": user.DisplayName,
	})

	// update the usernames the user has on various playlist.
	// NB: I wasn't sure how to really handle this, if its better to do it in the createUserQuery above or split here
	// decided to split here because its just easier for me to bother with right now.
	_, err = c.DB.Exec(queries.UpdatePlatformUsernames, user.Email, string(serialized))

	// update user platform token
	_, err = c.DB.Exec(queries.UpdateUserPlatformToken, encryptedRefreshToken, "spotify", user.Email)
	if err != nil {
		log.Printf("[db][UpdateUserPlatformToken] error updating user platform token. %v\n", err)
		return err
	}

	log.Printf("\n[user][controller][AuthUser] Method - App with the email %s just signed up or logged in with their Spotify account.\n", user.Email)
	// create a jwt
	claim := &blueprint.OrchdioUserToken{
		Email:    user.Email,
		Username: user.DisplayName,
		UUID:     userProfile.UUID,
		Platform: "spotify",
	}
	token, err := util.SignJwt(claim)
	redirectTo := os.Getenv("ZOOVE_AUTH_URL")

	allowedOrigins := []string{
		"https://orchdio.com",
		"http://localhost:4044",
		os.Getenv("ZOOVE_AUTH_URL"),
		"https://zoove.xyz",
		"https://www.zoove.xyz",
		"https://api.orchdio.dev",
		"https://api.orchdio.com",
	}

	// get the origin of the initial request
	//origin := ctx.Get("Origin")
	for _, origin := range allowedOrigins {
		if origin == ctx.Get("Origin") {
			redirectTo = origin
			break
		}
	}

	return ctx.Redirect(redirectTo + "?token=" + string(token))
}
func (c *UserController) AuthOrchdioSpotifyUser(ctx *fiber.Ctx) error {
	// swagger:route GET /spotify/auth AuthSpotifyUser
	//
	// Authorizes a user with spotify account. This is connects a user with a spotify account, with Orchdio.
	//
	// ---
	// Consumes:
	//  - application/json
	//
	// Produces:
	//  - application/json
	//
	// Schemes: https
	//
	// Responses:
	//  200: redirectAuthResponse
	var uniqueID, _ = uuid.NewUUID()
	state := ctx.Query("state")
	errorCode := ctx.Query("error")

	if errorCode == "access_denied" {
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "unauthorized", "App denied access")
	}

	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "State is not present")
	}

	// create a new net/http instance since *fasthttp.Request() cannot be passed
	r, err := http.NewRequest("GET", string(ctx.Request().RequestURI()), nil)

	if err != nil {
		log.Printf("[controllers][account][user] Error - error creating a new http request - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An unexpected error occurred.")
	}
	// recover from panic
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[controller][account][user][error] %v\n", r)
		}
	}()

	client, refreshToken, err := spotify.CompleteUserAuth(context.Background(), r)
	if err != nil {
		log.Printf("[controllers][account][user] Error - error completing user auth - %v\n", err)
		if err == blueprint.EINVALIDAUTHCODE {
			log.Printf("[controllers][account][user] Error - invalid auth code - %v\n", err)
			return util.ErrorResponse(ctx, http.StatusBadRequest, err.Error(), "code has expired")
		}

		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An unexpected error")
	}
	encryptedRefreshToken, encErr := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
	if encErr != nil {
		log.Printf("\n[controllers][account][user] Error - could not encrypt refreshToken - %v\n", encErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not encrypt refresh token")
	}

	log.Printf("[account][auth] encrypted refresh token: %v\n", encryptedRefreshToken)

	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Printf("\n[controllers][account][user] Error - could not fetch current spotify user- %v\n", err)

		// THIS IS THE BEGINING OF A SUPPOSEDLY CURSED IMPLEMENTATION
		// since we might need to request token quota extension for users
		// AQB7csFtf_58P-Rq-jqrfFMhBXDJnC2xwFjLMwXr439vxbXCZdFxKpwTrnDLzJvFrY3nc2B4YeCRLOs5zgrMA4zwWZROc4P7qPt_ySlTi-qHM5w5y_eQ27PUJzLKQae5SJs
		// when the user just auths for the first time, it seems that the refresh token is gotten (for some reason, during dev)

		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "An unexpected error occurred")
	}
	log.Printf("%v", user)

	userProfile := &blueprint.User{}

	query := queries.CreateUserQuery
	newUser := c.DB.QueryRowx(
		query,
		user.Email,
		user.DisplayName,
		uniqueID,
		encryptedRefreshToken,
		user.ID,
	)

	dbErr := newUser.StructScan(userProfile)

	if dbErr != nil {
		log.Printf("\n[controller][account][user][spotify]: [AuthUser] Error executing query: %v\n", dbErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred and could not updated user profile in database")
	}

	serialized, err := json.Marshal(map[string]string{
		"spotify": user.DisplayName,
	})

	// update the usernames the user has on various playlist.
	// NB: I wasn't sure how to really handle this, if it's better to do it in the createUserQuery above or split here
	// decided to split here because it's just easier for me to bother with right now.
	_, err = c.DB.Exec(queries.UpdatePlatformUsernames, user.Email, string(serialized))

	// update user platform token
	_, err = c.DB.Exec(queries.UpdateUserPlatformToken, encryptedRefreshToken, "spotify", user.Email)
	if err != nil {
		log.Printf("[db][UpdateUserPlatformToken] error updating user platform token. %v\n", err)
		return err
	}

	log.Printf("\n[user][controller][AuthUser] Method - App with the email %s just signed up or logged in with their Spotify account.\n", user.Email)
	// create a jwt
	claim := &blueprint.OrchdioUserToken{
		Email:    user.Email,
		Username: user.DisplayName,
		UUID:     userProfile.UUID,
		Platform: "spotify",
	}
	token, err := util.SignJwt(claim)
	redirectTo := os.Getenv("ORCHDIO_REDIRECT_URI")

	return ctx.Redirect(redirectTo + "?token=" + string(token))
}

// AuthDeezerUser authorizes a user with deezer account. It generates a JWT token for
// a new user
func (c *UserController) AuthDeezerUser(ctx *fiber.Ctx) error {
	//// swagger:route GET /deezer/auth AuthSpotifyUser
	////
	//// Authorizes a user with spotify account. This is connects a user with a deezer account, with Orchdio.
	////
	//// ---
	//// Consumes:
	////  - application/json
	////
	//// Produces:
	////  - application/json
	////
	//// Schemes: https
	////
	//// Responses:
	////  200: deezerAuthResponse
	//
	//// swagger:response deezerAuthResponse
	//type DeezerAuthResponse struct {
	//	Message string `json:"message"`
	//	Status  string `json:"status"`
	//	// Example: "https://connect.deezer.com/oauth/auth.php?app_id=&redirect_uri=&perms=basic_access,email"
	//	//
	//	// Required: true
	//	Data interface{} `json:"data"`
	//}

	var uniqueID, _ = uuid.NewUUID()
	code := ctx.Query("code")
	state := ctx.Query("state")
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "State is not present")
	}

	if code == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Code wasn't passed")
	}

	dz := deezer.Deezer{
		ClientID:     os.Getenv("DEEZER_ID"),
		ClientSecret: os.Getenv("DEEZER_SECRET"),
		RedirectURI:  os.Getenv("DEEZER_REDIRECT_URL"),
	}

	token := dz.FetchAccessToken(code)
	if token == nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error fetching token: No deezer token fetched")
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not fetch deezer token")
	}

	encryptedRefreshToken, err := util.Encrypt(token, []byte(encryptionSecretKey))
	if err != nil {
		log.Printf("\n[controllers][account][users][AuthDeezerUser] Method - Error encrypting deezer token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not encrypt refresh token")
	}
	user, err := dz.CompleteUserAuth(token)
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error fetching deezer user: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "could not fetch user token")
	}

	userProfile := &blueprint.User{}
	// TODO: here, check if the user is already in the DB, in that case, we just update platform username

	log.Printf("[user][controller][AuthDeezerUser] Running create user query: '%s' with '%s', '%s', '%s' \n", queries.CreateUserQuery, user.Email, user.Name, uniqueID)
	deezerID := strconv.Itoa(user.ID)
	newUser := c.DB.QueryRowx(queries.CreateUserQuery,
		user.Email,
		user.Name,
		uniqueID,
		encryptedRefreshToken,
		deezerID,
	)

	scanErr := newUser.StructScan(userProfile)

	if scanErr != nil {
		log.Printf("[user][controller][AuthDeezerUser] could not upsert createUserQuery. %v\n", scanErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
	}

	serialized, err := json.Marshal(map[string]string{
		"deezer": user.Name,
	})

	_, err = c.DB.Exec(queries.UpdatePlatformUsernames, user.Email, string(serialized))
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] could not upsert createUserQuery. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occurred")
	}

	// update user platform token
	_, err = c.DB.Exec(queries.UpdateUserPlatformToken, encryptedRefreshToken, "deezer", user.Email)
	if err != nil {
		log.Printf("[db][UpdateUserPlatformToken] error updating user platform token. %v\n", err)
		return err
	}

	// now create a token
	claims := &blueprint.OrchdioUserToken{
		Email:    user.Email,
		Username: user.Name,
		UUID:     userProfile.UUID,
		Platform: "deezer",
	}

	jToken, err := util.SignJwt(claims)
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error signing token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "could not sign token")
	}

	log.Printf("[user][controller][AuthDeezerUser] new user login/signup. Created new login.")

	redirectTo := os.Getenv("ZOOVE_AUTH_URL")

	return ctx.Redirect(redirectTo + "?token=" + string(jToken))
}

// AuthAppleMusicUser authenticates a user with apple music account and saves the user to the db. It also creates a token for the user.
func (c *UserController) AuthAppleMusicUser(ctx *fiber.Ctx) error {
	bod := &blueprint.AppleMusicAuthBody{}
	err := ctx.BodyParser(&bod)
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error parsing body: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Invalid request body. please check the body and try again.")
	}

	uniqueID, _ := uuid.NewUUID()
	state := bod.State
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "state is required")
	}
	var displayname = "-"
	// if firstnasme isnt null, then we last name is not null either.
	if bod.FirstName != "" {
		displayname = bod.FirstName + " " + bod.LastName
	}

	encryptedRefreshToken, err := util.Encrypt([]byte(bod.Token), []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error encrypting refresh token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not encrypt refresh token")
	}

	// apple doesnt (seem to) have a context of user ID here, in the API, we're using the music user token and
	// developer tokens to auth and make user auth requests. Therefore, we'll simply generate an md5 hash of the
	// email address and use that as the user ID.
	hash := md5.New()
	// write the email address to the hash
	_, err = hash.Write([]byte(bod.Email))
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error hashing email: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "could not hash email.")
	}
	// get the hash as a string
	hashedEmail := hex.EncodeToString(hash.Sum(nil))
	userProfile := &blueprint.User{}
	newUser := c.DB.QueryRowx(queries.CreateUserQuery, bod.Email, displayname, uniqueID, encryptedRefreshToken, hashedEmail)
	err = newUser.StructScan(userProfile)
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error creating user: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create user")
	}

	serialized, err := json.Marshal(map[string]string{
		"applemusic": displayname,
	})

	// update the usernames the user has on various playlist.
	// NB: I wasn't sure how to really handle this, if its better to do it in the createUserQuery above or split here
	// decided to split here because its just easier for me to bother with right now.
	_, err = c.DB.Exec(queries.UpdatePlatformUsernames, bod.Email, string(serialized))
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error updating platform usernames: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not update platform usernames")
	}

	// update user platform token
	_, err = c.DB.Exec(queries.UpdateUserPlatformToken, encryptedRefreshToken, "applemusic", bod.Email)
	if err != nil {
		log.Printf("[db][UpdateUserPlatformToken] error updating user platform token. %v\n", err)
		return err
	}

	claim := &blueprint.OrchdioUserToken{
		Email:    bod.Email,
		Username: displayname,
		UUID:     uniqueID,
		Platform: "applemusic",
	}

	token, err := util.SignJwt(claim)
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error signing JWT: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not sign token.")
	}

	// redirect to the frontend with the token
	return util.SuccessResponse(ctx, http.StatusOK, string(token))
}

// FetchProfile fetches the playlist of the person, on the platform
func (c *UserController) FetchProfile(ctx *fiber.Ctx) error {
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

	database := db.NewDB{
		DB: c.DB,
	}

	user, err := database.FindUserByEmail(claims.Email, claims.Platform)
	if err != nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not fetch user profile")
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
	user, err := database.FindUserByEmail(claims.Email, claims.Platform)
	existingKey, err := database.FetchUserApikey(user.Email)
	if err != nil && err != sql.ErrNoRows {
		if err != sql.ErrNoRows {

			log.Printf("[controller][user][GenerateApiKey] could not fetch api key from db. %v\n", err)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not fetch key from database")
		}
	}

	// first check if the user already has an api key. if they do, return a
	// http conflict status
	if existingKey != nil {
		log.Printf("[controller][user][Generate] warning - user already has key")
		errResponse := "You already have a key"
		return util.ErrorResponse(ctx, http.StatusConflict, "record conflict", errResponse)
	}

	// save into db
	query := queries.CreateNewKey
	_, dbErr := c.DB.Exec(query,
		apiToken.String(),
		user.UUID,
	)

	if dbErr != nil {
		log.Printf("\n[controller][account][user][AuthUser] Error executing query: %v\n. Could not create new key", dbErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create new key")
	}

	response := map[string]string{
		"key": apiToken.String(),
	}
	log.Printf("[controller][accounnt][user]: Created a new api key for user\n")
	return util.SuccessResponse(ctx, http.StatusCreated, response)

}

// RevokeKey revokes an api key.
func (c *UserController) RevokeKey(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")
	// we want to set the value of revoked to true
	database := db.NewDB{DB: c.DB}

	err := database.RevokeApiKey(apiKey)
	if err != nil {
		log.Printf("[controller][user][RevokeKey] error revoking key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}

	return util.SuccessResponse(ctx, http.StatusOK, nil)
}

// UnRevokeKey unrevokes an api key.
func (c *UserController) UnRevokeKey(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")
	// we want to set the value of revoked to true
	database := db.NewDB{DB: c.DB}

	err := database.UnRevokeApiKey(apiKey)
	if err != nil {
		log.Printf("[controller][user][RevokeKey] error revoking key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	log.Printf("[controller][user][UnRevokeKey] UnRevoked key")
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}

// RetrieveKey retrieves an API key associated with the user
func (c *UserController) RetrieveKey(ctx *fiber.Ctx) error {
	// swagger:route GET /key RetrieveKey
	//
	// Retrieves an API key associated with the user. The user is known by examining the request header and as such, the user must be authenticated
	//
	// ---
	// Consumes:
	//  - application/json
	//
	// Produces:
	//  - application/json
	//
	// Schemes: https
	//
	// Security:
	// 	api_key:
	// 		[x-orchdio-key]:
	//
	// Responses:
	//  200: retrieveApiKeyResponse

	// swagger:response retrieveApiKeyResponse
	type RetrieveApiKeyResponse struct {
		// The message attached to the response.
		//
		// Required: true
		//
		// Example: "This is a message about whatever i can tell you about the error"
		Message string `json:"message"`
		// Description: The error code attached to the response. This will return 200 (or 201), depending on the endpoint. It returns 4xx - 5xx as suitable, otherwise.
		//
		// Required: true
		//
		// Example: 201
		Status string `json:"status"`
		// The key attached to the response.
		//
		// Example: c8e51d6c-4d6f-42f6-bcb6-9da19fc5b848
		//
		// Required: true
		Data interface{} `json:"data"`
	}

	log.Printf("[controller][user][RetrieveKey] - Retrieving API key")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
	database := db.NewDB{
		DB: c.DB,
	}

	key, err := database.FetchUserApikey(claims.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[controller][user][RetrieveKey] - App does not have a key")
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "You do not have a key")
		}

		log.Printf("[controller][user][RetrieveKey] - Could not retrieve user key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error")
	}
	log.Printf("[controller][user][RetrieveKey] - Retrieved apikey for user %+v\n", key)
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
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error")
	}

	if len(deletedKey) == 0 {
		log.Printf("[controller][user][DeleteKey] - key already deleted")
		return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "Key not found. You already deleted this key")
	}

	log.Printf("[controller][user][DeleteKey] - deleted key for user %v\n", claims)
	return util.SuccessResponse(ctx, http.StatusOK, string(deletedKey))
}

func (c *UserController) AddToWaitlist(ctx *fiber.Ctx) error {
	// we want to be able to add users to the waitlist. This means that we add the email to a "waitlist" table in the db
	// we check if the user already has been added to waitlist, if so we tell them we'll onboard them soon, if not, we add them to waitlist

	// get the email from the request body
	body := blueprint.AddToWaitlistBody{}
	err := json.Unmarshal(ctx.Body(), &body)
	if err != nil {
		log.Printf("[controller][user][AddToWaitlist] - error unmarshalling body %v\n", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body")
	}

	_, err = mail.ParseAddress(body.Email)
	if err != nil {
		log.Printf("[controller][user][AddToWaitlist] - invalid email %v\n", body)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid email")
	}

	// generate a uuid
	uniqueID, _ := uuid.NewRandom()

	// check if the user already exists in the waitlist
	database := db.NewDB{DB: c.DB}
	alreadyAdded := database.AlreadyInWaitList(body.Email)

	if alreadyAdded {
		log.Printf("[controller][user][AddToWaitlist] - user already in waitlist %v\n", body)
		return util.ErrorResponse(ctx, http.StatusConflict, "already exists", "You are already on the wait list")
	}

	// then insert the email into the waitlist table. it returns an email and updates the updated_at field if email is already in the table.
	result := c.DB.QueryRowx(queries.CreateWaitlistEntry, uniqueID, body.Email, body.Platform)
	var emailFromDB string
	err = result.Scan(&emailFromDB)
	if err != nil {
		log.Printf("[controller][user][AddToWaitlist] - error inserting email into waitlist table %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	return util.SuccessResponse(ctx, http.StatusOK, emailFromDB)
}

func (c *UserController) CreateOrUpdateRedirectURL(ctx *fiber.Ctx) error {
	// swagger:route POST /redirect CreateOrUpdateRedirectURL
	// Creates or updates a redirect URL for a user
	//
	claims := ctx.Locals("developer").(*blueprint.User)

	body := ctx.Body()
	redirectURL := struct {
		Url string `json:"redirect_url"`
	}{}
	err := json.Unmarshal(body, &redirectURL)
	if err != nil {
		log.Printf("[controller][user][CreateOrUpdateRedirectURL] - error unmarshalling body %v\n", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body")
	}

	if redirectURL.Url == "" {
		log.Printf("[controller][user][CreateOrUpdateRedirectURL] - redirect url is empty %v\n", redirectURL)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body")
	}

	// TODO: validate redirectURL, perform network check to see if it's reachable
	database := db.NewDB{DB: c.DB}
	err = database.UpdateRedirectURL(claims.UUID.String(), redirectURL.Url)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[controller][user][CreateOrUpdateRedirectURL] - user not found %v\n", claims)
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "App not found")
		}
		log.Printf("[controller][user][CreateOrUpdateRedirectURL] - error updating redirect url %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	log.Printf("[controller][user][CreateOrUpdateRedirectURL] - updated redirect url for user %v\n", claims.UUID.String())
	return util.SuccessResponse(ctx, http.StatusOK, nil)
}
