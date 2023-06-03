package util

// THIS CODE IS JUST A MODIFIED COPY/PASTE VERSION OF THIS:
// https://github.com/gtank/cryptopasta/blob/master/encrypt.go
import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/teris-io/shortid"
	"golang.org/x/text/unicode/norm"
	"io"
	"log"
	"orchdio/blueprint"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	if data == nil {
		return ctx.Status(statusCode).JSON(&fiber.Map{
			"message": "Success",
			"status":  statusCode,
		})
	}
	return ctx.Status(statusCode).JSON(
		fiber.Map{
			"message": "Request Ok",
			"status":  statusCode,
			"data":    data,
		})
}

// ErrorResponse sends back an error http response to the client.
func ErrorResponse(ctx *fiber.Ctx, statusCode int, err interface{}, message string) error {
	return ctx.Status(statusCode).JSON(&blueprint.ErrorResponse{
		Message: message,
		Status:  statusCode,
		Error:   err,
	})
}

// SignOrgLoginJWT signs the org login jwt token with the passed params
func SignOrgLoginJWT(claims *blueprint.AppJWT) ([]byte, error) {
	to := jwt.NewWithClaims(jwt.SigningMethodHS256, &blueprint.AppJWT{
		OrgID:       claims.OrgID,
		DeveloperID: claims.DeveloperID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 12)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	token, err := to.SignedString([]byte(os.Getenv("jwt_secret")))
	if err != nil {
		log.Printf("[util]: [SignOrgLoginJWT] error -  could not sign token %v", err)
		return nil, err
	}

	return []byte(token), nil
}

// SignJwt create a new jwt token
func SignJwt(claims *blueprint.OrchdioUserToken) ([]byte, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &blueprint.OrchdioUserToken{
		UUID:     claims.UUID,
		Email:    claims.Email,
		Platform: claims.Platform,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 12)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	jToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		log.Printf("[util]: [SignJwt] - Error signing token %v", err)
		return nil, err
	}
	return []byte(jToken), nil
}

// SignAuthJwt signs the auth jwt token with the passed params
func SignAuthJwt(claims *blueprint.AppAuthToken) ([]byte, error) {
	// this token will expire in 10 mins.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &blueprint.AppAuthToken{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute * 10)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.New().String(),
		},
		App:         claims.App,
		RedirectURL: claims.RedirectURL,
		Platform:    claims.Platform,
		Action:      claims.Action,
		Scopes:      claims.Scopes,
	})

	signedToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		log.Printf("[util]: [SignAuthJwt] error -  could not sign redirect token %v", err)
		return nil, err
	}
	return []byte(signedToken), nil
}

// DecodeAuthJwt parses the auth jwt token into a ```blueprint.AppAuthToken```
func DecodeAuthJwt(token string) (*blueprint.AppAuthToken, error) {
	// decode jwt
	decodedToken, err := jwt.ParseWithClaims(token, &blueprint.AppAuthToken{}, func(token *jwt.Token) (interface{}, error) {
		// TODO: check the alg and signature is intact
		return []byte(os.Getenv("JWT_SECRET")), nil
	})
	if err != nil {
		log.Printf("[util]: [DecodeAuthJwt] error -  could not decode redirect token %v", err)
		return nil, err
	}
	if claims, ok := decodedToken.Claims.(*blueprint.AppAuthToken); ok && decodedToken.Valid {
		return claims, nil
	} else {
		log.Printf("[util]: [DecodeAuthJwt] error -  could not decode redirect token. Claims may be wrong %v", err)
		return nil, err
	}
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

// GetFormattedDuration returns the duration of a track in format ``hh:mm:ss``
func GetFormattedDuration(v int) string {
	hour := 0
	min := v / 60
	sec := v % 60
	seconds := strconv.Itoa(sec)
	if len(seconds) == 1 {
		seconds = "0" + seconds
	}

	if min >= 24 {
		hour = min / 60
		min = min % 60
		hr := strconv.Itoa(hour)
		if len(hr) == 1 {
			hr = "0" + hr
		}

		if len(strconv.Itoa(min)) == 1 {
			return fmt.Sprintf("%s:0%s:%s", hr, strconv.Itoa(min), seconds)
		}
		return fmt.Sprintf("%s:%d:%s", hr, min, seconds)
	}

	return fmt.Sprintf("%d:%s", min, seconds)
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

func NormalizeString(src string) string {
	// normalize strings using the norm package
	bp := norm.NFD.String(src)
	// remove all non-ascii characters
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		log.Fatal(err)
	}
	// trimout spaces.
	processedString := strings.ToLower(strings.ReplaceAll(reg.ReplaceAllString(bp, ""), " ", ""))
	return processedString
}

// TODO: remove hashing and hashing references and use norm package to normalize the strings instead

// HashIdentifier returns a hash of the identifier
func HashIdentifier(id string) string {
	hasher := md5.New()
	hasher.Write([]byte(id))
	return hex.EncodeToString(hasher.Sum(nil))
}

// BuildTidalAssetURL returns a string of the tidal asset id
func BuildTidalAssetURL(id string) string {
	// for now, we get the asset type of image, at 320/320 by default
	id = strings.Replace(id, "-", "/", -1)
	return fmt.Sprintf("https://resources.tidal.com/images/%s/320x320.jpg", id)
}

// IsValidUUID checks if an id is a valid UUID
func IsValidUUID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}

// GenerateHMAC generates the hmac for a given message
func GenerateHMAC(message interface{}, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	// serialize message
	payload, err := json.Marshal(message)
	if err != nil {
		log.Printf("\n error serializing message before generating sha256 hash %v\n", err)
		return nil
	}
	mac.Write(payload)
	return []byte(hex.EncodeToString(mac.Sum(nil)))
}

// GenerateShortID generates a short id, used for short url for final conversion/entity results and deezer state. The ID is 10 characters long
func GenerateShortID() []byte {
	const format = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"
	sid, err := shortid.New(1, format, 2342)
	if err != nil {
		log.Printf("\n[main][GenerateShortID] - error generating short id %v\n", err)
		return nil
	}
	shortID, err := sid.Generate()
	if err != nil {
		log.Printf("\n[main][GenerateShortID] - error generating short id %v\n", err)
		return nil
	}
	return []byte(shortID)
}

func TidalIsCollaborative(level string) bool {
	return level == "UNRESTRICTED" // || level == "PRIVATE"
}
func TidalIsPrivate(level string) bool {
	return level == "PRIVATE"
}

// IsTaskType checks if a task type is a type of specific task
func IsTaskType(tasktype, task string) bool {
	return strings.Contains(tasktype, task)
}

func DeserializeAppCredentials(data []byte) (*blueprint.IntegrationCredentials, error) {
	var appCredentials blueprint.IntegrationCredentials
	decr, err := Decrypt(data, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		log.Printf("[util]: [DeserializeAppCredentials] error -  could not decrypt app credentials %v", err)
		return nil, err
	}

	err = json.Unmarshal(decr, &appCredentials)
	if err != nil {
		log.Printf("[util]: [DeserializeAppCredentials] error -  could not deserialize app credentials %v", err)
		return nil, err
	}
	return &appCredentials, nil
}

func DeezerIsExplicitContent(explicitContent string) bool {
	return explicitContent == "explicit_lyrics" || explicitContent == "explicit_display"
}

// DeezerSubscriptionPlan returns the subscription plan of a deezer user by checking for some fields in the opts passed
// Deezer does not have a field in the api responses that specify subscription plan, but we can infer from the
// info (based on personal testing) if some fields are present or have certain values, then the user is on a free plan or not.
// for now, we just check for few fields, we may need to extend this in the future.
func DeezerSubscriptionPlan(opts map[string]interface{}) string {
	if opts["ads_display"] == true && opts["ads_audio"] == true && opts["radio_skips"].(int) > 0 {
		return "free"
	}
	// todo: handle proper check for other plans as necessary
	return "premium"
}

// ExtractTitle will extract the title from the track title. This is important because
// it will remove the (feat. artiste) from the title and or normalize the title
// in order to enhance search results.
func ExtractTitle(text string) blueprint.ExtractedTitleInfo {
	// TODO: improve artiste title having spaces in between and more than one word
	//       implement supporting [track_title] (feat. [artiste_name]) [remix_name]
	//       implement supporting [track_title] (with) [remix_name]
	//       implement supporting [track_title] (with [artiste_name])
	// remove the (feat. artiste) from the title
	// this is a very naive way of doing it, but it works for now

	// first extraction requirement is that we want to detect when the title contains
	// a paranthesis and contains (feat. or (ft. or (ft). In any of these combinations
	// (and similar, in the future), we want to remove the content of the paranthesis
	// and also probably get the featured artiste name
	// Define the regular expression pattern to match the track title and artistes
	pattern := regexp.MustCompile(`^(?i)\s*\[\s*(.+?)\s*\]\s*([(\[]?\s*(?:with|feat\.?)[\s&]*([a-z0-9 .,;&]+)\s*[)\]]?)?\s*$`)
	// Apply the regular expression to the track title string
	matches := pattern.FindStringSubmatch(text)

	// Check if the title was matched
	if len(matches) < 2 {
		fmt.Printf("Error: Could not extract title from track title string: %v\n", text)
		// Return a fallback to the original title and an empty array of artists
		res := blueprint.ExtractedTitleInfo{
			Title:   strings.Trim(text, "[] "),
			Artists: []string{},
		}
		return res
	}

	// Extract the title and artistes from the matches
	title := strings.TrimSpace(matches[1])
	artistes := make([]string, 0)
	if matches[3] != "" {
		// If there are artistes, split them by commas, semicolons, and/or ampersands
		artistes = strings.FieldsFunc(matches[3], func(r rune) bool {
			return r == ',' || r == ';' || r == '&'
		})
		for i, a := range artistes {
			artistes[i] = strings.TrimSpace(a)
		}
	}

	// Create the final map with title and artistes keys
	res := blueprint.ExtractedTitleInfo{
		Title:   title,
		Artists: artistes,
	}
	return res
}

func parseFeaturedArtistes(feat string) []string {
	if feat == "" {
		return nil
	}
	return regexp.MustCompile(`\s*&\s*|\s*,\s*|\s+and\s+`).Split(feat, -1)
}

func FetchMethodFromInterface(service interface{}, method string) (reflect.Value, bool) {
	var ptr reflect.Value
	var serviceMethod reflect.Value
	var value reflect.Value

	serviceValue := reflect.ValueOf(service)
	if serviceValue.IsNil() {
		return serviceMethod, false
	}
	if serviceValue.Type().Kind() == reflect.Ptr {
		ptr = serviceValue
		value = ptr.Elem()
	} else {
		ptr = reflect.New(reflect.TypeOf(service))
		_temp := ptr.Elem()
		_temp.Elem().Set(serviceValue)
	}

	_method := value.MethodByName(method)
	if _method.IsValid() {
		serviceMethod = _method
	}

	serviceMethod = ptr.MethodByName(method)
	return serviceMethod, true
}
