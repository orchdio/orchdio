package account

import (
	"database/sql"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"net/mail"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/util"
)

// CreateOrg creates a new org
func (u *UserController) CreateOrg(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][CreateOrg] - creating org")
	// todo: implement email verification for org owner creation when they pass their email
	//claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

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
	userInf, err := database.FindUserByEmail(body.OwnerEmail)
	if err != nil {
		if err == sql.ErrNoRows {
			// create user
			ubx := uuid.NewString()
			_, dErr := u.DB.Exec(queries.CreateNewOrgUser, body.OwnerEmail, ubx, string(hashedPass))
			if dErr != nil {
				log.Printf("[controller][account][CreateOrg] - error creating user: %v", dErr)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "Could not create organization")
			}
			log.Printf("[controller][account][CreateOrg] - user created new user with email %s and id: %s", body.OwnerEmail, ubx)
			userId = ubx
		}
	}

	if userInf != nil {
		// in the case where the user was created from authing with a platform for example, there will be no password
		// so in this case we need to update the user with the password
		userId = userInf.UUID.String()
		_, dErr := u.DB.Exec(queries.UpdateUserPassword, string(hashedPass), userId)
		if dErr != nil {
			log.Printf("[controller][account][CreateOrg] - error updating user password: %v", dErr)
			return util.ErrorResponse(ctx, http.StatusInternalServerError, dErr, "Could not create organization")
		}
	}

	// todo: send email verification to user
	uid, err := database.CreateOrg(uniqueId, body.Name, body.Description, userId)
	if err != nil {
		log.Printf("[controller][account][CreateOrg] - error creating org: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not create organization")
	}

	res := map[string]string{
		"org_id":      string(uid),
		"name":        body.Name,
		"description": body.Description,
	}

	log.Printf("[controller][account][CreateOrg] - org created with unique id: %s %s", body.Name, uid)
	return util.SuccessResponse(ctx, http.StatusCreated, res)
}

// DeleteOrg deletes  an org belonging to the user.
func (u *UserController) DeleteOrg(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][DeleteOrg] - deleting org")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

	orgId := ctx.Params("orgId")

	if orgId == "" {
		log.Printf("[controller][account][DeleteOrg] - error: Org ID is empty")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is empty. Please pass a valid Org ID")
	}

	if !util.IsValidUUID(orgId) {
		log.Printf("[controller][account][DeleteOrg] - error: Org ID is invalid")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "bad request", "Org ID is invalid. Please pass a valid Org ID")
	}

	// check if the user is the owner of the org
	// if not, return error

	database := db.NewDB{DB: u.DB}
	err := database.DeleteOrg(orgId, claims.UUID.String())
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
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

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
	err = database.UpdateOrg(orgId, claims.UUID.String(), &updateData)
	if err != nil {
		log.Printf("[controller][account][UpdateOrg] - error updating org: %v", err)
		if err == sql.ErrNoRows {
			return util.ErrorResponse(ctx, http.StatusNotFound, "NOT_FOUND", "Could not update organization. Organization not found. Please make sure this Organization and it belongs to you.")
		}
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not update organization")
	}

	return util.SuccessResponse(ctx, http.StatusOK, "success")
}

// FetchUserOrgs returns all orgs belonging to the user.
func (u *UserController) FetchUserOrgs(ctx *fiber.Ctx) error {
	log.Printf("[controller][account][GetOrgs] - getting orgs")
	claims := ctx.Locals("claims").(*blueprint.OrchdioUserToken)

	database := db.NewDB{DB: u.DB}
	orgs, err := database.FetchOrgs(claims.UUID.String())
	if err != nil {
		log.Printf("[controller][account][GetOrgs] - error getting orgs: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err, "Could not get organizations")
	}

	return util.SuccessResponse(ctx, http.StatusOK, orgs)
}
