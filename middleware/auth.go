package middleware

import (
	"database/sql"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"orchdio/blueprint"
	"orchdio/db"
	"orchdio/db/queries"
	"orchdio/services/spotify"
	"orchdio/util"
	"strings"
	"time"
)

type AuthMiddleware struct {
	DB *sqlx.DB
}

func NewAuthMiddleware(db *sqlx.DB) *AuthMiddleware {
	return &AuthMiddleware{DB: db}
}

// ValidateKey validates that the key is valid
func (a *AuthMiddleware) ValidateKey(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")

	if len([]byte(apiKey)) > 36 {
		log.Printf("[middleware][ValidateKey] key is too long. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Key too long")
	}

	isValid := util.IsValidUUID(apiKey)

	if !isValid {
		log.Printf("[controller][user][Revoke] invalid key. Bad request %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid apikey")
	}

	// fetch the user from the database
	database := db.NewDB{DB: a.DB}

	user, err := database.FetchUserWithApiKey(apiKey)
	if err != nil {

		if err == sql.ErrNoRows {
			log.Printf("[middleware][ValidateKey] key not found. %s\n", apiKey)
			return util.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid apikey")
		}

		log.Printf("[middleware][ValidateKey] error - Could not fetch user with api key: %v\n", err)
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}
	ctx.Locals("user", user)
	log.Printf("[middleware][ValidateKey] API key is valid")
	return ctx.Next()
}

func (a *AuthMiddleware) LogIncomingRequest(ctx *fiber.Ctx) error {
	log.Printf("[middleware][LogIncomingRequest] incoming request: %s  %s: %s\n", ctx.IP(), ctx.Method(), ctx.Path())
	return ctx.Next()
}

func (a *AuthMiddleware) AddDeveloperToContext(ctx *fiber.Ctx) error {
	log.Printf("[db][middleware][FetchAppDeveloperWithSecretKey] developer -  fetching app developer with secret key\n")
	key := ctx.Get("x-orchdio-key")
	if key == "" {
		log.Printf("[db][FetchAppDeveloperWithSecretKey] developer -  error: could not fetch app developer with secret")
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "missing x-orchdio-key header")
	}

	//encryptedKey, err := util.Encrypt([]byte(key), []byte(os.Getenv("ENCRYPTION_SECRET")))
	//if err != nil {
	//	log.Printf("[db][FetchAppDeveloperWithSecretKey] developer - error: Key could not be encrypted %v\n", err)
	//	return util.ErrorResponse(ctx, fiber.StatusBadRequest, "invalid x-orchdio-key header")
	//}
	//
	//encodedKey := base64.StdEncoding.EncodeToString(encryptedKey)
	//log.Printf("[db][FetchAppDeveloperWithSecretKey] developer -  encoded key: %s\n", encodedKey)

	var developer blueprint.User
	err := a.DB.QueryRowx(queries.FetchAuthorizedAppDeveloperBySecretKey, key).StructScan(&developer)
	if err != nil {
		log.Printf("[db][FetchAppDeveloperWithSecretKey] developer -  error: could not fetch app developer with secret %v\n", err)
		if errors.Is(err, sql.ErrNoRows) {
			return util.ErrorResponse(ctx, fiber.StatusNotFound, "app not found")
		}
		return util.ErrorResponse(ctx, fiber.StatusBadRequest, "invalid x-orchdio-key header")
	}
	ctx.Locals("developer", &developer)
	return ctx.Next()
}

func (a *AuthMiddleware) HandleTrolls(ctx *fiber.Ctx) error {
	var blacklists = []string{"/.env", "/_profiler/phpinfo", "/.htcaccess", "/robot.txt", "/admin.php"}
	for _, blacklist := range blacklists {
		if strings.Contains(ctx.Path(), blacklist) {
			log.Printf("[middleware][HandleTrolls] warning - Trolling attempt from IP: %s at path: %s at time: %s\n", ctx.IP(), ctx.Path(), time.Now().String())
			return util.ErrorResponse(ctx, http.StatusExpectationFailed, "lol 🖕🏾")
		}
	}
	return ctx.Next()
}

// CheckOrInitiateUserAuthStatus checks if the user is already authenticated on orchdio. If the user has been authorized, we will
// continue to the next handler in line by proceeding to next but if the user is not authenticated then we
// will return a redirect auth for the platform the user is trying to perform an action and or auth on.
func (a *AuthMiddleware) CheckOrInitiateUserAuthStatus(ctx *fiber.Ctx) error {
	// extract the user id from the path. tbhe assumotion here is that the user id would be passed in the endpoints path
	userId := ctx.Params("userId")
	// attach appId as query params. if we change the verb to POST, we can simply attach the appId to the header as
	// x-orchdio-app-id
	appId := ctx.Query("app_id")
	if userId == "" {
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Missing user id")
	}

	if appId == "" {
		log.Printf("[middleware][CheckUserAuthStatus] missing app id")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Missing app id")
	}

	isValidUserId := util.IsValidUUID(userId)
	if !isValidUserId {
		log.Printf("[middleware][CheckUserAuthStatus] invalid user id")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid user id")
	}

	// extract the platform to auth for
	platform := ctx.Query("platform")
	if platform == "" {
		log.Printf("[middleare][auth][CheckUserAuthStatus] - platform not present. Please specify platform to auth user on")
		return util.ErrorResponse(ctx, http.StatusBadRequest, "Invalid Auth platform")
	}

	// find the user in the db with the id
	database := db.NewDB{DB: a.DB}
	user, err := database.FindUserByUUID(userId)
	if err != nil {
		if err == sql.ErrNoRows {
			return util.ErrorResponse(ctx, http.StatusNotFound, "User not found")
		}
		return util.ErrorResponse(ctx, http.StatusInternalServerError, err)
	}

	// check if the user is authenticated on orchdio
	if !user.Authorized {
		log.Printf("[middleware][CheckUserAuthStatus] user is not authorized. Redirecting to auth page. %s\n", userId)
		// TODO: handle fetching auth url depending on the platform here.
		developerApp, err := database.FetchAppByAppId(appId)
		if err != nil {
			log.Printf("[middleware][CheckUserAuthStatus] could not retrieve developer app")
			return util.ErrorResponse(ctx, http.StatusNotFound, "Invalid App ID")
		}

		redirectToken := blueprint.AppAuthToken{
			App:         appId,
			RedirectURL: developerApp.RedirectURL,
			Action: struct {
				Payload interface{} `json:"payload"`
				Action  string      `json:"action"`
			}(struct {
				Payload interface{}
				Action  string
			}{Payload: nil, Action: "app_auth"}),
			Platform: platform,
		}

		switch platform {
		case "spotify":
			encryptedAuthToken, err := util.SignAuthJwt(&redirectToken)
			if err != nil {
				log.Printf("[middleware][auth][CheckUserAuthStatus] - could not sign auth token. This is a serious error - %v\n", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, "Could not sign JWT token")
			}

			authURL := spotify.FetchAuthURL(string(encryptedAuthToken))
			log.Printf("[middleware][auth][CheckUserAuthStatus] - user auth redirect url - %v\n", string(authURL))
			return util.SuccessResponse(ctx, http.StatusOK, string(authURL))

		case "deezer":
			log.Printf("[middleware][auth][CheckAuthMiddleware] - generating user deezer auth redirect url")
			encryptedToken, err := util.SignAuthJwt(&redirectToken)
			if err != nil {
				log.Printf("[middleare][auth][CheckUserAuthStatus] - could not sign auth token. This is a serious error: %v\n", err)
				return util.ErrorResponse(ctx, http.StatusInternalServerError, "Could not sign redirect token")
			}
			return util.SuccessResponse(ctx, http.StatusOK, string(encryptedToken))

		case "applemusic":
			log.Printf("[middleware][CheckUserAuthStatus] - generating apple music auth token")
			return ctx.Status(http.StatusNotImplemented).JSON(fiber.Map{
				"message": "Apple Music auth not implemented yet",
			})
		}
	}

	return ctx.Next()
}
