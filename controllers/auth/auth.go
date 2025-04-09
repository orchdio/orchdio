package auth

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	logger2 "orchdio/logger"
	"orchdio/queue"
	"orchdio/services"
	"orchdio/services/applemusic"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type Controller struct {
	DB          *sqlx.DB
	AsyncClient *asynq.Client
	AsynqServer *asynq.Server
	AsynqRouter *asynq.ServeMux
	Redis       *redis.Client
}

func NewAuthController(db *sqlx.DB, asynqClient *asynq.Client, asynqServer *asynq.Server, asyqRouter *asynq.ServeMux, r *redis.Client) *Controller {
	return &Controller{DB: db, AsyncClient: asynqClient, AsynqServer: asynqServer, AsynqRouter: asyqRouter, Redis: r}
}

// AppAuthRedirect is called when an application performs authorization and authentication for a specific platform.
// This controller takes care of the various auth cases and how they work and tries as best as possible to unify their
// behaviour.
func (a *Controller) AppAuthRedirect(ctx *fiber.Ctx) error {
	// we want to check the incoming platform redirect. we make sure its only valid for spotify apple and deezer
	platform := ctx.Locals("platform").(string)
	pubKey := ctx.Get("x-orchdio-public-key")
	appScopes := ctx.Query("scopes")
	developerPubKey := ctx.Locals("app_pub_key")

	// fixme: this seems redundant in most cases i am using it. remove where unnecessary and consider the possible useful cases.
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		ApplicationPublicKey: zap.String("pubkey:", developerPubKey.(string)).String,
		Platform:             platform,
	}

	logger := logger2.NewZapSentryLogger(loggerOpts)
	if pubKey == "" {
		logger.Error("[controllers][AppAuthRedirect] developer -  error: no app id provided\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is not present. please pass your app id as a header")
	}

	// get the hostname
	hostname := ctx.Hostname()
	if hostname == "" {
		logger.Error("[controllers][AppAuthRedirect] developer -  error: no hostname provided")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Hostname is not present. please pass your hostname as a header")
	}

	if !lo.Contains([]string{"https://"}, hostname) {
		hostname = fmt.Sprintf("https://%s", hostname)
	}

	if appScopes == "" && platform != deezer.IDENTIFIER {
		logger.Error("[controllers][AppAuthRedirect] developer -  error: no scopes provided while trying to connect platform.", zap.String("platform", platform))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "No scopes provided. Please pass the scope you want to request from the user")
	}

	authScopes := strings.Split(appScopes, ",")
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
		logger.Error("[controllers][AppAuthRedirect] developer -  error: invalid public key", zap.String("public_key", pubKey))
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Invalid public key")
	}

	// get the redirect url of the developer
	database := db.NewDB{DB: a.DB}

	// TODO: check if the app is actually "active"
	developerApp, err := database.FetchAppByPublicKeyWithoutDevId(pubKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
		}
		logger.Error("[controllers][AppAuthRedirect] developer -  error: unable to  fetch the developer app with pubKey", zap.Error(err), zap.String("public_key", pubKey))
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	logger.Info("[controllers][AppAuthRedirect] developer -  fetched developer app", zap.String("app_name", developerApp.Name))

	// create new AppAuthToken action
	var redirectToken = blueprint.AppAuthToken{
		App: developerApp.UID.String(),
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
			logger.Error("[controllers][AppAuthRedirect] developer -  error: spotify integration is not enabled for this app")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Spotify integration is not enabled for this app. Please make sure you update the app with your Spotify credentials")
		}

		credentials, decErr := util.Decrypt(developerApp.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			logger.Error("[controllers][AppAuthRedirect] developer -  error: unable to decrypt spotify integrationCredentials", zap.Error(decErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		var decryptedCredentials blueprint.IntegrationCredentials

		// deserialize the app integration integrationCredentials
		serErr := json.Unmarshal(credentials, &decryptedCredentials)
		if serErr != nil {
			logger.Error("[controllers][AppAuthRedirect] developer -  error: unable to deserialize spotify integrationCredentials", zap.Error(serErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		encryptedToken, sErr := util.SignAuthJwt(&redirectToken)
		if sErr != nil {
			logger.Error("[controllers][AppAuthRedirect] developer -  error: unable to sign spotify auth jwt", zap.Error(sErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authURL, fErr := spotify.FetchAuthURL(string(encryptedToken), redirectURL, authScopes, &decryptedCredentials)
		if fErr != nil {
			logger.Error("[controllers][AppAuthRedirect] developer -  error: unable to fetch spotify auth url", zap.Error(fErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", fErr.Error())
		}

		response["url"] = string(authURL)
		return util.SuccessResponse(ctx, fiber.StatusOK, response)

	case deezer.IDENTIFIER:
		redirectToken.Platform = deezer.IDENTIFIER
		if string(developerApp.DeezerCredentials) == "" {
			logger.Error("[controllers][AppAuthRedirect] developer -  error: deezer integration is not enabled for this app")
			return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Deezer integration is not enabled for this app. Please make sure you update the app with your Deezer credentials")
		}

		var decryptedCredentials blueprint.IntegrationCredentials
		// decrypt the app integration integrationCredentials
		credentials, decErr := util.Decrypt(developerApp.DeezerCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
		if decErr != nil {
			logger.Error("[controllers][AppAuthRedirect] developer -  error: unable to decrypt deezer integrationCredentials", zap.Error(decErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// deserialize the app integration integrationCredentials
		dErr := json.Unmarshal(credentials, &decryptedCredentials)
		if dErr != nil {
			logger.Error("[controllers][AppAuthRedirect] developer -  error: unable to deserialize deezer integrationCredentials", zap.Error(dErr))
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
		logger.Warn("[controllers][AppAuthRedirect] developer -  error: apple music auth not implemented yet")
		return util.ErrorResponse(ctx, fiber.StatusNotImplemented, "not supported", "Apple music auth not implemented yet")
	}

	return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred in auth. perhaps not implemented yet")
}

// HandleAppAuthRedirect handles the redirect from the platform auth page to orchdio.
func (a *Controller) HandleAppAuthRedirect(ctx *fiber.Ctx) error {

	updatedUserCredentials := struct {
		Username   string `json:"username,omitempty"`
		Platform   string `json:"platform,omitempty"`
		PlatformId string `json:"platform_id,omitempty"`
		Token      []byte `json:"token,omitempty"`
	}{}

	developerPubKey := ctx.Locals("app_pub_key")
	platform := ctx.Locals("platform").(string)

	reqId := ctx.Get("x-orchdio-request-id")
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            reqId,
		ApplicationPublicKey: zap.String("pubkey:", developerPubKey.(string)).String,
		Platform:             platform,
	}

	logger := logger2.NewZapSentryLogger(loggerOpts)

	// if the verb is POST, it means that the auth is most likely Apple Music auth, so we'll handle it differently
	if ctx.Method() == "POST" {
		// apple music auth flow
		logger.Info("[controllers][HandleAppAuthRedirect] developer -  handling apple music auth flow")
		uniqueID := uuid.NewString()
		encryptionSecret := os.Getenv("ENCRYPTION_SECRET")
		// todo: add a new "verified_email" column in the db
		// and use the result here (if present) to determine
		// apple music auth verification. if the email is not verified,
		// we'll send an email to the user to verify their email
		body := &blueprint.AppleMusicAuthBody{}
		err := ctx.BodyParser(body)
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to parse apple music auth body", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		database := db.NewDB{DB: a.DB}
		state := body.State

		if state == "" {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: no state present. please pass a state for apple music auth")
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "no state present. please pass a state for apple music auth")
		}

		// ensure the incoming app exists
		developerApp, fErr := database.FetchAppByPublicKeyWithoutDevId(body.App)
		if fErr != nil {
			if errors.Is(fErr, sql.ErrNoRows) {
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "invalid app. please make sure that the public key belongs to an existing app.")
			}
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch developer app", zap.Error(fErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		var displayName = "-"
		if body.FirstName != "" {
			displayName = fmt.Sprintf("%v %v", body.FirstName, body.LastName)
		}
		encryptedRefreshToken, err := util.Encrypt([]byte(body.MusicToken), []byte(encryptionSecret))
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt apple music user token", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// apple doesn't (seem to) have a context of user ID here, in the API, we're using the music user token and
		// developer tokens to auth and make user auth requests. Therefore, we'll simply generate an md5 hash of the
		// email address and use that as the user ID.
		hash := md5.New()
		_, err = hash.Write([]byte(body.Email))
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to hash apple music user email", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		hashedEmail := hex.EncodeToString(hash.Sum(nil))
		userProfile := &blueprint.User{}
		newUser := a.DB.QueryRowx(queries.CreateUserQuery, body.Email, uniqueID)

		err = newUser.StructScan(userProfile)
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to create new user during final Auth step", zap.Error(err), zap.String("platform", "apple_music"))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		updatedUserCredentials.Username = displayName
		updatedUserCredentials.PlatformId = hashedEmail
		updatedUserCredentials.Platform = applemusic.IDENTIFIER
		updatedUserCredentials.Token = encryptedRefreshToken

		var userPlatformToken = encryptedRefreshToken

		t := blueprint.OrchdioUserToken{
			RegisteredClaims: jwt.RegisteredClaims{},
			Email:            body.Email,
			Username:         displayName,
			UUID:             userProfile.UUID,
			Platform:         applemusic.IDENTIFIER,
		}

		authToken, err := util.SignJwt(&t)
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign apple music auth jwt", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		var authedUserEmail = body.Email

		userAppController := services.NewUserDevAppController(a.DB)
		userAppData := &blueprint.CreateNewUserAppData{
			RefreshToken: userPlatformToken,
			Scopes:       []string{"email", "name"},
			User:         userProfile.UUID,
			App:          developerApp.UID,
			Platform:     applemusic.IDENTIFIER,
		}

		userAppIDBytes, cErr := userAppController.CreateOrUpdateUserApp(userAppData)
		if cErr != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to create new or update apple music app", zap.Error(cErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		if userAppIDBytes != nil {
			logger.Warn("[controllers][HandleAppAuthRedirect] developer - User has not created an apple music app before. Created one",
				zap.String("user_id", userProfile.UUID.String()), zap.String("username", displayName))
		}
		_, err = a.DB.Exec(queries.UpdatePlatformUserNameIdAndToken, updatedUserCredentials.Username,
			updatedUserCredentials.PlatformId, updatedUserCredentials.Token, userProfile.UUID.String(), updatedUserCredentials.Platform, string(userAppIDBytes))
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to update user credentials", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		logger.Info("[controllers][HandleAppAuthRedirect] developer -  user authorization and authentication done")

		newUserApp, fErr := database.FetchAppByPublicKeyWithoutDevId(body.App)
		if fErr != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch user app", zap.Error(fErr))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		taskID := uuid.NewString()
		taskApp := &blueprint.AppTaskData{
			Name: newUserApp.Name,
			UUID: newUserApp.UID.String(),
		}

		var emailTaskData = &blueprint.EmailTaskData{
			App:  taskApp,
			From: os.Getenv("ALERT_EMAIL"),
			To:   authedUserEmail,
			Payload: map[string]interface{}{
				"APP_NAME": newUserApp.Name,
				"PLATFORM": strings.Title(applemusic.IDENTIFIER),
				"SCOPES":   services.BuildScopesExplanation([]string{"email", "name"}, applemusic.IDENTIFIER),
			},
			TaskID:     taskID,
			TemplateID: 2,
		}
		// schedule a job to send notification email
		_ = queue.NewOrchdioQueue(a.AsyncClient, a.DB, a.Redis, a.AsynqRouter)
		// serialize the task data
		_, err = json.Marshal(emailTaskData)
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to serialize email task data", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		logger.Info("[controllers][HandleAppAuthRedirect] developer - Authentication done. sending email notification")
		ctx.Accepts("access-control-allow-credentials", "true")
		return util.SuccessResponse(ctx, fiber.StatusOK, map[string]interface{}{
			"token": string(authToken),
		})
	}

	// GET is for all other platforms except Apple Music
	if ctx.Method() == "GET" {
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

		uniqueId := uuid.NewString()
		state := ctx.Query("state")
		code := ctx.Query("code")
		ctxPlatform := ctx.Params("platform")
		// this is for the error code that may be returned for deezer (only)
		errorCode := ctx.Query("error")
		encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
		if state == "" && ctxPlatform != applemusic.IDENTIFIER {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: no state present. please pass a state")
			if ctxPlatform == deezer.IDENTIFIER {
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
		if !lo.Contains([]string{applemusic.IDENTIFIER, deezer.IDENTIFIER}, ctxPlatform) {
			dec, err := util.DecodeAuthJwt(state)
			// decode state
			if err != nil {
				if errors.Is(err, jwt.ErrTokenExpired) {
					logger.Warn("[controllers][HandleAppAuthRedirect] developer -  error: token expired", zap.Error(err), zap.String("platform", ctxPlatform))
					return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "token expired")
				}
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: could not decode the auth token", zap.Error(err))
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
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: invalid state. Too long", zap.String("state", dState))
					return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "invalid state.")
				}

				// check if the user has authorized deezer for this app
				devApp, fErr := database.FetchAppByDeezerState(dState)
				if fErr != nil {
					if errors.Is(fErr, sql.ErrNoRows) {
						logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch app by deezer state", zap.Error(fErr))
						return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "invalid state. please try again")
					}
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch app by deezer state", zap.Error(err))
					return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "unable to fetch app by deezer state")
				}

				decodedState = &blueprint.AppAuthToken{
					RegisteredClaims: jwt.RegisteredClaims{},
					App:              devApp.UID.String(),
					RedirectURL:      fmt.Sprintf("%s/v1/auth/deezer?state=%s", os.Getenv("APP_URL"), dState),
					Platform:         deezer.IDENTIFIER,
					Action: blueprint.Action{
						Payload: nil,
						Action:  "auth",
					},
					Scopes: strings.Split(deezer.ValidScopes, ","),
				}
			}
		}

		var authedUserEmail string
		var userPlatformToken []byte
		// we fetch the developer app using the app id and grab the redirect url from there. this is different from the integration redirect url
		// which is the one the developers will put in their platform apps. this redirect url is an orchdio redirect url where the result of the auth
		// is redirected to, similar to how it would be if orchdio wasnt a middleman.

		// fetch the app that made the auth request in order to add to the task data, used for sending customized email
		// informing user that the app has been authorized.
		app, err := database.FetchAppByAppIdWithoutDevId(decodedState.App)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
			}
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch app by app id", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		logger.Info("[controllers][HandleAppAuthRedirect] developer - App found.", zap.String("app_name", app.Name), zap.String("app_uuid", app.UID.String()), zap.String("Organization", app.Organization))
		hostname := ctx.Hostname()
		if hostname == "" {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: no hostname provided")
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
			r, rErr := http.NewRequest("GET", string(ctx.Request().RequestURI()), nil)
			if rErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to create new http request for spotify auth", zap.Error(rErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			defer func() error {
				if zErr := recover(); zErr != nil {
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: app panicked in HandleAppAuthRedirect")
					return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
				}
				return nil
			}()

			if app.SpotifyCredentials == nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: No spotify credentials found for app. Please add spotify credential", zap.String("app_pubkey", app.PublicKey.String()))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			// decrypt the app's integration credentials
			decryptedIntegrationCredentials, dErr := util.Decrypt(app.SpotifyCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
			if dErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to decrypt spotify credentials", zap.Error(dErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}
			integrationCredentials := &blueprint.IntegrationCredentials{}
			err = json.Unmarshal(decryptedIntegrationCredentials, integrationCredentials)
			if err != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to unmarshal spotify credentials", zap.Error(err))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			client, refreshToken, cErr := spotify.CompleteUserAuth(ctx.Context(), r, redirectURL, integrationCredentials)
			if cErr != nil {
				// possible spotify auth errors.
				if errors.Is(err, blueprint.ErrInvalidAuthCode) {
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: invalid auth code")
					return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid auth code")
				}

				if err.Error() == blueprint.ErrSpotifyInvalidGrant {
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: invalid grant")
					return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid grant")
				}
				if err.Error() == blueprint.ErrSpotifyInvalidClient {
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: invalid client")
					return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid client. please make sure the client id is valid")
				}
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete spotify user auth", zap.Error(cErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			encryptedRefreshToken, rErr := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
			if rErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt spotify refresh token", zap.Error(rErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			user, uErr := client.CurrentUser(ctx.Context())
			if uErr != nil {
				if strings.Contains(uErr.Error(), blueprint.ErrSpotifyUserNotRegistered) {
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: user not registered", zap.Error(uErr))
					return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "You have not been added to the waitlist for access to spotify. Please join the waitlist and try again.")
				}
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to get current user during auth", zap.Error(uErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			newUser := a.DB.QueryRowx(queries.CreateUserQuery, user.Email, uniqueId)
			scErr := newUser.StructScan(userProfile)
			if scErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan user during final auth", zap.Error(scErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			// set the usernames and platform ids for spotify
			updatedUserCredentials.Username = user.DisplayName
			updatedUserCredentials.PlatformId = user.ID
			updatedUserCredentials.Platform = spotify.IDENTIFIER
			updatedUserCredentials.Token = encryptedRefreshToken

			userPlatformToken = encryptedRefreshToken

			t := blueprint.OrchdioUserToken{
				RegisteredClaims: jwt.RegisteredClaims{},
				Email:            userProfile.Email,
				Username:         user.DisplayName,
				UUID:             userProfile.UUID,
				Platform:         spotify.IDENTIFIER,
			}

			authT, sErr := util.SignJwt(&t)
			if sErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign spotify auth jwt", zap.Error(sErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}
			authedUserEmail = userProfile.Email
			// update the redirect url to the developer app redirect url. this is the final redirect url at the end of the auth flow
			redirectURL = fmt.Sprintf("%s?token=%s", app.RedirectURL, string(authT))

			// deezer auth flow
		case deezer.IDENTIFIER:
			logger.Info("[controllers][HandleAppAuthRedirect] developer -  handling deezer auth flow")
			if state == "" {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: no state present. please pass a state")
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "no code present. please pass a code")
			}

			if errorCode == blueprint.ErrDeezerAccessDenied {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: deezer returned an error", zap.String("error", errorCode))
				return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "Deezer app access denied")
			}

			var deezerCredentials blueprint.IntegrationCredentials
			creds, credErr := util.Decrypt(app.DeezerCredentials, []byte(encryptionSecretKey))
			if credErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to decrypt deezer credentials", zap.Error(credErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			err = json.Unmarshal(creds, &deezerCredentials)
			if err != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to unmarshal deezer credentials", zap.Error(err))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			deezerAuth := deezer.NewDeezerAuth(deezerCredentials.AppID, deezerCredentials.AppSecret, redirectURL)
			deezerToken := deezerAuth.FetchAccessToken(code)
			deezerUser, aErr := deezerAuth.CompleteUserAuth(deezerToken)
			if aErr != nil {
				if errors.Is(err, blueprint.ErrInvalidPermissions) {
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: deezer returned an invalid permissions error", zap.Error(aErr))
					return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "Deezer app access denied. Please check the permissions passed")
				}

				if errors.Is(err, blueprint.ErrServiceClosed) {
					logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: deezer returned a service closed error. free service closed", zap.Error(aErr))
					return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "Deezer app access denied. Free service closed")
				}

				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete deezer auth", zap.Error(aErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			if deezerUser == nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: deezer user is empty. This is not expected at this point.")
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			encryptedRefreshToken, encErr := util.Encrypt(deezerToken, []byte(encryptionSecretKey))
			if encErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt deezer refresh token", zap.Error(encErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			deezerID := strconv.Itoa(deezerUser.ID)
			newUser := a.DB.QueryRowx(queries.CreateUserQuery, deezerUser.Email, uniqueId)

			err = newUser.StructScan(userProfile)
			if err != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan deezer user during final auth", zap.Error(err))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}

			updatedUserCredentials.Username = deezerUser.Name
			updatedUserCredentials.PlatformId = deezerID
			updatedUserCredentials.Platform = deezer.IDENTIFIER
			updatedUserCredentials.Token = encryptedRefreshToken

			userPlatformToken = encryptedRefreshToken

			// generate jwt token for orchdio labs
			t := blueprint.OrchdioUserToken{
				RegisteredClaims: jwt.RegisteredClaims{},
				Email:            deezerUser.Email,
				Username:         deezerUser.Name,
				UUID:             userProfile.UUID,
				Platform:         deezer.IDENTIFIER,
			}

			authToken, sErr := util.SignJwt(&t)
			if sErr != nil {
				logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign deezer auth jwt", zap.Error(sErr))
				return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
			}
			authedUserEmail = deezerUser.Email
			redirectURL = fmt.Sprintf("%s?token=%s", app.RedirectURL, string(authToken))

		// redirect to the redirect url from the state token.
		default:
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: invalid platform", zap.String("platform", decodedState.Platform))
			return util.ErrorResponse(ctx, fiber.StatusNotImplemented, "unauthorized", "invalid platform")
		}

		logger.Info("[controllers][HandleAppAuthRedirect] developer -  App authenticating user is", zap.String("app_name", app.Name))
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
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to create user app", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		if userAppIDBytes != nil {
			logger.Info("[controllers][HandleAppAuthRedirect] developer -  User has not created an app before. Created one.", zap.String("appInfo", app.UID.String()),
				zap.String("platform", decodedState.Platform))
		}

		_, err = a.DB.Exec(queries.UpdatePlatformUserNameIdAndToken, updatedUserCredentials.Username, updatedUserCredentials.PlatformId,
			updatedUserCredentials.Token, userProfile.UUID.String(), updatedUserCredentials.Platform, string(userAppIDBytes))
		if err != nil {
			logger.Error("[controllers][HandleAppAuthRedirect] developer -  error: unable to update platform usernames", zap.Error(err))
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// create a new user email task
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
		var _ = &blueprint.EmailTaskData{
			App:  taskApp,
			From: os.Getenv("ALERT_EMAIL"),
			To:   authedUserEmail,
			Payload: map[string]interface{}{
				"APP_NAME": app.Name,
				"PLATFORM": strings.Title(decodedState.Platform),
				"SCOPES":   services.BuildScopesExplanation(decodedState.Scopes, decodedState.Platform),
			},
			TaskID:     taskID,
			TemplateID: 2,
		}
		// schedule a job to send notification email
		_ = queue.NewOrchdioQueue(a.AsyncClient, a.DB, a.Redis, a.AsynqRouter)
		logger.Info("[controllers][HandleAppAuthRedirect] developer -  user authorization and authentication done. Redirecting")
		return ctx.Redirect(redirectURL, fiber.StatusTemporaryRedirect)
	}

	return util.ErrorResponse(ctx, fiber.StatusNotImplemented, "not implemented", "invalid auth verb")
}
