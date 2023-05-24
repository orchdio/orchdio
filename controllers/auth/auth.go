package auth

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"log"
	"net/http"
	"net/url"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/queue"
	"orchdio/services"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
	"strconv"
	"strings"
	"time"
)

type AuthController struct {
	DB          *sqlx.DB
	AsyncClient *asynq.Client
	AsynqServer *asynq.Server
	AsynqRouter *asynq.ServeMux
	Redis       *redis.Client
}

func NewAuthController(db *sqlx.DB, asynqClient *asynq.Client, asynqServer *asynq.Server, asyqRouter *asynq.ServeMux, r *redis.Client) *AuthController {
	return &AuthController{DB: db, AsyncClient: asynqClient, AsynqServer: asynqServer, AsynqRouter: asyqRouter, Redis: r}
}

func (a *AuthController) AppAuthRedirect(ctx *fiber.Ctx) error {
	// we want to check the incoming platform redirect. we make sure its only valid for spotify apple and deezer
	platform := ctx.Locals("platform").(string)
	pubKey := ctx.Get("x-orchdio-public-key")
	aScopes := ctx.Query("scopes")

	if pubKey == "" {
		log.Printf("[controllers][AppAuthRedirect] developer -  error: no app id provided\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is not present. please pass your app id as a header")
	}

	// get the hostname
	hostname := ctx.Hostname()
	if hostname == "" {
		log.Printf("[controllers][AppAuthRedirect] developer -  error: no hostname provided\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Hostname is not present. please pass your hostname as a header")
	}

	if !lo.Contains([]string{"https://"}, hostname) {
		hostname = fmt.Sprintf("https://%s", hostname)
	}

	if aScopes == "" && platform != deezer.IDENTIFIER {
		log.Printf("[controllers][AppAuthRedirect] developer -  error: no scopes provided. Please pass the scope you want to request from the user\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "No scopes provided. Please pass the scope you want to request from the user")
	}

	authScopes := strings.Split(aScopes, ",")

	if platform == deezer.IDENTIFIER {
		authScopes = strings.Split(deezer.ValidScopes, ",")
	}

	// we always use this as the redirect url for the platform. the devs will put this in their redirect url on the platforms
	// they want to support.
	redirectURL := fmt.Sprintf("%s/v1/auth/%s/callback", hostname, platform)
	// we want to get the developer redirect url to redirect to after authenticating the user
	// after the user has been redirected to the platform auth page, the platform will redirect
	//  the user back to orchdio and orchdio will redirect the user back to the developer redirect url

	// in order to do this, we will create a jwt token with the redirect url and other information on the
	// operation and use it in the state of the original redirect url from the auth platform. In the auth
	// handler for the redirect url, we will decode/verify the jwt and redirect to the developer token there
	// The jwt token should have a lifetime of 5 - 15 minutes.

	// validate app id is a valid uuid
	isValid := util.IsValidUUID(pubKey)
	if !isValid {
		log.Printf("[controllers][AppAuthRedirect] developer -  error: invalid app id\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Invalid app id")
	}

	// get the redirect url of the developer
	database := db.NewDB{DB: a.DB}
	// TODO: check if the app is actually "active"
	developerApp, err := database.FetchAppByPublicKeyWithoutDevId(pubKey)
	if err != nil {
		log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to  fetch the developer app with pubKey %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	log.Printf("[controllers][AppAuthRedirect] developer -  developer app")

	// fetch the app integration integrationCredentials
	//var integrationCredentials []byte

	// create new AppAuthToken action
	var redirectToken = blueprint.AppAuthToken{
		App: developerApp.UID.String(),
		// FIXME (suggestion): maybe not encrypt this in the jwt but just the app id and then fetch app and app info from db using the app id
	}

	// create new action
	action := blueprint.Action{
		Payload: nil,
		Action:  "app_auth",
	}

	response := fiber.Map{
		"url": "",
	}

	redirectToken.Action = action
	redirectToken.RedirectURL = redirectURL
	redirectToken.Platform = platform
	redirectToken.Scopes = authScopes

	switch platform {
	case spotify.IDENTIFIER:
		if string(developerApp.SpotifyCredentials) == "" {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: no spotify integration integrationCredentials\n")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Spotify integration is not enabled for this app. Please make sure you update the app with your Spotify credentials")
		}

		credentials, err := util.Decrypt(developerApp.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to decrypt spotify integrationCredentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		var decryptedCredentials blueprint.IntegrationCredentials

		// deserialize the app integration integrationCredentials
		err = json.Unmarshal(credentials, &decryptedCredentials)
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to deserialize spotify integration integrationCredentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		encryptedToken, err := util.SignAuthJwt(&redirectToken)
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to generate auth token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authURL, err := spotify.FetchAuthURL(string(encryptedToken), redirectURL, authScopes, &decryptedCredentials)
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to fetch spotify auth url: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", err.Error())
		}

		response["url"] = string(authURL)
		return util.SuccessResponse(ctx, fiber.StatusOK, response)

	case deezer.IDENTIFIER:
		log.Printf("[controllers][AppAuthRedirect] developer -  redirecting to deezer auth url\n")
		redirectToken.Platform = "deezer"

		if string(developerApp.DeezerCredentials) == "" {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: deezer integration is not enabled for this app\n")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Deezer integration is not enabled for this app. Please make sure you update the app with your Deezer credentials")
		}

		var decryptedCredentials blueprint.IntegrationCredentials
		// decrypt the app integration integrationCredentials
		credentials, err := util.Decrypt(developerApp.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to decrypt deezer integrationCredentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// deserialize the app integration integrationCredentials
		err = json.Unmarshal(credentials, &decryptedCredentials)
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to deserialize deezer integration integrationCredentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// due to the fact that deezer doesnt support state in the url, we'll add the state as a query param to the redirect url
		redirectURL = fmt.Sprintf("%s?state=%s", redirectURL, developerApp.DeezerState)
		deezerAuth := deezer.NewDeezerAuth(decryptedCredentials.AppID, decryptedCredentials.AppSecret, redirectURL)
		authURL := deezerAuth.FetchAuthURL(authScopes)
		response["url"] = authURL
		return util.SuccessResponse(ctx, fiber.StatusOK, response)
	// TODO: handle apple music auth
	case applemusic.IDENTIFIER:
		log.Printf("[controllers][AppAuthRedirect] developer -  redirecting to apple music auth url\n")
		return util.ErrorResponse(ctx, fiber.StatusNotImplemented, "not supported", "Apple music auth not implemented yet")
	}

	return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurrednknown action in auth. perhaps not implemented yet")
}

// HandleAppAuthRedirect handles the redirect from the platform auth page to orchdio.
func (a *AuthController) HandleAppAuthRedirect(ctx *fiber.Ctx) error {
	// after the platform auths the user, it will redirect here
	// we will get the jwt (state) from the redirect url. first if there is no state, we do not
	// accept the redirect url. We verify the state and get the redirect url and other developer
	// information from the token.
	// if there is an action in the flow that the developer wants to perform —e.g. when the user wants
	// to add a new playlist to the user platform; we will perform the action here and or create another token
	// that contains this information, that is sent to developer redirect/webhook url. The developer
	// will then make a request to orchdio to perform the action.

	// ideally, we want to make the developer do as little as possible. We want to do as much as possible,
	// so maybe not add to token

	//requestID := ctx.Get("-orchdio-request-id")
	uniqueId := uuid.NewString()
	state := ctx.Query("state")
	code := ctx.Query("code")
	ctxPlatform := ctx.Params("platform")
	// this is for the error code that may be returned for deezer (only)
	errorCode := ctx.Query("error")
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" && ctxPlatform != "applemusic" {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: no state provided\n")
		if ctxPlatform == "deezer" {
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "bad request", "You must provide a state. A state is the same as your Orchdio app id and must be updated both in your app credentials on Orchdio and in your app profile on Deezer")
		}
		return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "no state present. please pass a state")
	}

	decodedState := &blueprint.AppAuthToken{}
	database := db.NewDB{DB: a.DB}

	// if the platform is not spotify, we decode the state. this is because
	// we use the same route for all platforms for auth and these platforms has some things we do differently
	// in this case, we do not encode state in auth url for apple music so we dont decode it here
	// and as for deezer, the state has to be passed statically in the redirect url as deezer does not support
	// state in the redirect url
	if !lo.Contains([]string{"applemusic", "deezer"}, ctxPlatform) {
		dec, err := util.DecodeAuthJwt(state)
		// decode state
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: could not decode the auth token: %v\n", err)
			if errors.Is(err, jwt.ErrTokenExpired) {
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "token expired")
			}
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "unable to decode state")
		}
		decodedState = dec
	} else {
		// since deezer does not support state, we cannot decode the states passed.
		// a suggested workaround for this would be to create a simple redis store
		// and when the user authorizes, an entry is added to the store. they key might be the
		// deezer state and some other information (e.g. xdj_#2-deezer-unix-timestamp) and the value
		// would be the scopes (and other information needed to be persisted). the entry would be set to expire
		// in 12 mins. this is 10mins which a typical auth url is supposed to last and 2 mins to account for systems and
		// unexpected delays. when the user is redirected, we would check if the state is in the store and if it is,
		// we would fetch the scopes and other information from the store and use it to create the auth token.
		// For now, it might not really be worth the implementation stress. on to the next thing.
		//
		// update(apr 15): its not so practical to use the above approach. we'd need to find a way to recognize each
		// request session in order to retrieve the corresponding scopes from redis. so for deezer, we always
		// use the same scopes, which are all the scopes that are available for deezer. this is not as wild as it
		// might sound, since deezer scopes are not that much, deezer apps are not that many and the scopes are pretty
		// much going to make sense to be available for all deezer apps, as deezer api is already quirky enough, perhaps
		// elimination scopes caused errors would be another advantage.
		if ctxPlatform == deezer.IDENTIFIER {
			// fetch the deezer app using the state. deezer redirects url is limited to 100 bytes. so we generate a shortid as state
			dState, err := url.QueryUnescape(state)
			if len(dState) > 10 {
				log.Print("[controllers][HandleAppAuthRedirect] developer -  error: state is too long")
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "invalid state.")
			}

			// check if the user has authorized deezer for this app
			//userAppRow := a.DB.QueryRowx(queries.FetchUserAppByPlatform, ctxPlatform)
			devApp, err := database.FetchAppByDeezerState(dState)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "invalid state. please try again")
				}
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch app by deezer state: %v\n", err)
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "unable to fetch app by deezer state")
			}

			decodedState = &blueprint.AppAuthToken{
				RegisteredClaims: jwt.RegisteredClaims{},
				App:              devApp.UID.String(),
				RedirectURL:      fmt.Sprintf("%s/v1/auth/deezer?state=%s", os.Getenv("APP_URL"), dState),
				Platform:         "deezer",
				Action: blueprint.Action{
					Payload: nil,
					Action:  "auth",
				},
				// devApp.Scopes,
				Scopes: strings.Split(deezer.ValidScopes, ","),
			}
		}
	}

	updatedUserCredentials := struct {
		Username   string `json:"username,omitempty"`
		Platform   string `json:"platform,omitempty"`
		PlatformId string `json:"platform_id,omitempty"`
		Token      []byte `json:"token,omitempty"`
	}{}

	var authedUserEmail string
	var userPlatformToken []byte
	// we fetch the developer app using the app id and grab the redirect url from there. this is different from the integration redirect url
	// which is the one the developers will put in their platform apps. this redirect url is an orchdio redirect url where the result of the auth
	// is redirected to, similar to how it would be if orchdio wasnt a middleman.

	// fetch the app that made the auth request in order to add to the task data, used for sending customized email
	// informing user that the app has been authorized.
	app, err := database.FetchAppByAppIdWithoutDevId(decodedState.App)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch app from database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	log.Printf("[controllers][HandleAppAuthRedirect] developer - App found. App authed is %s and is owned by org with uuid %s", app.Name, app.Organization)
	hostname := ctx.Hostname()
	if hostname == "" {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to get hostname from request\n")
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "Could not get hostname from request")
	}

	if !lo.Contains([]string{"https://"}, hostname) {
		hostname = fmt.Sprintf("https://%s", hostname)
	}

	userProfile := &blueprint.User{}

	// this is the original redirect url that the developer provided in the app creation process.
	// and its always in this format: https://orchdio.com/v1/auth/{platform}/callback. we will use this
	// to complete the auth flow on the authorizing platform. It is different from the developer app redirect url
	// as the dev app redirect url is the final redirect at the end of this flow/controller.
	redirectURL := fmt.Sprintf("%s/v1/auth/%s/callback", hostname, ctxPlatform)
	switch decodedState.Platform {
	// spotify auth flow
	case spotify.IDENTIFIER:
		// create a new http request to be used for the spotify auth
		r, err := http.NewRequest("GET", string(ctx.Request().RequestURI()), nil)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to create new http request for spotify auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		defer func() error {
			if err := recover(); err != nil {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  ⚠️ App panicked in HandleAppAuthRedirect.: %v\n", err)
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}
			return nil
		}()

		if app.SpotifyCredentials == nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: spotify credentials are nil\n")
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// decrypt the app's integration credentials
		decryptedIntegrationCredentials, err := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to decrypt spotify credentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		integrationCredentials := &blueprint.IntegrationCredentials{}
		err = json.Unmarshal(decryptedIntegrationCredentials, integrationCredentials)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to unmarshal spotify credentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		client, refreshToken, err := spotify.CompleteUserAuth(ctx.Context(), r, redirectURL, integrationCredentials)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete spotify user auth: %v\n", err)
			if err == blueprint.EINVALIDAUTHCODE {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: invalid auth code\n")
				return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid auth code")
			}

			if err.Error() == "invalid_grant" {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: invalid grant\n")
				return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid grant")
			}
			if err.Error() == "invalid_client" {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: invalid client\n")
				return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid client. please make sure the client id is valid")
			}
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete spotify user auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		log.Printf("[controllers][HandleAppAuthRedirect] developer -  Spotify user auth completed successfully.. Refresh token to be encrypted: %v \n", refreshToken)

		encryptedRefreshToken, err := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt spotify refresh token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		user, err := client.CurrentUser(ctx.Context())
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to get current user during auth: %v\n", err)
			if strings.Contains(err.Error(), "User not registered") {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: user not registered\n")
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "You have not been added to the waitlist for access to spotify. Please join the waitlist and try again.")
			}
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		newUser := a.DB.QueryRowx(queries.CreateUserQuery, user.Email, uniqueId)
		err = newUser.StructScan(userProfile)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan user during final auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// set the usernames and platform ids for spotify
		//userNames.Spotify = user.DisplayName
		//userPlatformIds.Spotify = user.ID

		updatedUserCredentials.Username = user.DisplayName
		updatedUserCredentials.PlatformId = user.ID
		updatedUserCredentials.Platform = "spotify"
		updatedUserCredentials.Token = encryptedRefreshToken

		userPlatformToken = encryptedRefreshToken

		t := blueprint.OrchdioUserToken{
			RegisteredClaims: jwt.RegisteredClaims{},
			Email:            userProfile.Email,
			Username:         user.DisplayName,
			UUID:             userProfile.UUID,
			Platform:         "spotify",
		}

		_, err = util.SignJwt(&t)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign spotify auth jwt: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authedUserEmail = userProfile.Email
		// update the redirect url to the developer app redirect url. this is the final redirect url at the end of the auth flow
		redirectURL = fmt.Sprintf("%s?state=%s", app.RedirectURL, app.UID.String())

		// deezer auth flow
	case deezer.IDENTIFIER:
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  deezer auth flow")
		if code == "" {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: no code provided for deezer auth.\n")
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "no code present. please pass a code")
		}

		if errorCode == "access_denied" {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: deezer returned an %v error\n", errorCode)
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "Deezer app access denied")
		}

		var deezerCredentials blueprint.IntegrationCredentials
		creds, err := util.Decrypt(app.DeezerCredentials, []byte(encryptionSecretKey))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to decrypt deezer credentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		err = json.Unmarshal(creds, &deezerCredentials)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to unmarshal deezer credentials: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		deezerAuth := deezer.NewDeezerAuth(deezerCredentials.AppID, deezerCredentials.AppSecret, redirectURL)
		deezerToken := deezerAuth.FetchAccessToken(code)
		deezerUser, err := deezerAuth.CompleteUserAuth(deezerToken)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete deezer auth. could not fetch deezer complete auth token: %v\n", err)
			if err == blueprint.EINVALIDPERMISSIONS {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: deezer returned an invalid permissions error\n")
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "Deezer app access denied. Please check the permissions passed")
			}

			if err == blueprint.ESERVICECLOSED {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: deezer returned a service closed error. free service closed\n")
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "Deezer app access denied. Free service closed")
			}
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		if deezerUser == nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: deezer user is empty. This is not expected at this point.\n")
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		encryptedRefreshToken, err := util.Encrypt(deezerToken, []byte(encryptionSecretKey))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt refresh token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		deezerID := strconv.Itoa(deezerUser.ID)
		newUser := a.DB.QueryRowx(queries.CreateUserQuery, deezerUser.Email, uniqueId)

		err = newUser.StructScan(userProfile)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan deezer user during final auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		updatedUserCredentials.Username = deezerUser.Name
		updatedUserCredentials.PlatformId = deezerID
		updatedUserCredentials.Platform = "deezer"
		updatedUserCredentials.Token = encryptedRefreshToken

		userPlatformToken = encryptedRefreshToken

		// generate jwt token for orchdio labs
		t := blueprint.OrchdioUserToken{
			RegisteredClaims: jwt.RegisteredClaims{},
			Email:            deezerUser.Email,
			Username:         deezerUser.Name,
			UUID:             userProfile.UUID,
			Platform:         "deezer",
		}

		authToken, err := util.SignJwt(&t)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign deezer auth jwt: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authedUserEmail = deezerUser.Email
		redirectURL = fmt.Sprintf("%s?token=%s", app.RedirectURL, string(authToken))

		// apple music auth flow
	case applemusic.IDENTIFIER:
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  apple music auth flow")
		body := &blueprint.AppleMusicAuthBody{}
		err := ctx.BodyParser(body)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to parse apple music auth body: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		state := body.State
		if state == "" {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: no state provided in apple music auth body\n")
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "no state present. please pass a state for apple music auth")
		}

		var displayName = "-"
		if body.FirstName != "" {
			displayName = fmt.Sprintf("%v %v", body.FirstName, body.LastName)
		}
		encryptedRefreshToken, err := util.Encrypt([]byte(body.Token), []byte(encryptionSecretKey))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt user apple music refresh token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// apple doesn't (seem to) have a context of user ID here, in the API, we're using the music user token and
		// developer tokens to auth and make user auth requests. Therefore, we'll simply generate an md5 hash of the
		// email address and use that as the user ID.
		hash := md5.New()
		_, err = hash.Write([]byte(body.Email))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to hash apple music user email: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		hashedEmail := hex.EncodeToString(hash.Sum(nil))
		userProfile := &blueprint.User{}
		newUser := a.DB.QueryRowx(queries.CreateUserQuery, hashedEmail, uniqueId)

		err = newUser.StructScan(userProfile)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan apple music user during final auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		updatedUserCredentials.Username = displayName
		updatedUserCredentials.PlatformId = hashedEmail
		updatedUserCredentials.Platform = "applemusic"
		updatedUserCredentials.Token = encryptedRefreshToken

		userPlatformToken = encryptedRefreshToken

		t := blueprint.OrchdioUserToken{
			RegisteredClaims: jwt.RegisteredClaims{},
			Email:            body.Email,
			Username:         displayName,
			UUID:             userProfile.UUID,
			Platform:         "applemusic",
		}

		authToken, err := util.SignJwt(&t)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign apple music auth jwt: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authedUserEmail = body.Email

		redirectURL = fmt.Sprintf("%v?token=%v", app.RedirectURL, authToken)
	// redirect to the redirect url from the state token.
	default:
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: invalid platform: %v\n", decodedState.Platform)
		return util.ErrorResponse(ctx, fiber.StatusNotImplemented, "unauthorized", "invalid platform")
	}

	log.Printf("[controllers][HandleAppAuthRedirect][debug] app authing user is -  app: %v\n", app.Name)
	// create new user app
	userAppController := services.NewUserDevAppController(a.DB)
	userAppData := &blueprint.CreateNewUserAppData{
		Platform:     decodedState.Platform,
		RefreshToken: userPlatformToken,
		Scopes:       decodedState.Scopes,
		App:          app.UID,
		User:         userProfile.UUID,
	}
	// userAppIDBytes might be nil and error nil at the same time. in this case it means that the user already has an app,
	// and we just updated the refresh token and scopes.
	userAppIDBytes, err := userAppController.CreateOrUpdateUserApp(userAppData)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to create user app: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}
	if userAppIDBytes != nil {
		log.Printf("[controllers][HandleAppAuthRedirect][debug] created new app %s for user %v\n", string(userAppIDBytes), authedUserEmail)
	}

	_, err = a.DB.Exec(queries.UpdatePlatformUserNameIdAndToken, updatedUserCredentials.Username, updatedUserCredentials.PlatformId,
		updatedUserCredentials.Token, userProfile.UUID.String(), updatedUserCredentials.Platform)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to update platform usernames: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	// create a new user app
	taskID := uuid.New().String()

	// an app task data struct. a task is a job that is scheduled to be executed at a later time.
	// this is used to email the user that just authed with the app.
	taskApp := &blueprint.AppTaskData{
		Name: app.Name,
		UUID: app.UID.String(),
	}

	// we need to let the user know what just happened, so we'll send them an email.
	// And what just happened? We let them know that the App they just authed uses our service in order to
	// request data, in order to make it easier for them to integrate multiple platforms into their app.
	// We let them know that we do store some of their data but it is the apis that the app needs to function.
	// Their permission is easily revocable by going to their account settings and removing the app and going to
	// https://orchdio.com/my-data and deleting their data.
	var emailTaskData = &blueprint.EmailTaskData{
		App:  taskApp,
		From: os.Getenv("ALERT_EMAIL"),
		To:   authedUserEmail,
		Payload: map[string]interface{}{
			"APP_NAME": app.Name,
			"PLATFORM": strings.Title(decodedState.Platform),
			"SCOPES":   services.BuildScopesExplanation(decodedState.Scopes, decodedState.Platform),
		},
		TaskID:     taskID,
		TemplateID: 1,
	}
	// schedule a job to send notification email
	emailQueue := queue.NewOrchdioQueue(a.AsyncClient, a.DB, a.Redis, a.AsynqRouter)
	// serialize the task data
	serializedEmailTaskData, err := json.Marshal(emailTaskData)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to serialize email task data: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	emailTask, err := emailQueue.NewTask(fmt.Sprintf("send:appauth:email:%s", taskID), queue.EmailTask, 3, serializedEmailTaskData)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to create email task: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	// enqueue the task
	err = emailQueue.EnqueueTask(emailTask, queue.EmailQueue, taskID, time.Second*5)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to enqueue email task: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	// we dont really need to create a new task in the db since it is not a task that contains the data needed by a developer
	// set the task handler
	log.Printf("[controllers][HandleAppAuthRedirect][debug] user authorization and authentication done\n")
	return ctx.Redirect(redirectURL, fiber.StatusTemporaryRedirect)
}
