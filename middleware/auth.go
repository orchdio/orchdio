package middleware

import (
	"database/sql"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"log"
	"net/http"
	"orchdio/db"
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

func (a *AuthMiddleware) AddAPIDeveloperToContext(ctx *fiber.Ctx) error {
	// get the api key from the header
	apiKey := ctx.Get("x-orchdio-key")

	if len([]byte(apiKey)) > 36 {
		log.Printf("[middleware][ValidateKey] key is too long. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Key too long")
	}

	if apiKey == "" {
		log.Printf("[middleware][ValidateKey] key is empty. %s\n", apiKey)
		return util.ErrorResponse(ctx, http.StatusUnauthorized, "Key is empty")
	}

	isValid := util.IsValidUUID(apiKey)
	if isValid {
		log.Printf("[middleware][ValidateKey] key is valid. %s\n", apiKey)
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
		ctx.Locals("developer", user)
		return ctx.Next()
	}
	return util.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid apikey")
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

// CheckUserAuthStatus checks if the user is already authenticated on orchdio. If the user has been authorized, we will
// continue to the next handler in line by proceeding to next but if the user is not authenticated then we
// will return a redirect auth for the platform the user is trying to perform an action and or auth on.
func (a *AuthMiddleware) CheckUserAuthStatus(ctx *fiber.Ctx) error {

	// check for the user in the session

	return ctx.Next()
	//return util.SuccessResponse(ctx, http.StatusOK, "User is authenticated")
}
