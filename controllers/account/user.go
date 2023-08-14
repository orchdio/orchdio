package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/queue"
	"orchdio/services"
	"orchdio/services/deezer"
	orchdioFollow "orchdio/services/follow"
	"orchdio/services/spotify"
	"orchdio/util"
	"os"
	"strings"
	"time"
)

type UserController struct {
	DB          *sqlx.DB
	Redis       *redis.Client
	AsynqClient *asynq.Client
	AsynqServer *asynq.ServeMux
}

func NewUserController(db *sqlx.DB, r *redis.Client, asynqClient *asynq.Client, asynqServer *asynq.ServeMux) *UserController {
	return &UserController{
		DB:          db,
		Redis:       r,
		AsynqClient: asynqClient,
		AsynqServer: asynqServer,
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

// FetchProfile fetches the user profile
func (u *UserController) FetchProfile(ctx *fiber.Ctx) error {
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)
	if claims.DeveloperID == "" {
		log.Printf("\n[user][controller][FetchUserProfile] warning - developer id not passed. Please pass a valid developer id")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Developer id not passed. Please pass a valid developer id")
	}
	log.Printf("\n[user][controller][FetchUserProfile] fetching user profile with id %s\n", claims.DeveloperID)
	// get the user via the email
	database := db.NewDB{DB: u.DB}
	user, err := database.FindUserByUUID(claims.DeveloperID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
		if errors.Is(err, sql.ErrNoRows) {
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

func (u *UserController) ResetPassword(ctx *fiber.Ctx) error {

	// GET: check if the token is valid
	// if the method is a GET, we want to check if the token is valid and return a 200 if not and 500 otherwise
	if ctx.Method() == http.MethodGet {
		log.Printf("[controller][user][ResetPassword] - checking if token is valid")
		// get the token from the redis store
		token := ctx.Query("token")
		if token == "" {
			log.Printf("[controller][user][ResetPassword] - token not passed. Please pass a valid token")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Token not passed")
		}

		// check if the token is valid
		val := u.Redis.Get(context.Background(), token).Val()
		if val == "" {
			log.Printf("[controller][user][ResetPassword] - token is invalid")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Token is invalid")
		}
		log.Printf("[controller][user][ResetPassword] - token is valid")
		return util.SuccessResponse(ctx, http.StatusOK, token)
	}

	// POST: reset the password
	log.Printf("[controller][user][ResetPassword] - resetting password")
	body := struct {
		Email string `json:"email"`
	}{}

	err := ctx.BodyParser(&body)
	if body.Email == "" {
		log.Printf("[controller][user][ResetPassword] - email not passed. Please pass a valid email")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email not passed")
	}

	// parse email
	_, err = mail.ParseAddress(body.Email)
	if err != nil {
		log.Printf("[controller][user][ResetPassword] - invalid email passed")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid email", "Please pass a valid email")
	}

	DB := db.NewDB{DB: u.DB}
	user, err := DB.FindUserByEmail(body.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[controller][user][ResetPassword] - user not found")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid data", "Login failed. The email may be invalid. Please make sure it is a valid email.")
		}
	}
	resetToken := util.GenerateResetToken(2)

	err = DB.SaveUserResetToken(user.UUID.String(), string(resetToken), time.Now().Add(15*time.Minute))
	if err != nil {
		log.Printf("[controller][user][ResetPassword] - error saving reset token: %v", err)
		return util.ErrorResponse(ctx, http.StatusUnprocessableEntity, err, "Could not set reset token")
	}
	//_, err = u.Redis.Set(context.Background(), redisKey, redisValue, time.Minute*15).Result()
	//if err != nil {
	//	log.Printf("[controller][user][ResetPassword] - error setting reset token in redis: %v", err)
	//	return util.ErrorResponse(ctx, http.StatusUnprocessableEntity, err, "Could not set reset token")
	//}

	taskID := uuid.New().String()
	_ = &blueprint.AppTaskData{
		Name: "reset-password",
		UUID: taskID,
	}

	// then send the email....
	orchdioQueue := queue.NewOrchdioQueue(u.AsynqClient, u.DB, u.Redis, u.AsynqServer)
	taskData := &blueprint.EmailTaskData{
		From: os.Getenv("ALERT_EMAIL"),
		To:   body.Email,
		Payload: map[string]interface{}{
			"RESETLINK": fmt.Sprintf("%s/reset-password?token=%s", os.Getenv("FRONTEND_URL"), resetToken),
		},
		TaskID:     taskID,
		TemplateID: 4,
		Subject:    "Password Reset",
	}

	log.Printf("task data:")
	spew.Dump(taskData)

	serializedEmailData, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("[controller][user][ResetPassword] - error serializing email data: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not serialize email data")
	}

	sendMail, err := orchdioQueue.NewTask(fmt.Sprintf("send:reset_password_email:%s", taskID), queue.EmailTask, 2, serializedEmailData)
	if err != nil {
		log.Printf("[controller][user][ResetPassword] - error creating send email task: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create send email task")
	}

	err = orchdioQueue.EnqueueTask(sendMail, queue.EmailQueue, taskID, time.Second*2)
	if err != nil {
		log.Printf("[controller][user][ResetPassword] - error enqueuing send email task: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not enqueue send email task")
	}

	return util.SuccessResponse(ctx, http.StatusOK, "Reset token sent successfully")
}

func (u *UserController) ChangePassword(ctx *fiber.Ctx) error {
	log.Printf("[controller][user][ChangePassword] - changing password")
	body := struct {
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
		ResetToken      string `json:"reset_token"`
	}{}

	err := ctx.BodyParser(&body)
	if body.ResetToken == "" {
		log.Printf("[controller][user][ChangePassword] - email not passed. Please pass a valid email")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email not passed")
	}
	if body.Password == "" {
		log.Printf("[controller][user][ChangePassword] - password not passed. Please pass a valid password")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Password not passed")
	}

	if body.ConfirmPassword == "" {
		log.Printf("[controller][user][ChangePassword] - confirm password not passed. Please pass a valid confirm password")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Confirm Password not passed")
	}

	if body.Password != body.ConfirmPassword {
		log.Printf("[controller][user][ChangePassword] - password and confirm password do not match")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Password and Confirm Password do not match")
	}

	DB := db.NewDB{DB: u.DB}

	// get key from redis.
	user, err := DB.FindUserByResetToken(body.ResetToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[controller][user][ChangePassword] - user not found")
			return util.ErrorResponse(ctx, http.StatusBadRequest, "invalid data", "Token not found or has expired. Please retry the password reset or check credentials passed.")
		}
		log.Printf("[controller][user][ChangePassword] - error finding user: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not find user")
	}

	// hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[controller][user][ChangePassword] - error hashing password: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not hash password")
	}

	// update password
	err = DB.UpdateUserPassword(string(hashedPassword), user.UUID.String())
	if err != nil {
		log.Printf("[controller][user][ChangePassword] - error updating password: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not update password")
	}

	userOrg, dErr := DB.FetchOrgs(user.UUID.String())
	if dErr != nil {
		log.Printf("[controller][user][ChangePassword] - error fetching user org: %v", dErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "Could not fetch user org")
	}

	// create app jwt token
	token, err := util.SignOrgLoginJWT(&blueprint.AppJWT{
		OrgID:       userOrg.UID.String(),
		DeveloperID: user.UUID.String(),
	})

	apps, er := DB.FetchApps(userOrg.UID.String(), user.UUID.String())
	if er != nil {
		log.Printf("[controller][user][ChangePassword] - error fetching user apps: %v", er)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, er, "Could not fetch user apps")
	}

	// todo: perhaps move this to a util/separate func. also found in org.go
	result := map[string]interface{}{
		"org_id":      userOrg.UID.String(),
		"name":        userOrg.Name,
		"description": userOrg.Description,
		"token":       token,
		"apps":        apps,
	}
	return util.SuccessResponse(ctx, http.StatusOK, result)
}
