package account

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"net/mail"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/queue"
	"orchdio/util"
	"os"
	"time"
)

// CreateOrg creates a new org
func (u *UserController) CreateOrg(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][CreateOrg] - creating org")
	// todo: implement email verification for org owner creation when they pass their email

	var body blueprint.CreateOrganizationData
	err := ctx.BodyParser(&body)
	if err != nil {
		log.Printf("[controller][account][CreateOrg] - error parsing body: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not create organization. Invalid body passed")
	}

	if body.Name == "" {
		log.Printf("[controller][account][CreateOrg] - name is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "name is empty", "Could not create organization. Name is empty")
	}

	if body.OwnerEmail == "" {
		log.Printf("[controller][account][CreateOrg] - owner email is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "owner email is empty", "Could not create organization. Owner email is empty")
	}

	if body.OwnerPassword == "" {
		log.Printf("[controller][account][CreateOrg] - owner password is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "owner password is empty", "Could not create organization. Owner password is empty")
	}

	validEmail, err := mail.ParseAddress(body.OwnerEmail)
	if err != nil {
		log.Printf("[controller][account][CreateOrg] - invalid email: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not create organization. Invalid email")
	}
	if validEmail.Address != body.OwnerEmail {
		log.Printf("[controller][account][CreateOrg] - invalid email: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not create organization. Invalid email")
	}

	database := db.NewDB{DB: u.DB}
	uniqueId := uuid.NewString()
	var userId string

	hashedPass, bErr := bcrypt.GenerateFromPassword([]byte(body.OwnerPassword), bcrypt.DefaultCost)
	if bErr != nil {
		log.Printf("[controller][account][CreateOrg] - error hashing password: %v", bErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, bErr, "Could not create organization")
	}

	// if the user has been created, then we need the user id to create the org. if not, we need to create the user first
	userInfo, err := database.FindUserByEmail(body.OwnerEmail)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// create user
			newUserId := uuid.NewString()
			_, dErr := u.DB.Exec(queries.CreateNewOrgUser, body.OwnerEmail, newUserId, string(hashedPass))
			if dErr != nil {
				log.Printf("[controller][account][CreateOrg] - error creating user: %v", dErr)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "Could not create organization")
			}
			log.Printf("[controller][account][CreateOrg] - user created new user with email %s and id: %s", body.OwnerEmail, newUserId)
			userId = newUserId
		}
	}

	if userInfo != nil {
		// get the organizations users have.
		// An email can be connected to a streaming platform account and then used to connect to an app on orchdio,
		// so we need to check if the user already has an organization. This is because if a user
		// is created by signing up by creating an organization, they must have an organization already. So if they dont have
		// an organization we can assume that they were created by signing up with a streaming platform account.
		// and if they do, we return an error saying that they already have an organization.
		orgs, fetchErr := database.FetchOrg(userInfo.UUID.String())
		if fetchErr != nil && !errors.Is(fetchErr, sql.ErrNoRows) {
			log.Printf("[controller][account][CreateOrg] - error getting orgs: %v", fetchErr)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, fetchErr, "Could not create organization.")
		}

		if orgs != nil {
			log.Printf("[controller][account][CreateOrg] - user already has an organization")
			return util.ErrorResponse(ctx, http.StatusConflict, err, "Could not create organization. User already has an organization")
		}
		// in the case where the user was created from authing with a platform for example, there will be no existing password for the user
		// so in this case we need to update the user with the password
		userId = userInfo.UUID.String()
		_, dErr := u.DB.Exec(queries.UpdateUserPassword, string(hashedPass), userId)
		if dErr != nil {
			log.Printf("[controller][account][CreateOrg] - error updating user password: %v", dErr)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "Could not create organization")
		}
	}

	// TODO: allow users to have multiple orgs. for now we allow only 1.
	org, err := database.FetchOrg(userId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[controller][account][CreateOrg] - no orgs found for user: %s. Going to create user", userId)

			// todo: send verification email to user
			uid, cErr := database.CreateOrg(uniqueId, body.Name, body.Description, userId)
			if cErr != nil {
				log.Printf("[controller][account][CreateOrg] - error creating org: %v", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create organization")
			}

			appToken, sErr := util.SignOrgLoginJWT(&blueprint.AppJWT{
				OrgID:       string(uid),
				DeveloperID: userId,
			})

			if sErr != nil {
				log.Printf("[controller][account][CreateOrg] - error signing app token: %v", sErr)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, sErr, "Could not create organization")
			}

			res := map[string]string{
				"org_id":      string(uid),
				"name":        body.Name,
				"description": body.Description,
				"token":       string(appToken),
			}

			mailErr := u.SendAdminWelcomeEmail(body.OwnerEmail)
			if mailErr != nil {
				log.Printf("[controller][account][LoginUserToOrg] - error sending welcome email: %v", mailErr)
			}

			log.Printf("[controller][account][CreateOrg] - org '%s' created", body.Name)
			return util.SuccessResponse(ctx, http.StatusCreated, res)
		}
	}

	appToken, err := util.SignOrgLoginJWT(&blueprint.AppJWT{
		OrgID:       org.UID.String(),
		DeveloperID: userId,
	})

	res := &blueprint.OrchdioOrgCreateResponse{
		OrgID:       org.UID.String(),
		Name:        org.Name,
		Description: org.Description,
		Token:       string(appToken),
	}

	mailErr := u.SendAdminWelcomeEmail(body.OwnerEmail)
	if mailErr != nil {
		log.Printf("[controller][account][LoginUserToOrg] - error sending welcome email: %v", mailErr)
	}

	log.Printf("Should have sent email to %s", body.OwnerEmail)
	return util.SuccessResponse(ctx, http.StatusOK, res)
}

// DeleteOrg deletes  an org belonging to the user.
func (u *UserController) DeleteOrg(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][DeleteOrg] - deleting org")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)

	orgId := ctx.Params("orgId")

	if orgId == "" {
		log.Printf("[controller][account][DeleteOrg] - error: Org ID is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is empty. Please pass a valid Org ID")
	}

	if !util.IsValidUUID(orgId) {
		log.Printf("[controller][account][DeleteOrg] - error: Org ID is invalid")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is invalid. Please pass a valid Org ID")
	}

	database := db.NewDB{DB: u.DB}
	err := database.DeleteOrg(orgId, claims.DeveloperID)
	if err != nil {
		log.Printf("[controller][account][DeleteOrg] - error deleting org: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not delete organization")
	}

	log.Printf("[controller][account][DeleteOrg] - org deleted with unique id: %s", orgId)
	return util.SuccessResponse(ctx, http.StatusOK, "success")
}

// UpdateOrg updates an org belonging to the user.
func (u *UserController) UpdateOrg(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][UpdateOrg] - updating org")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)

	orgId := ctx.Params("orgId")
	var updateData blueprint.UpdateOrganizationData
	err := ctx.BodyParser(&updateData)
	if err != nil {
		log.Printf("[controller][account][UpdateOrg] - error parsing body: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not update organization. Invalid body passed")
	}

	if orgId == "" {
		log.Printf("[controller][account][UpdateOrg] - error: Org ID is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is empty. Please pass a valid Org ID")
	}

	if !util.IsValidUUID(orgId) {
		log.Printf("[controller][account][UpdateOrg] - error: Org ID is invalid")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is invalid. Please pass a valid Org ID")
	}

	database := db.NewDB{DB: u.DB}
	err = database.UpdateOrg(orgId, claims.DeveloperID, &updateData)
	if err != nil {
		log.Printf("[controller][account][UpdateOrg] - error updating org: %v", err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, http.StatusNotFound, "NOT_FOUND", "Could not update organization. Organization not found. Please make sure this Organization and it belongs to you.")
		}
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not update organization")
	}

	return util.SuccessResponse(ctx, http.StatusOK, "success")
}

// FetchUserOrgs returns all orgs belonging to the user.
func (u *UserController) FetchUserOrgs(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][GetOrgs] - getting orgs")
	claims := ctx.Locals("app_jwt").(*blueprint.AppJWT)

	database := db.NewDB{DB: u.DB}
	orgs, err := database.FetchOrg(claims.DeveloperID)
	if err != nil {
		log.Printf("[controller][account][GetOrgs] - error getting orgs: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not get organizations")
	}

	return util.SuccessResponse(ctx, http.StatusOK, orgs)
}

func (u *UserController) LoginUserToOrg(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][LoginUserToOrg] - logging user into org")
	var body blueprint.LoginToOrgData
	err := ctx.BodyParser(&body)
	if err != nil {
		log.Printf("[controller][account][LoginUserToOrg] - error parsing body: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, err, "Could not login to organization. Invalid body passed")
	}

	if body.Email == "" {
		log.Printf("[controller][account][LoginUserToOrg] - error: email is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email is empty. Please pass a valid email")
	}
	isValidEmail, err := mail.ParseAddress(body.Email)
	if err != nil {
		log.Printf("[controller][account][LoginUserToOrg] - error: email is invalid")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email is invalid. Please pass a valid email")
	}
	if isValidEmail.Address != body.Email {
		log.Printf("[controller][account][LoginUserToOrg] - error: email is invalid")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Email is invalid. Please pass a valid email")
	}

	if body.Password == "" {
		log.Printf("[controller][account][LoginUserToOrg] - error: password is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Password is empty. Please pass a valid password")
	}
	database := db.NewDB{DB: u.DB}
	scanRes := u.DB.QueryRowx(queries.FetchUserEmailAndPassword, body.Email)
	var user blueprint.User
	sErr := scanRes.StructScan(&user)
	if sErr != nil {
		if errors.Is(sErr, sql.ErrNoRows) {
			log.Printf("[controller][account][LoginUserToOrg] - error: could not find user with email %s during login attempt", body.Email)
			return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid login", "Could not login to organization. Password or email is incorrect.")
		}
		log.Printf("[controller][account][LoginUserToOrg] - error: %v", sErr)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, sErr, "Could not login to organization")
	}

	ct := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))
	if ct != nil {
		log.Printf("[controller][account][LoginUserToOrg] - error: %v", err)
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid login", "Could not login to organization. Password or email is incorrect.")
	}

	org, err := database.FetchOrg(user.UUID.String())
	if err != nil {
		log.Printf("[controller][account][LoginUserToOrg] - error getting orgs: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not get organizations")
	}

	// encrypt the result as a JWT
	token, err := util.SignOrgLoginJWT(&blueprint.AppJWT{
		OrgID:       org.UID.String(),
		DeveloperID: user.UUID.String(),
	})

	apps, err := database.FetchApps(user.UUID.String(), org.UID.String())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("[controller][account][LoginUserToOrg] - error getting apps: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not get apps")
	}

	result := &blueprint.OrchdioLoginUserResponse{
		OrgID:       org.UID.String(),
		Name:        org.Name,
		Description: org.Description,
		Token:       string(token),
		Apps:        apps,
	}

	// return a single org for now.
	return util.SuccessResponse(ctx, http.StatusOK, result)
}

func (u *UserController) SendAdminWelcomeEmail(email string) error {
	// prepare welcome email
	taskID := uuid.NewString()
	orchdioQueue := queue.NewOrchdioQueue(u.AsynqClient, u.DB, u.Redis, u.AsynqServer)
	taskData := &blueprint.EmailTaskData{
		From:    os.Getenv("ALERT_EMAIL"),
		To:      email,
		Payload: nil,
		// todo: move this to a configuration, to make it easier to override
		Subject:    "Welcome to Orchdio",
		TaskID:     taskID,
		TemplateID: 3,
	}

	serializedEmailData, sErr := json.Marshal(taskData)
	if sErr != nil {
		log.Printf("[controller][account][CreateOrg] - error serializing email data: %v", sErr)
		return sErr
	}

	sendMail, zErr := orchdioQueue.NewTask(fmt.Sprintf("%s:%s", blueprint.SendWelcomeEmailTaskPattern, taskID), blueprint.EmailQueueName, 2, serializedEmailData)
	if zErr != nil {
		log.Printf("[controller][account][CreateOrg] - error creating task: %v", zErr)
		return zErr
	}

	err := orchdioQueue.EnqueueTask(sendMail, blueprint.EmailQueueName, taskID, time.Second*2)
	if err != nil {
		log.Printf("[controller][account][CreateOrg] - error enqueuing task: %v", err)
	}
	return err
}
