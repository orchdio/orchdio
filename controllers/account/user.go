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

	if platform == "applemusic" {
		log.Printf("[account][auth] trying to connect to apple music")
		return ctx.Render("auth", fiber.Map{
			"Token": os.Getenv("APPLE_MUSIC_API_KEY"),
		})
	}

	return util.ErrorResponse(ctx, http.StatusNotImplemented, "Other Platforms have not been implemented")
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
	encryptedRefreshToken, encErr := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
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
	_, dbErr := c.DB.Exec(
		query,
		user.Email,
		user.DisplayName,
		uniqueID,
		encryptedRefreshToken,
		user.ID,
	)

	if dbErr != nil {
		log.Printf("\n[controller][account][user][spotify]: [AuthUser] Error executing query: %v\n", dbErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, dbErr)
	}

	serialized, err := json.Marshal(map[string]string{
		"spotify": user.DisplayName,
	})

	// update the usernames the user has on various playlist.
	// NB: I wasn't sure how to really handle this, if its better to do it in the createUserQuery above or split here
	// decided to split here because its just easier for me to bother with right now.
	_, err = c.DB.Exec(queries.UpdatePlatformUsernames, user.Email, string(serialized))

	log.Printf("\n[user][controller][AuthUser] Method - User with the email %s just signed up or logged in with their Spotify account.\n", user.Email)
	// create a jwt
	claim := &blueprint.OrchdioUserToken{
		Email:    user.Email,
		Username: user.DisplayName,
		UUID:     uniqueID,
	}
	token, err := util.SignJwt(claim)
	//redirectTo := os.Getenv("ZOOVE_AUTH_URL")

	//return ctx.Redirect(redirectTo + "?token=" + string(token))
	return util.SuccessResponse(ctx, http.StatusOK, string(token))
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

	encryptedRefreshToken, err := util.Encrypt(token, []byte(encryptionSecretKey))
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

	// TODO: here, check if the user is already in the DB, in that case, we just update platform username

	log.Printf("[user][controller][AuthDeezerUser] Running create user query: '%s' with '%s', '%s', '%s' \n", queries.CreateUserQuery, user.Email, user.Name, uniqueID)
	deezerID := strconv.Itoa(user.ID)
	userProfile := c.DB.QueryRowx(queries.CreateUserQuery,
		user.Email,
		user.Name,
		uniqueID,
		encryptedRefreshToken,
		deezerID,
	)

	scanErr := userProfile.StructScan(&profScan)

	if scanErr != nil {
		log.Printf("[user][controller][AuthDeezerUser] could not upsert createUserQuery. %v\n", scanErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error occurred")
	}

	serialized, err := json.Marshal(map[string]string{
		"deezer": user.Name,
	})

	_, err = c.DB.Exec(queries.UpdatePlatformUsernames, user.Email, string(serialized))
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] could not upsert createUserQuery. %v\n", err)
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

	redirectTo := os.Getenv("ZOOVE_AUTH_URL")

	return ctx.Redirect(redirectTo + "?token=" + string(jToken))
}

// AuthAppleMusicUser2 authorizes a user with apple music account.
// NB: it doesnt really work for now so not active.
//func (c *UserController) AuthAppleMusicUser2(ctx *fiber.Ctx) error {
//	bod := &blueprint.AppleMusicAuthBody{}
//	err := ctx.BodyParser(&bod)
//	if err != nil {
//		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error parsing body: %v", err)
//		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
//	}
//
//	//authedUser := map[string]interface{}{}
//	// serialize the user
//	//err := json.Unmarshal([]byte(bod.Authorization.), &authedUser)
//	//if err != nil {
//	//	log.Printf("[user][controller][AuthAppleMusicUser] Method - Error serializing user: %v", err)
//	//	return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
//	//}
//
//	//log.Printf("[user][controller][AuthAppleMusicUser] Method - Autheduser,  idToken and state: %v, %v, %v", authedUser, idToken, state)
//
//	// verify the token to get the user
//	_ = "https://appleid.apple.com/auth/token"
//
//	client := apple.New()
//	//vReq := apple.WebValidationTokenRequest{
//	//	ClientID:     os.Getenv("APPLE_CLIENT_ID"),
//	//	ClientSecret: os.Getenv("APPLE_MUSIC_API_KEY"),
//	//	Code:         bod.Authorization.Code,
//	//	RedirectURI:  "https://zoove.xyz",
//	//}
//
//	cSecret := `-----BEGIN PRIVATE KEY-----`
//	secret, err := apple.GenerateClientSecret(cSecret, os.Getenv("APPLE_TEAM_ID"), "com.orchdiodev.zoovestaging", os.Getenv("APPLE_KEY_ID"))
//	if err != nil {
//		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error generating client secret: %v", err)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
//	}
//
//	vReq := apple.WebValidationTokenRequest{
//		ClientID:     "com.orchdiodev.zoovestaging",
//		ClientSecret: secret,
//		Code:         bod.Authorization.IdToken,
//		//RedirectURI:  "https://constitution-replication-metadata-quick.trycloudflare.com/api/v1/applemusic/auth",
//	}
//
//	var resp apple.ValidationResponse
//	err = client.VerifyWebToken(context.Background(), vReq, &resp)
//	if err != nil {
//		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error verifying token: %v", err)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
//	}
//
//	if resp.Error != "" {
//		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error verifying token: %v", resp.Error)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, resp.Error)
//	}
//
//	unique, err := apple.GetUniqueID(resp.IDToken)
//	if err != nil {
//		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error getting unique id: %v", err)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
//	}
//
//	claim, err := apple.GetClaims(resp.IDToken)
//	if err != nil {
//		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error getting claims: %v", err)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
//	}
//
//	log.Printf("[user][controller][AuthAppleMusicUser] Method - Claims: %v\n", claim)
//	log.Printf("[user][controller][AuthAppleMusicUser] Method - Unique: %v\n", unique)
//	return util.SuccessResponse(ctx, http.StatusOK, "Apple Music Auth")
//}

// AuthAppleMusicUser authenticates a user with apple music account and saves the user to the db. It also creates a token for the user.
func (c *UserController) AuthAppleMusicUser(ctx *fiber.Ctx) error {
	bod := &blueprint.AppleMusicAuthBody{}
	err := ctx.BodyParser(&bod)
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error parsing body: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
	}

	uniqueID, _ := uuid.NewUUID()
	state := bod.State
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "state is required")
	}
	var displayname = "-"
	// if firstnasme isnt null, then we last name is not null either.
	if bod.FirstName != "" {
		displayname = bod.FirstName + " " + bod.LastName
	}

	encryptedRefreshToken, err := util.Encrypt([]byte(bod.Token), []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error encrypting refresh token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	// apple doesnt (seem to) have a context of user ID here, in the API, we're using the music user token and
	// developer tokens to auth and make user auth requests. Therefore, we'll simply generate an md5 hash of the
	// email address and use that as the user ID.
	hash := md5.New()
	// write the email address to the hash
	_, err = hash.Write([]byte(bod.Email))
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error hashing email: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	// get the hash as a string
	hashedEmail := hex.EncodeToString(hash.Sum(nil))

	_, err = c.DB.Exec(queries.CreateUserQuery, bod.Email, displayname, uniqueID, encryptedRefreshToken, hashedEmail)
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error creating user: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	serialized, err := json.Marshal(map[string]string{
		"applemusic": displayname,
	})

	// update the usernames the user has on various playlist.
	// NB: I wasn't sure how to really handle this, if its better to do it in the createUserQuery above or split here
	// decided to split here because its just easier for me to bother with right now.
	_, err = c.DB.Exec(queries.UpdatePlatformUsernames, bod.Email, string(serialized))

	claim := &blueprint.OrchdioUserToken{
		Email:    bod.Email,
		Username: displayname,
		UUID:     uniqueID,
	}

	token, err := util.SignJwt(claim)
	if err != nil {
		log.Printf("[user][controller][AuthAppleMusicUser] Method - Error signing JWT: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
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
	existingKey, err := database.FetchUserApikey(user.Email)
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
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")
	// we want to set the value of revoked to true
	database := db.NewDB{DB: c.DB}

	err := database.RevokeApiKey(apiKey)
	if err != nil {
		log.Printf("[controller][user][RevokeKey] error revoking key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error occured")
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
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error occured")
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
			log.Printf("[controller][user][RetrieveKey] - User does not have a key")
			return util.ErrorResponse(ctx, http.StatusNotFound, "You do not have a key")
		}

		log.Printf("[controller][user][RetrieveKey] - Could not retrieve user key. %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
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
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "An unexpected error")
	}

	if len(deletedKey) == 0 {
		log.Printf("[controller][user][DeleteKey] - key already deleted")
		return util.ErrorResponse(ctx, http.StatusNotFound, "Key not found. You already deleted this key")
	}

	log.Printf("[controller][user][DeleteKey] - deleted key for user %v\n", claims)
	return util.SuccessResponse(ctx, http.StatusOK, string(deletedKey))
}
