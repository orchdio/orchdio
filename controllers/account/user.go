package account

import (
	"context"
	"database/sql"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"zoove/blueprint"
	"zoove/db"
	"zoove/db/queries"
	"zoove/services/deezer"
	"zoove/services/spotify"
	"zoove/util"
)

type UserController struct {
	DB *sql.DB
}

func NewUserController(db *sql.DB) *UserController {
	return &UserController{
		DB: db,
	}
}

// RedirectAuth returns the authorization URL when a user wants to connect their platform
func (c *UserController) RedirectAuth(ctx *fiber.Ctx) error {
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
	return util.ErrorResponse(ctx, http.StatusNotImplemented, "Other Platforms have not been implemented")
}

// AuthSpotifyUser authorizes a user with spotify account
func (c *UserController) AuthSpotifyUser(ctx *fiber.Ctx) error {
	state := ctx.Query("state")
	encryptionSecretKey := os.Getenv("ENCRYPTION_SECRET")
	if state == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "State is not present")
	}

	// create a new net/http instance since *fasthttp.Request() cannot be passed
	r, err := http.NewRequest("GET", string(ctx.Request().RequestURI()), nil)

	if err != nil {
		log.Println("[controllers][account][user] Error - error creating a new http request - %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	client, refreshToken := spotify.CompleteUserAuth(context.Background(), r)
	encryptedToken, encErr := util.Encrypt(refreshToken, []byte(encryptionSecretKey))
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
	_, dbErr := c.DB.Exec(query,
		user.Email,
		user.ID,
		user.DisplayName,
		user.ExternalURLs["spotify"], // FIXME: handle this properly
		spotify.IDENTIFIER,
		encryptedToken,
	)

	if dbErr != nil {
		log.Printf("\n[controller][account][user] : [AuthUser] Error executing query: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	log.Printf("\n[user][controller][AuthUser] Method - User with the email %s just signed up or logged in with their Spotify account.\n", user.Email)
	// create a jwt
	claim := &blueprint.ZooveUserToken{
		Role:       "user",
		Email:      user.Email,
		Platform:   "spotify",
		PlatformID: user.ID,
	}
	token, err := util.SignJwt(claim)
	return util.SuccessResponse(ctx, http.StatusOK, string(token))
}

// AuthDeezerUser authorizes a user with deezer account
func (c *UserController) AuthDeezerUser(ctx *fiber.Ctx) error {
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

	encryptedToken, err := util.Encrypt(token, []byte(encryptionSecretKey))
	if err != nil {
		log.Printf("\n[controllers][account][users][AuthDeezerUser] Method - Error encrypting deezer token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	user, err := dz.CompleteUserAuth(token)
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error fetching deezer user: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	_, err = c.DB.Exec(queries.CreateUserQuery,
		user.Email,
		// it seems to work when i dont convert to string.
		// postgres still saves as string but I am not taking chances
		strconv.Itoa(user.ID),
		user.Name,
		user.Link,
		deezer.IDENTIFIER,
		encryptedToken,
	)

	// now create a token
	claims := &blueprint.ZooveUserToken{
		Role:       "user",
		Email:      user.Email,
		Platform:   "deezer",
		PlatformID: strconv.Itoa(user.ID),
	}
	jToken, err := util.SignJwt(claims)
	if err != nil {
		log.Printf("[user][controller][AuthDeezerUser] Method - Error signing token: %v", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	return util.SuccessResponse(ctx, http.StatusOK, string(jToken))
}

// FetchProfile fetches the playlist of the person, on the platform
func (c *UserController) FetchProfile(ctx *fiber.Ctx) error {
	claims := ctx.Locals("claims").(*blueprint.ZooveUserToken)

	database := db.NewDB{
		DB: c.DB,
	}

	user, err := database.FindUserByEmail(claims.Email)
	if err != nil {
		return util.ErrorResponse(ctx, http.StatusBadRequest, err)
	}
	return util.SuccessResponse(ctx, http.StatusOK, user)
}
