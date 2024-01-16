package account

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/mail"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	logger2 "orchdio/logger"
	"orchdio/queue"
	"orchdio/util"
	"os"
	"time"
)

// CreateOrg creates a new org
func (u *UserController) CreateOrg(ctx *fiber.Ctx) error {
	loggerOpts := &blueprint.OrchdioLoggerOptions{
		RequestID:            ctx.Get("x-orchdio-request-id"),
		ApplicationPublicKey: zap.String("app_pub_key", ctx.Get("x-orchdio-app-pub-key")).String,
	}

	orchdioLogger := logger2.NewZapSentryLogger(loggerOpts)
	// todo: implement email verification for org owner creation when they pass their email
	//claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)
	u.Logger = orchdioLogger

	var body blueprint.CreateOrganizationData
	err := ctx.BodyParser(&body)
	if err != nil {
		u.Logger.Error("[controller][account][CreateOrg] - error parsing body", zap.Error(err))
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not create organization. Invalid body passed")
	}

	if body.Name == "" {
		u.Logger.Warn("[controller][account][CreateOrg] - name is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "name is empty", "Could not create organization. Name is empty")
	}

	if body.OwnerEmail == "" {
		u.Logger.Warn("[controller][account][CreateOrg] - owner email is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "owner email is empty", "Could not create organization. Owner email is empty")
	}

	if body.OwnerPassword == "" {
		u.Logger.Warn("[controller][account][CreateOrg] - owner password is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "owner password is empty", "Could not create organization. Owner password is empty")
	}

	validEmail, err := mail.ParseAddress(body.OwnerEmail)
	if err != nil {
		u.Logger.Warn("[controller][account][CreateOrg] - invalid email", zap.Error(err))
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not create organization. Invalid email")
	}
	if validEmail.Address != body.OwnerEmail {
		u.Logger.Warn("[controller][account][CreateOrg] - invalid email", zap.Error(err))
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not create organization. Invalid email")
	}

	//database := db.NewDB{DB: u.DB}
	database := db.New(u.DB, u.Logger)
	uniqueId := uuid.NewString()
	var userId string

	hashedPass, bErr := bcrypt.GenerateFromPassword([]byte(body.OwnerPassword), bcrypt.DefaultCost)
	if bErr != nil {
		u.Logger.Error("[controller][account][CreateOrg] - error hashing password", zap.Error(bErr))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, bErr, "Could not create organization")
	}

	// if the user has been created, then we need the user id to create the org. if not, we need to create the user first
	userInf, err := database.FindUserByEmail(body.OwnerEmail)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// create user
			ubx := uuid.NewString()
			_, dErr := u.DB.Exec(queries.CreateNewOrgUser, body.OwnerEmail, ubx, string(hashedPass))
			if dErr != nil {
				u.Logger.Error("[controller][account][CreateOrg] - error creating user", zap.Error(dErr))

				spew.Dump(dErr)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "Could not create organization")
			}
			u.Logger.Info("[controller][account][CreateOrg] - user created new user", zap.String("email", body.OwnerEmail), zap.String("id", ubx))
			userId = ubx
		}

		u.Logger.Info("Unrecognized error")
		spew.Dump(err)
	}

	if userInf != nil {
		// get the organizations users have.
		// An email can be connected to a streaming platform account and then used to connect to an app on orchdio,
		// so we need to check if the user already has an organization. This is because if a user
		// is created by signing up by creating an organization, they must have an organization already. So if they dont have
		// an organization we can assume that they were created by signing up with a streaming platform account.
		// and if they do, we return an error saying that they already have an organization.
		orgs, fErr := database.FetchOrgs(userInf.UUID.String())
		if fErr != nil && !errors.Is(fErr, sql.ErrNoRows) {
			u.Logger.Error("[controller][account][CreateOrg] - error getting orgs", zap.Error(fErr))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create organization.")
		}

		if orgs != nil {
			u.Logger.Warn("[controller][account][CreateOrg] - user already has an organization", zap.String("user_id", userInf.UUID.String()))
			return util.ErrorResponse(ctx, http.StatusConflict, err, "Could not create organization. User already has an organization")
		}
		// in the case where the user was created from authing with a platform for example, there will be no existing password for the user
		// so in this case we need to update the user with the password
		userId = userInf.UUID.String()
		_, dErr := u.DB.Exec(queries.UpdateUserPassword, string(hashedPass), userId)
		if dErr != nil {
			u.Logger.Error("[controller][account][CreateOrg] - error updating user password", zap.Error(dErr))
			return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "Could not create organization")
		}
	}

	// fetch all the orgs.
	// TODO: allow users to have multiple orgs. for now we allow only 1.
	org, err := database.FetchOrgs(userId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			u.Logger.Info("[controller][account][CreateOrg] - no orgs found for user. Going to create user", zap.String("user_id", userId))

			// todo: send email verification to user
			uid, cErr := database.CreateOrg(uniqueId, body.Name, body.Description, userId)
			if cErr != nil {
				u.Logger.Error("[controller][account][CreateOrg] - error creating org", zap.Error(cErr))
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create organization")
			}

			appToken, sErr := util.SignOrgLoginJWT(&blueprint.AppJWT{
				OrgID:       string(uid),
				DeveloperID: userId,
			})

			if sErr != nil {
				u.Logger.Error("[controller][account][CreateOrg] - error signing app token", zap.Error(sErr))
				return util.ErrorResponse(ctx, http.StatusInternalServerError, sErr, "Could not create organization")
			}

			res := map[string]string{
				"org_id":      string(uid),
				"name":        body.Name,
				"description": body.Description,
				"token":       string(appToken),
			}

			taskId, mailErr := u.SendAdminWelcomeEmail(body.OwnerEmail)
			if mailErr != nil {
				u.Logger.Warn("[controller][account][CreateOrg] - error sending welcome email", zap.Error(mailErr),
					zap.String("email_task_id", taskId))
			}
			u.Logger.Info("[controller][account][CreateOrg] - org created", zap.String("org_id", string(uid)), zap.String("org_name", body.Name))
			return util.SuccessResponse(ctx, http.StatusCreated, res)
		}
	}

	appToken, err := util.SignOrgLoginJWT(&blueprint.AppJWT{
		OrgID:       org.UID.String(),
		DeveloperID: userId,
	})

	res := map[string]string{
		"org_id":      org.UID.String(),
		"name":        org.Name,
		"description": org.Description,
		"token":       string(appToken),
	}

	taskId, mailErr := u.SendAdminWelcomeEmail(body.OwnerEmail)
	if mailErr != nil {
		u.Logger.Warn("[controller][account][CreateOrg] - error sending welcome email", zap.Error(mailErr),
			zap.String("email_task_id", taskId))
	}

	u.Logger.Info("[controller][account][CreateOrg] - user email sent. Organization created", zap.String("org_id", org.UID.String()))
	return util.SuccessResponse(ctx, http.StatusOK, res)
}

// DeleteOrg deletes  an org belonging to the user.
func (u *UserController) DeleteOrg(ctx *fiber.Ctx) error {
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)
	orgId := ctx.Params("orgId")

	if orgId == "" {
		u.Logger.Warn("[controller][account][DeleteOrg] - error: Org ID is empty",
			zap.String("org_id", orgId))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is empty. Please pass a valid Org ID")
	}

	if !util.IsValidUUID(orgId) {
		u.Logger.Warn("[controller][account][DeleteOrg] - error: Org ID is invalid",
			zap.String("org_id", orgId))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is invalid. Please pass a valid Org ID")
	}

	// check if the user is the owner of the org
	// if not, return error

	database := db.NewDB{DB: u.DB}
	err := database.DeleteOrg(orgId, claims.DeveloperID)
	if err != nil {
		u.Logger.Error("[controller][account][DeleteOrg] - error deleting org", zap.Error(err),
			zap.String("org_id", orgId))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not delete organization")
	}

	u.Logger.Info("[controller][account][DeleteOrg] - org deleted", zap.String("org_id", orgId))
	return util.SuccessResponse(ctx, http.StatusOK, "success")
}

// UpdateOrg updates an org belonging to the user.
func (u *UserController) UpdateOrg(ctx *fiber.Ctx) error {
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)

	orgId := ctx.Params("orgId")
	var updateData blueprint.UpdateOrganizationData
	err := ctx.BodyParser(&updateData)
	if err != nil {
		u.Logger.Error("[controller][account][UpdateOrg] - error parsing body", zap.Error(err),
			zap.String("org_id", orgId))
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not update organization. Invalid body passed")
	}

	if orgId == "" {
		u.Logger.Warn("[controller][account][UpdateOrg] - error: Org ID is empty",
			zap.String("org_id", orgId))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is empty. Please pass a valid Org ID")
	}

	if !util.IsValidUUID(orgId) {
		u.Logger.Warn("[controller][account][UpdateOrg] - error: Org ID is invalid",
			zap.String("org_id", orgId))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is invalid. Please pass a valid Org ID")
	}

	database := db.NewDB{DB: u.DB}
	err = database.UpdateOrg(orgId, claims.DeveloperID, &updateData)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, http.StatusNotFound, "NOT_FOUND", "Could not update organization. Organization not found. Please make sure this Organization and it belongs to you.")
		}
		u.Logger.Error("[controller][account][UpdateOrg] - error updating org", zap.Error(err),
			zap.String("org_id", orgId))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not update organization")
	}

	return util.SuccessResponse(ctx, http.StatusOK, "success")
}

// FetchUserOrgs returns all orgs belonging to the user.
func (u *UserController) FetchUserOrgs(ctx *fiber.Ctx) error {
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)

	database := db.NewDB{DB: u.DB}
	orgs, err := database.FetchOrgs(claims.DeveloperID)
	if err != nil {
		u.Logger.Error("[controller][account][FetchUserOrgs] - error getting orgs", zap.Error(err), zap.String("user_id", claims.DeveloperID))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not get organizations")
	}

	return util.SuccessResponse(ctx, http.StatusOK, orgs)
}

func (u *UserController) LoginUserToOrg(ctx *fiber.Ctx) error {
	var body blueprint.LoginToOrgData
	err := ctx.BodyParser(&body)
	if err != nil {
		u.Logger.Error("[controller][account][LoginUserToOrg] - error parsing body", zap.Error(err))
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not login to organization. Invalid body passed")
	}

	if body.Email == "" {
		u.Logger.Warn("[controller][account][LoginUserToOrg] - error: email is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email is empty. Please pass a valid email")
	}
	isValidEmail, err := mail.ParseAddress(body.Email)
	if err != nil {
		u.Logger.Warn("[controller][account][LoginUserToOrg] - error: email is invalid", zap.String("email", body.Email))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email is invalid. Please pass a valid email")
	}
	if isValidEmail.Address != body.Email {
		u.Logger.Warn("[controller][account][LoginUserToOrg] - error: email is valid but not verified email is not the same as passed email. Something might be suspicious", zap.String("email", body.Email))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email is invalid. Please pass a valid email")
	}

	if body.Password == "" {
		u.Logger.Warn("[controller][account][LoginUserToOrg] - error: password is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Password is empty. Please pass a valid password")
	}
	// todo: implement db method to login user login for user, returning the org id, name and description
	database := db.NewDB{DB: u.DB}
	scanRes := u.DB.QueryRowx(queries.FetchUserEmailAndPassword, body.Email)
	var user blueprint.User
	sErr := scanRes.StructScan(&user)
	if sErr != nil {
		if errors.Is(sErr, sql.ErrNoRows) {
			u.Logger.Warn("[controller][account][LoginUserToOrg] - error: User with the email does not exist.", zap.String("email", body.Email))
			return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid login", "Could not login to organization. Password or email is incorrect.")
		}
		u.Logger.Warn("[controller][account][LoginUserToOrg] - error: could not find user with email during login attempt", zap.String("email", body.Email))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, sErr, "Could not login to organization")
	}

	ct := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))
	if ct != nil {
		u.Logger.Warn("[controller][account][LoginUserToOrg] - error: password is incorrect", zap.String("email", body.Email))
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid login", "Could not login to organization. Password or email is incorrect.")
	}

	org, err := database.FetchOrgs(user.UUID.String())
	if err != nil {
		u.Logger.Error("[controller][account][LoginUserToOrg] - error getting orgs", zap.Error(err), zap.String("user_id", user.UUID.String()))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not get organizations")
	}

	// encrypt the result as a JWT
	token, err := util.SignOrgLoginJWT(&blueprint.AppJWT{
		OrgID:       org.UID.String(),
		DeveloperID: user.UUID.String(),
	})

	apps, err := database.FetchApps(user.UUID.String(), org.UID.String())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		u.Logger.Error("[controller][account][LoginUserToOrg] - error getting apps", zap.Error(err), zap.String("user_id", user.UUID.String()))
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not get apps")
	}

	result := map[string]interface{}{
		"org_id":      org.UID.String(),
		"name":        org.Name,
		"description": org.Description,
		"token":       string(token),
		"apps":        apps,
	}

	// return a single org for now.
	return util.SuccessResponse(ctx, http.StatusOK, result)
}

func (u *UserController) SendAdminWelcomeEmail(email string) (string, error) {
	// prepare welcome email
	taskID := uuid.NewString()
	orchdioQueue := queue.NewOrchdioQueue(u.AsynqClient, u.DB, u.Redis, u.AsynqServer)
	taskData := &blueprint.EmailTaskData{
		From:       os.Getenv("ALERT_EMAIL"),
		To:         email,
		Payload:    nil,
		Subject:    "Welcome to Orchdio",
		TaskID:     taskID,
		TemplateID: 3,
	}

	serializedEmailData, sErr := json.Marshal(taskData)
	if sErr != nil {
		u.Logger.Error("[controller][account][CreateOrg] - error serializing email data", zap.Error(sErr))
		return taskID, sErr
	}

	sendMail, zErr := orchdioQueue.NewTask(fmt.Sprintf("send:welcome_email:%s", taskID), queue.EmailTask, 2, serializedEmailData)
	if zErr != nil {
		u.Logger.Error("[controller][account][CreateOrg] - error creating welcome email task", zap.Error(zErr))
		return taskID, zErr
	}

	err := orchdioQueue.EnqueueTask(sendMail, queue.EmailQueue, taskID, time.Second*2)
	if err != nil {
		u.Logger.Warn("======================================================\nError enqueuing task\n======================================================", zap.Error(err))
		return taskID, err
	}
	return taskID, nil
}
