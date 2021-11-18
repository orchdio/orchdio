package util

// THIS CODE IS JUST A MODIFIED COPY/PASTE VERSION OF THIS:
// https://github.com/gtank/cryptopasta/blob/master/encrypt.go
import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"zoove/blueprint"
)

// Encrypt encrypts data using 256-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Output takes the
// form nonce|ciphertext|tag where '|' indicates concatenation.
func Encrypt(plaintext []byte, key []byte) (ciphertext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data using 256-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Expects input
// form nonce|ciphertext|tag where '|' indicates concatenation.
func Decrypt(ciphertext []byte, key []byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("malformed ciphertext")
	}

	return gcm.Open(nil,
		ciphertext[:gcm.NonceSize()],
		ciphertext[gcm.NonceSize():],
		nil,
	)
}

// SuccessResponse sends back a success http response to the client.
func SuccessResponse(ctx *fiber.Ctx, statusCode int, data interface{}) error {
	return ctx.Status(statusCode).JSON(fiber.Map{
		"message": "Request Ok",
		"status":  statusCode,
		"data":    data,
	})
}

// ErrorResponse sends back an error http response to the client.
func ErrorResponse(ctx *fiber.Ctx, statusCode int, err interface{}) error {
	return ctx.Status(statusCode).JSON(fiber.Map{
		"message": "Error with response",
		"status":  statusCode,
		"error":   err,
	})
}

// SignJwt create a new jwt token
func SignJwt(claims *blueprint.ZooveUserToken) ([]byte, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &blueprint.ZooveUserToken{
		PlatformID: claims.PlatformID,
		Platform:   claims.Platform,
		Role:       claims.Role,
		Email:      claims.Email,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * 12).Unix(),
			IssuedAt:  time.Now().Unix(),
		},
	})

	jToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		log.Printf("[util]: [SignJwt] - Error signing token %v", err)
		return nil, err
	}
	return []byte(jToken), nil
}

func Find(s []string, e string) string {
	for _, a := range s {
		if a == e {
			return a
		}
	}
	return ""
}

// DeezerIsExplicit returns a true or false specifying if it's an explicit content
func DeezerIsExplicit(v int) bool {
	out := false
	if v == 1 {
		out = true
	}
	return out
}

// GetFormattedDuration returns the duration of a track in format ``hh:mm``
func GetFormattedDuration(v int) string {
	hour := v / 60
	sec := v % 60
	seconds := strconv.Itoa(sec)
	if len(seconds) == 1 {
		seconds = "0" + seconds
	}
	return fmt.Sprintf("%d:%s", hour, seconds)
}

// ExtractSpotifyID returns the spotify ID from a playlist pagination link
func ExtractSpotifyID(link string) string {
	firstIndex := strings.Index(link, "playlists/") + len("playlists/")
	lastIndex := strings.LastIndex(link, "/")

	if lastIndex < firstIndex {
		// get the index of ? incase there are nonsense tracking links attached
		qIndex := strings.Index(link, "?")
		if qIndex != -1 {
			link = link[:qIndex]
		}
		return link[firstIndex:]
	}
	return link[firstIndex:lastIndex]
}

// ExtractDeezerID returns the deezer ID from a playlist pagination link
func ExtractDeezerID(link string) string {
	firstIndex := strings.Index(link, "playlist/") + len("playlist/")
	lastIndex := strings.LastIndex(link, "/")

	if lastIndex < firstIndex {
		// get the index of ? incase there are nonsense tracking links attached
		qIndex := strings.Index(link, "?")
		if qIndex != -1 {
			link = link[:qIndex]
		}
		return link[firstIndex:]
	}
	return link[firstIndex:lastIndex]
}
