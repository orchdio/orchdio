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
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/queue"
	"orchdio/services/deezer"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
	"regexp"
	"strconv"
	"strings"
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
	platform := ctx.Params("platform")
	pubKey := ctx.Get("x-orchdio-public-key")
	//developer := ctx.Locals("developer").(blueprint.App)
	reg := regexp.MustCompile(`spotify|apple|deezer`)

	if !reg.MatchString(platform) {
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "Invalid platform")
	}

	if pubKey == "" {
		log.Printf("[controllers][AppAuthRedirect] developer -  error: no app id provided\n")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "App ID is not present. please pass your app id as a header")
	}

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
	developerApp, err := database.FetchAppByPublicKey(pubKey)
	if err != nil {
		log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to  fetch the developer app with pubKey %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "not found", "App not found")
		}
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	// create new AppAuthToken action
	var redirectToken = blueprint.AppAuthToken{
		App: developerApp.UID.String(),
		// FIXME (suggestion): maybe not encrypt this in the jwt but just the app id and then fetch app and app info from db using the app id
		RedirectURL: developerApp.RedirectURL,
	}

	// create new action
	action := blueprint.Action{
		Payload: nil,
		Action:  "app_auth",
	}

	response := fiber.Map{
		"url": "",
	}

	switch platform {
	case "spotify":

		redirectToken.Platform = "spotify"
		redirectToken.Action = action
		encryptedToken, err := util.SignAuthJwt(&redirectToken)
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to generate auth token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authURL := spotify.FetchAuthURL(string(encryptedToken))

		response["url"] = string(authURL)
		return util.SuccessResponse(ctx, fiber.StatusOK, response)

	case "deezer":
		log.Printf("[controllers][AppAuthRedirect] developer -  redirecting to deezer auth url\n")
		redirectToken.Platform = "deezer"
		encryptedToken, err := util.SignAuthJwt(&redirectToken)
		if err != nil {
			log.Printf("[controllers][AppAuthRedirect] developer -  error: unable to generate auth token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		deezerSecret := os.Getenv("DEEZER_SECRET")
		deezerRedirectURL := os.Getenv("DEEZER_REDIRECT_URL")
		deezerClientID := os.Getenv("DEEZER_CLIENT_ID")

		deezerAuth := deezer.NewDeezerAuth(deezerClientID, deezerSecret, deezerRedirectURL)
		authURL := deezerAuth.FetchAuthURL(string(encryptedToken))
		response["url"] = authURL
		return util.SuccessResponse(ctx, fiber.StatusOK, response)

	// TODO: handle apple music auth later. test the flow using zoove as developer and implement flow from there
	//   in accordance with new auth flow.
	case "applemusic":
		log.Printf("[controllers][AppAuthRedirect] developer -  redirecting to apple music auth url\n")
		return util.ErrorResponse(ctx, fiber.StatusNotImplemented, "not supported", "Apple music auth not implemented yet")
	}

	return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurrednknown action in auth. perhaps not implemented yet")
}

// HandleAppAuthRedirect handles the redirect from the platform auth page to orchdio
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

	uniqueId := uuid.NewString()
	state := ctx.Query("state")
	code := ctx.Query("code")
	// this is for the error code that may be returned for deezer (only)
	errorCode := ctx.Query("error")
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: no state provided\n")
		return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "no state present. please pass a state")
	}

	// decode state
	decodedState, err := util.DecodeAuthJwt(state)
	redirectURL := ""
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: could not decode the auth token: %v\n", err)
		if errors.Is(err, jwt.ErrTokenExpired) {
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "token expired")
		}
		return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "unable to decode state")
	}

	var authedUserEmail string

	switch decodedState.Platform {
	// spotify auth flow
	case "spotify":
		// create a new http request to be used for the spotify auth
		r, err := http.NewRequest("GET", string(ctx.Request().RequestURI()), nil)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to create new http request for spotify auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		defer func() {
			if err := recover(); err != nil {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to close request body: %v\n", err)
			}
		}()
		client, refreshToken, err := spotify.CompleteUserAuth(ctx.Context(), r)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete spotify user auth: %v\n", err)
			if err == blueprint.EINVALIDAUTHCODE {
				log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: invalid auth code\n")
				return util.ErrorResponse(ctx, fiber.StatusBadRequest, "bad request", "invalid auth code")
			}
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete spotify user auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		encryptedRefreshToken, err := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt refresh token: %v\n", err)
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

		userProfile := &blueprint.User{}
		newUser := a.DB.QueryRowx(queries.CreateUserQuery, user.Email, user.DisplayName, uniqueId, encryptedRefreshToken, user.ID)

		err = newUser.StructScan(userProfile)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan user during final auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		serialized, err := json.Marshal(map[string]string{
			"spotify": user.DisplayName,
		})
		platformIds, err := json.Marshal(map[string]string{
			"spotify": user.ID,
		})

		_, err = a.DB.Exec(queries.UpdatePlatformUsernamesAndIds, user.Email, string(serialized), string(platformIds))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to update platform usernames: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		_, err = a.DB.Exec(queries.UpdateUserPlatformToken, encryptedRefreshToken, "spotify", user.Email)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to update user spotify refresh platform token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  user platform usernames updated")

		// if the developer is Orchdio Labs, we want to redirect to the respective redirect urls for the apps and a fallback from env incase
		// TODO: implement checking for orchdio and getting the redirect urls for the apps and handle fallback in .env

		// decode the developer token
		appToken, err := util.DecodeAuthJwt(state)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: could not decode state token. perhaps it has expired: %v\n", err)
			// TODO: handle expired token
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		t := blueprint.OrchdioUserToken{
			RegisteredClaims: jwt.RegisteredClaims{},
			Email:            userProfile.Email,
			Username:         user.DisplayName,
			UUID:             userProfile.UUID,
			Platform:         "spotify",
		}

		authToken, err := util.SignJwt(&t)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign spotify auth jwt: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authedUserEmail = userProfile.Email
		// if developer is orchdio labs, create jwt token and redirect to the app url
		redirectURL = fmt.Sprintf("%s?token=%s", appToken.RedirectURL, string(authToken))

		// deezer auth flow
	case "deezer":
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  deezer auth flow")
		if code == "" {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: no code provided\n")
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "no code present. please pass a code")
		}

		if errorCode == "access_denied" {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: deezer returned an %v error\n", errorCode)
			return util.ErrorResponse(ctx, fiber.StatusUnauthorized, "unauthorized", "App access denied")
		}

		deezerSecret := os.Getenv("DEEZER_SECRET")
		deezerRedirectURL := os.Getenv("DEEZER_REDIRECT_URL")
		deezerClientID := os.Getenv("DEEZER_CLIENT_ID")

		deezerAuth := deezer.NewDeezerAuth(deezerClientID, deezerSecret, deezerRedirectURL)
		deezerToken := deezerAuth.FetchAccessToken(code)
		deezerUser, err := deezerAuth.CompleteUserAuth(deezerToken)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to complete deezer auth. could not fetch deezer complete auth token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		encryptedRefreshToken, err := util.Encrypt(deezerToken, []byte(encryptionSecretKey))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to encrypt refresh token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		userProfile := &blueprint.User{}
		deezerID := strconv.Itoa(deezerUser.ID)
		newUser := a.DB.QueryRowx(queries.CreateUserQuery, deezerUser.Email, deezerUser.Name, uniqueId, encryptedRefreshToken, deezerID)

		err = newUser.StructScan(userProfile)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan deezer user during final auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		serialed, err := json.Marshal(map[string]string{
			"deezer": deezerUser.Name,
		})
		_, err = a.DB.Exec(queries.UpdatePlatformUsernamesAndIds, deezerUser.Email, string(serialed))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to update deezer platform usernames: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		_, err = a.DB.Exec(queries.UpdateUserPlatformToken, encryptedRefreshToken, "deezer", deezerUser.Email)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to update deezer platform token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		appToken, err := util.DecodeAuthJwt(state)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: could not decode state token. perhaps it has expired: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		// generate jwt token for orchdio labs
		t := blueprint.OrchdioUserToken{
			RegisteredClaims: jwt.RegisteredClaims{},
			Email:            deezerUser.Email,
			Username:         userProfile.Username,
			UUID:             userProfile.UUID,
			Platform:         "deezer",
		}

		authToken, err := util.SignJwt(&t)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to sign deezer auth jwt: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}
		authedUserEmail = deezerUser.Email

		redirectURL = fmt.Sprintf("%s?token=%s", appToken.RedirectURL, string(authToken))

		// apple music auth flow
	case "applemusic":
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

		// decode the developer token
		appToken, err := util.DecodeAuthJwt(state)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: could not decode state token. perhaps it has expired: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
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
		newUser := a.DB.QueryRowx(queries.CreateUserQuery, hashedEmail, displayName, uniqueId, encryptedRefreshToken, hashedEmail)

		err = newUser.StructScan(userProfile)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to scan apple music user during final auth: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		serialized, err := json.Marshal(map[string]string{
			"applemusic": displayName,
		})
		platformIds, _ := json.Marshal(map[string]string{
			"applemusic": hashedEmail,
		})

		_, err = a.DB.Exec(queries.UpdatePlatformUsernamesAndIds, hashedEmail, string(serialized), string(platformIds))
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to update apple music platform usernames: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

		_, err = a.DB.Exec(queries.UpdateUserPlatformToken, encryptedRefreshToken, "applemusic", body.Email)
		if err != nil {
			log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to update apple music platform token: %v\n", err)
			return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
		}

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

		redirectURL = fmt.Sprintf("%v?token=%v", appToken.RedirectURL, authToken)
	// redirect to the redirect url from the state token.
	default:
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: invalid platform: %v\n", decodedState.Platform)
		return util.ErrorResponse(ctx, fiber.StatusNotImplemented, "unauthorized", "invalid platform")
	}
	// fetch the app that made the auth request in order to add to the task data, used for sending customized email
	// informing user that the app has been authorized.
	database := db.NewDB{DB: a.DB}
	app, err := database.FetchAppByAppId(decodedState.App)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to fetch app from database: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	log.Printf("[controllers][HandleAppAuthRedirect][debug] app authing user is -  app: %v\n", app.Name)
	taskID := uuid.New().String()
	taskApp := &blueprint.AppTaskData{
		Name: app.Name,
		UUID: app.UID.String(),
	}
	emailTaskData := &blueprint.EmailTaskData{
		App:        taskApp,
		From:       "alert@orchdio.com", // TODO: make this configurable
		To:         authedUserEmail,
		Payload:    map[string]interface{}{"APP_NAME": app.Name, "PLATFORM": strings.Title(decodedState.Platform)},
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
	err = emailQueue.EnqueueTask(emailTask, queue.EmailQueue, taskID)
	if err != nil {
		log.Printf("[controllers][HandleAppAuthRedirect] developer -  error: unable to enqueue email task: %v\n", err)
		return util.ErrorResponse(ctx, fiber.StatusInternalServerError, "internal error", "An internal error occurred")
	}

	// we dont really need to create a new task in the db since it is not a task that contains the data needed by a developer
	// set the task handler
	emailQueue.RunTask(fmt.Sprintf("send:appauth:email:%s", taskID), emailQueue.SendEmailHandler)
	log.Printf("[controllers][HandleAppAuthRedirect] app auth email scheduled.\n")
	return ctx.Redirect(redirectURL, fiber.StatusTemporaryRedirect)
}
