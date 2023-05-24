package account

import (
	"database/sql"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/services"
	"orchdio/services/deezer"
	orchdioFollow "orchdio/services/follow"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
	"strings"
)

type UserController struct {
	DB    *sqlx.DB
	Redis *redis.Client
}

func NewUserController(db *sqlx.DB, r *redis.Client) *UserController {
	return &UserController{
		DB:    db,
		Redis: r,
	}
}

func (u *UserController) AddToWaitlist(ctx *fiber.Ctx) error {
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
	database := db.NewDB{DB: u.DB}
	alreadyAdded := database.AlreadyInWaitList(body.Email)

	if alreadyAdded {
		log.Printf("[controller][user][AddToWaitlist] - user already in waitlist %v\n", body)
		return util.ErrorResponse(ctx, http.StatusConflict, "already exists", "You are already on the wait list")
	}

	// then insert the email into the waitlist table. it returns an email and updates the updated_at field if email is already in the table.
	result := u.DB.QueryRowx(queries.CreateWaitlistEntry, uniqueID, body.Email, body.Platform)
	var emailFromDB string
	err = result.Scan(&emailFromDB)
	if err != nil {
		log.Printf("[controller][user][AddToWaitlist] - error inserting email into waitlist table %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	return util.SuccessResponse(ctx, http.StatusOK, emailFromDB)
}

//func (u *UserController) CreateOrUpdateRedirectURL(ctx *fiber.Ctx) error {
//	// swagger:route POST /redirect CreateOrUpdateRedirectURL
//	// Creates or updates a redirect URL for a user
//	//
//	claims := ctx.Locals("developer").(*blueprint.User)
//
//	body := ctx.Body()
//	redirectURL := struct {
//		Url string `json:"redirect_url"`
//	}{}
//	err := json.Unmarshal(body, &redirectURL)
//	if err != nil {
//		log.Printf("[controller][user][CreateOrUpdateRedirectURL] - error unmarshalling body %v\n", err)
//		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body")
//	}
//
//	if redirectURL.Url == "" {
//		log.Printf("[controller][user][CreateOrUpdateRedirectURL] - redirect url is empty %v\n", redirectURL)
//		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid request body")
//	}
//	return util.SuccessResponse(ctx, http.StatusOK, nil)
//}

// FetchProfile fetches the user profile
func (u *UserController) FetchProfile(ctx *fiber.Ctx) error {
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
	if claims.Email == "" {
		log.Printf("\n[user][controller][FetchUserProfile] warning - email not passed. Please pass email")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email not passed")
	}
	log.Printf("\n[user][controller][FetchUserProfile] fetching user profile with email %s\n", claims.Email)
	// get the user via the email
	database := db.NewDB{DB: u.DB}
	user, err := database.FindUserProfileByEmail(claims.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("\n[user][controller][FetchUserProfile] error - user not found %v\n", err)
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User profile not found. This user may not have connected to Orchdio yet")
		}
		log.Printf("\n[user][controller][FetchUserProfile] error - error fetching user profile %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	return util.SuccessResponse(ctx, http.StatusOK, user)
}

// FetchUserProfile fetches the user profile.
func (u *UserController) FetchUserProfile(ctx *fiber.Ctx) error {
	email := ctx.Query("email")
	if email == "" {
		log.Printf("\n[user][controller][FetchUserProfile] warning - email not passed. Please pass email")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email not passed")
	}
	log.Printf("\n[user][controller][FetchUserProfile] fetching user profile with email %s\n", email)

	// check if the email is valid
	_, err := mail.ParseAddress(email)
	if err != nil {
		log.Printf("\n[user][controller][FetchUserProfile] error - invalid email %v\n", err)
	}
	database := db.NewDB{DB: u.DB}
	user, err := database.FindUserProfileByEmail(email)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("\n[user][controller][FetchUserProfile] error - user not found %v\n", err)
			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "User profile not found. This user may not have connected to Orchdio yet")
		}
		log.Printf("\n[user][controller][FetchUserProfile] error - error fetching user profile %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
	}
	return util.SuccessResponse(ctx, http.StatusOK, user)
}

func (u *UserController) FollowPlaylist(ctx *fiber.Ctx) error {
	log.Printf("[controller][follow][FollowPlaylist] - follow playlist")

	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	var platforms = []string{"tidal", "spotify", "deezer"}

	user := ctx.Locals("user").(*blueprint.User)
	var subscriberBody = struct {
		Users []string `json:"users"`
		Url   string   `json:"url"`
	}{}
	err := ctx.BodyParser(&subscriberBody)

	if err != nil {
		log.Printf("[controller][follow][FollowPlaylist] - error parsing body: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not follow playlist. Invalid body passed")
	}

	if len(subscriberBody.Users) > 20 {
		log.Printf("[controller][follow][FollowPlaylist] - too many subscribers. Max is 20")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "large subscriber body", "too many subscribers. Maximum is 20")
	}
	for _, subscriber := range subscriberBody.Users {
		if !util.IsValidUUID(subscriber) {
			log.Printf("[controller][follow][FollowPlaylist] - error parsing subscriber uuid: %v", err)
			return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid subscriber uuid", "Invalid subscriber id present. Please make sure all subscribers are uuid format")
		}
	}

	linkInfo, err := services.ExtractLinkInfo(subscriberBody.Url)
	if err != nil {
		log.Printf("[controller][follow][FollowPlaylist] - error extracting link info: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not extract link information.")
	}

	_ = strings.ToLower(linkInfo.Platform)
	if !lo.Contains(platforms, linkInfo.Platform) {
		log.Printf("[controller][follow][FollowPlaylist] - platform not supported")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid platform", "platform not supported. Please make sure the tracks are from the supported platforms.")
	}

	if !strings.Contains(linkInfo.Entity, "playlist") {
		log.Printf("[controller][conversion][playlist] - not a playlist")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "not a playlist", "It seems your didnt pass a playlist url. Please check your url again")
	}

	follow := orchdioFollow.NewFollow(u.DB, u.Redis)

	followId, err := follow.FollowPlaylist(user.UUID.String(), app.UID.String(), subscriberBody.Url, linkInfo, subscriberBody.Users)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[controller][follow][FollowPlaylist] - error following playlist: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not follow playlist")
	}

	// if the error returned is sql.ErrNoRows, it means that the playlist is already followed
	//and the length of subscribers passed in the request body is 1
	if err == blueprint.EALREADY_EXISTS {
		log.Printf("[controller][follow][FollowPlaylist] - playlist already followed")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Already followed", "playlist already followed")
	}

	res := map[string]interface{}{"follow_id": string(followId)}
	return util.SuccessResponse(ctx, http.StatusOK, res)
}

func (u *UserController) FetchUserInfoByIdentifier(ctx *fiber.Ctx) error {
	app := ctx.Locals("app").(*blueprint.DeveloperApp)
	i := ctx.Query("identifier")
	if i == "" {
		log.Printf("[controller][user][FetchUserInfoByIdentifier] - identifier not passed. Please pass a valid Orchdio ID or email")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Identifier not passed")
	}
	log.Printf("[controller][user][FetchUserInfoByIdentifier] - fetching user info with identifier %s", i)

	// decode the identifier
	identifier, err := url.QueryUnescape(i)
	if err != nil {
		log.Printf("[controller][user][FetchUserInfoByIdentifier] - error decoding identifier: might be not be url encoded. %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Invalid identifier")
	}

	// check if the identifier is a valid uuid
	isUUID := util.IsValidUUID(identifier)
	parsedEmail, err := mail.ParseAddress(identifier)
	if err != nil {
		log.Printf("[controller][user][FetchUserInfoByIdentifier][warning] could not parse identifier as email. might be uuid identifier instead: %v", err)
	}

	isValidEmail := parsedEmail != nil
	if !isUUID && !isValidEmail {
		log.Printf("[controller][user][FetchUserInfoByIdentifier] - invalid identifier. Please pass a valid Orchdio ID or email")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid identifier", "Please pass a valid Orchdio ID or email")
	}

	database := db.NewDB{DB: u.DB}
	userProfile, err := database.FetchUserByIdentifier(identifier, app.UID.String())
	if err != nil {
		log.Printf("[controller][user][FetchUserInfoByIdentifier] - error fetching user info: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not fetch user info")
	}

	// for each of the response, depending on the platform, we want to make a request to the endpoint of the platform
	// to get the user info
	var userInfo blueprint.UserInfo
	for _, user := range *userProfile {
		userInfo.Email = user.Email
		userInfo.ID = user.UserID
		switch user.Platform {
		case spotify.IDENTIFIER:
			// decrypt the spotify credentials for this app
			log.Printf("decrypting %s's spotify refresh token", user.Username)
			credBytes, err := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
			if err != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier] - error decrypting spotify credentials: %v", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not decrypt spotify credentials")
			}

			var cred blueprint.IntegrationCredentials
			err = json.Unmarshal(credBytes, &cred)
			if err != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier] - error unmarshalling spotify credentials: %v", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not unmarshal spotify credentials")
			}

			// decrypt the user access token
			accessToken, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
			if err != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier] - error decrypting spotify access token: %v", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not decrypt spotify access token")
			}
			log.Printf("[controller][user][FetchUserInfoByIdentifier] - User's access token is %s", string(accessToken))

			spotifyService := spotify.NewService(&cred, u.DB, u.Redis)
			spotifyInfo, serviceErr := spotifyService.FetchUserInfo(string(accessToken))
			if serviceErr != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier	] - error fetching spotify user info: %v", serviceErr)
				continue
			}
			userInfo.Spotify = spotifyInfo

		case deezer.IDENTIFIER:
			// decrypt the deezer credentials for this app
			log.Printf("decrypting %s's deezer refresh token", user.Username)
			credBytes, decErr := util.Decrypt(app.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
			if decErr != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier] - error decrypting deezer credentials: %v", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not decrypt deezer credentials")
			}

			var cred blueprint.IntegrationCredentials
			cErr := json.Unmarshal(credBytes, &cred)
			if cErr != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier] - error unmarshalling deezer credentials: %v", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not unmarshal deezer credentials")
			}

			accessToken, err := util.Decrypt(user.RefreshToken, []byte(os.Getenv("ENCRYPTION_SECRET")))
			if err != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier] - error decrypting deezer access token: %v", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not decrypt deezer access token")
			}

			deezerService := deezer.NewService(&cred, u.DB, u.Redis)

			deezerInfo, err := deezerService.FetchUserInfo(string(accessToken))
			if err != nil {
				log.Printf("[controller][user][FetchUserInfoByIdentifier] - error fetching deezer user info: %v", err)
				continue
			}
			userInfo.Deezer = deezerInfo
		}
	}

	log.Printf("[controller][user][FetchUserInfoByIdentifier] - user info fetched successfully")
	return util.SuccessResponse(ctx, http.StatusOK, userInfo)
}

// GenerateAPIKey generates API key for users
//func (c *UserController) GenerateAPIKey(ctx *fiber.Ctx) error {
//	/**
//	  SPEC
//	=====================================================================================================
//	  When a user wants to generate keys, first they obviously must have an account
//	  At the moment, there shall be no rate limit on the APIs.
//
//	  The API key would be like so: "xxx-xxx-xxx-xxx". A UUID v4 seems to fit this the most
//	  but if there are other ways to generate an ID similar to that, then its okay. Specific way/tool to
//	  arrive at the solution is up to be decided when implementing.
//
//	  The API key shall keep count of how many requests have been made. This is to ensure that there is
//	  good tracking of requests per app since there are no specific rate-limiting yet.
//
//	  The API key shall be used in the header like: "x-orchdio-key".
//
//	  There shall be just one key allowed per user for the moment.
//	  =====================================================================================================
//
//
//	  IMPLEMENTATION NOTES
//	  Create a new table called apiKeys
//	  Create a 1-1 (for now) relationship for apiKeys to users
//
//
//	  First, check if the access token is valid. An api key is valid for indefinite time (for now)
//	  If its valid, then the user can make calls. If not, they need to auth again.
//	*/
//
//	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
//
//	apiToken, _ := uuid.NewUUID()
//
//	database := db.NewDB{
//		DB: c.DB,
//	}
//
//	// first fetch user
//	user, err := database.FindUserByEmail(claims.Email, claims.Platform)
//	existingKey, err := database.FetchUserApikey(user.Email)
//	if err != nil && err != sql.ErrNoRows {
//		if err != sql.ErrNoRows {
//
//			log.Printf("[controller][user][GenerateApiKey] could not fetch api key from db. %v\n", err)
//			return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "Could not fetch key from database")
//		}
//	}
//
//	// first check if the user already has an api key. if they do, return a
//	// http conflict status
//	if existingKey != nil {
//		log.Printf("[controller][user][Generate] warning - user already has key")
//		errResponse := "You already have a key"
//		return util.ErrorResponse(ctx, http.StatusConflict, "record conflict", errResponse)
//	}
//
//	// save into db
//	query := queries.CreateNewKey
//	_, dbErr := c.DB.Exec(query,
//		apiToken.String(),
//		user.UUID,
//	)
//
//	if dbErr != nil {
//		log.Printf("\n[controller][account][user][AuthUser] Error executing query: %v\n. Could not create new key", dbErr)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create new key")
//	}
//
//	response := map[string]string{
//		"key": apiToken.String(),
//	}
//	log.Printf("[controller][accounnt][user]: Created a new api key for user\n")
//	return util.SuccessResponse(ctx, http.StatusCreated, response)
//
//}

// RevokeKey revokes an api key.
//func (c *UserController) RevokeKey(ctx *fiber.Ctx) error {
//	// get the api key from the header
//	apiKey := ctx.Get("x-orchdio-key")
//	// we want to set the value of revoked to true
//	database := db.NewDB{DB: c.DB}
//
//	err := database.RevokeApiKey(apiKey)
//	if err != nil {
//		log.Printf("[controller][user][RevokeKey] error revoking key. %v\n", err)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
//	}
//
//	return util.SuccessResponse(ctx, http.StatusOK, nil)
//}

// UnRevokeKey unrevokes an api key.
//func (c *UserController) UnRevokeKey(ctx *fiber.Ctx) error {
//	// get the api key from the header
//	apiKey := ctx.Get("x-orchdio-key")
//	// we want to set the value of revoked to true
//	database := db.NewDB{DB: c.DB}
//
//	err := database.UnRevokeApiKey(apiKey)
//	if err != nil {
//		log.Printf("[controller][user][RevokeKey] error revoking key. %v\n", err)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error occured")
//	}
//	log.Printf("[controller][user][UnRevokeKey] UnRevoked key")
//	return util.SuccessResponse(ctx, http.StatusOK, nil)
//}

// RetrieveKey retrieves an API key associated with the user
//func (c *UserController) RetrieveKey(ctx *fiber.Ctx) error {
//	// swagger:route GET /key RetrieveKey
//	//
//	// Retrieves an API key associated with the user. The user is known by examining the request header and as such, the user must be authenticated
//	//
//	// ---
//	// Consumes:
//	//  - application/json
//	//
//	// Produces:
//	//  - application/json
//	//
//	// Schemes: https
//	//
//	// Security:
//	// 	api_key:
//	// 		[x-orchdio-key]:
//	//
//	// Responses:
//	//  200: retrieveApiKeyResponse
//
//	// swagger:response retrieveApiKeyResponse
//	type RetrieveApiKeyResponse struct {
//		// The message attached to the response.
//		//
//		// Required: true
//		//
//		// Example: "This is a message about whatever i can tell you about the error"
//		Message string `json:"message"`
//		// Description: The error code attached to the response. This will return 200 (or 201), depending on the endpoint. It returns 4xx - 5xx as suitable, otherwise.
//		//
//		// Required: true
//		//
//		// Example: 201
//		Status string `json:"status"`
//		// The key attached to the response.
//		//
//		// Example: c8e51d6c-4d6f-42f6-bcb6-9da19fc5b848
//		//
//		// Required: true
//		Payload interface{} `json:"data"`
//	}
//
//	log.Printf("[controller][user][RetrieveKey] - Retrieving API key")
//	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
//	database := db.NewDB{
//		DB: c.DB,
//	}
//
//	key, err := database.FetchUserApikey(claims.Email)
//	if err != nil {
//		if err == sql.ErrNoRows {
//			log.Printf("[controller][user][RetrieveKey] - App does not have a key")
//			return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "You do not have a key")
//		}
//
//		log.Printf("[controller][user][RetrieveKey] - Could not retrieve user key. %v\n", err)
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error")
//	}
//	log.Printf("[controller][user][RetrieveKey] - Retrieved apikey for user %+v\n", key)
//	return util.SuccessResponse(ctx, http.StatusOK, key.Key)
//}

// DeleteKey deletes a user's api key
//func (c *UserController) DeleteKey(ctx *fiber.Ctx) error {
//	log.Printf("[controller][user][DeleteKey] - deleting key")
//	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
//	apiKey := ctx.Get("x-orchdio-key")
//	database := db.NewDB{DB: c.DB}
//
//	deletedKey, err := database.DeleteApiKey(apiKey, claims.UUID.String())
//	if err != nil {
//		log.Printf("[controller][user][DeleteKey] - error deleting Key from database %s\n", err.Error())
//		return util.ErrorResponse(ctx, http.StatusInternalServerError, "internal error", "An unexpected error")
//	}
//
//	if len(deletedKey) == 0 {
//		log.Printf("[controller][user][DeleteKey] - key already deleted")
//		return util.ErrorResponse(ctx, http.StatusNotFound, "not found", "Key not found. You already deleted this key")
//	}
//
//	log.Printf("[controller][user][DeleteKey] - deleted key for user %v\n", claims)
//	return util.SuccessResponse(ctx, http.StatusOK, string(deletedKey))
//}
