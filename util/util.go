package util

// THIS CODE IS JUST A MODIFIED COPY/PASTE VERSION OF THIS:
// https://github.com/gtank/cryptopasta/blob/master/encrypt.go
import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"
	"orchdio/blueprint"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/teris-io/shortid"
	"golang.org/x/text/unicode/norm"
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

	log.Println("DEBUG:: SIGN LOGIN TOKEN", os.Getenv("JWT_SECRET"))
	token, err := to.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		log.Printf("[util]: [SignOrgLoginJWT] error -  could not sign token %v", err)
		return nil, err
	}

	return []byte(token), nil
}

// SignJwt create a new jwt token
func SignJwt(claims *blueprint.OrchdioUserToken) ([]byte, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &blueprint.OrchdioUserToken{
		UUID:               claims.UUID,
		Email:              claims.Email,
		Platforms:          claims.Platforms,
		LastAuthedPlatform: claims.LastAuthedPlatform,
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
		Email:       claims.Email,
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

// GetFormattedDuration returns the duration of a track in format “hh:mm:ss“
// the duration passed is given in milliseconds.
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

// GenerateResetToken generates a reset token to be sent in email.
// The token is 10 characters long and differs from the above normal
// GenerateShortID function in that it uses a different worker id
// to generate the token, to make it more unique/secure.
func GenerateResetToken(worker ...int) []byte {
	const format = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"
	wrk := 0
	if len(worker) == 0 {
		wrk = 1
	} else {
		wrk = worker[0]
	}

	sid, err := shortid.New(uint8(wrk), format, 2342)
	if err != nil {
		log.Printf("\n[main][GenerateResetToken] - error generating short id %v\n", err)
		return nil
	}
	shortID, err := sid.Generate()
	if err != nil {
		log.Printf("\n[main][GenerateResetToken] - error generating short id %v\n", err)
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

func ExtractTitle(text string) blueprint.ExtractedTitleInfo {
	// Enhanced version to extract title, artists, and subtitle (remix info)
	// Handles formats like:
	// - "I Don't Know Why - Manoo Remix"
	// - "I Don't Know Why (Manoo Remix)"
	// - "I Don't Know Why (feat. Beverly)"
	// - "I Don't Know Why (feat. Beverly) - Manoo Remix"

	title := text
	artistes := make([]string, 0)
	subtitle := ""

	// Pattern 1: Extract content in parentheses or after hyphen
	// This handles both (remix/feat) and - remix patterns
	parenthesesPattern := regexp.MustCompile(`(?i)\s*[(\[]\s*(.+?)\s*[)\]]\s*`)
	hyphenPattern := regexp.MustCompile(`(?i)\s*-\s*(.+?)\s*(?:remix|mix|edit|version|rework)\s*$`)

	// First, check for hyphen-based remix (e.g., "- Manoo Remix")
	hyphenMatches := hyphenPattern.FindStringSubmatch(title)
	if len(hyphenMatches) > 1 {
		subtitle = strings.TrimSpace(hyphenMatches[1] + " Remix")
		// Remove the remix part from title
		title = hyphenPattern.ReplaceAllString(title, "")
	}

	// Find all parentheses/bracket content
	allMatches := parenthesesPattern.FindAllStringSubmatch(title, -1)

	for _, match := range allMatches {
		if len(match) < 2 {
			continue
		}

		content := strings.TrimSpace(match[1])
		lowerContent := strings.ToLower(content)

		// Check if it's a featured artist pattern
		featPattern := regexp.MustCompile(`(?i)^(?:feat\.?|ft\.?|featuring|with)\s+(.+)$`)
		featMatches := featPattern.FindStringSubmatch(content)

		if len(featMatches) > 1 {
			// Extract featured artists
			featArtists := parseFeaturedArtistes(featMatches[1])
			artistes = append(artistes, featArtists...)
			// Remove from title
			title = strings.Replace(title, match[0], "", 1)
		} else if strings.Contains(lowerContent, "remix") ||
			strings.Contains(lowerContent, "mix") ||
			strings.Contains(lowerContent, "edit") ||
			strings.Contains(lowerContent, "version") ||
			strings.Contains(lowerContent, "rework") {
			// It's likely a remix/subtitle
			if subtitle == "" {
				subtitle = content
			}
			// Remove from title
			title = strings.Replace(title, match[0], "", 1)
		} else {
			// Check if content contains "feat" or "ft" anywhere (not just at start)
			if strings.Contains(lowerContent, "feat.") ||
				strings.Contains(lowerContent, "feat ") ||
				strings.Contains(lowerContent, "ft.") ||
				strings.Contains(lowerContent, "ft ") ||
				strings.Contains(lowerContent, "featuring") {
				// Extract artists after feat/ft
				afterFeatPattern := regexp.MustCompile(`(?i).*?(?:feat\.?|ft\.?|featuring)\s+(.+)`)
				afterFeatMatches := afterFeatPattern.FindStringSubmatch(content)
				if len(afterFeatMatches) > 1 {
					featArtists := parseFeaturedArtistes(afterFeatMatches[1])
					artistes = append(artistes, featArtists...)
				}
				title = strings.Replace(title, match[0], "", 1)
			} else if subtitle == "" {
				// Ambiguous content - could be subtitle
				subtitle = content
				title = strings.Replace(title, match[0], "", 1)
			}
		}
	}

	// Clean up the title
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "[] -")
	title = strings.TrimSpace(title)

	// Fallback: if no structured extraction worked, try simple bracket removal
	if title == text {
		simplePattern := regexp.MustCompile(`^(?i)\s*\[\s*(.+?)\s*\]\s*$`)
		simpleMatches := simplePattern.FindStringSubmatch(text)
		if len(simpleMatches) > 1 {
			title = strings.TrimSpace(simpleMatches[1])
		}
	}

	res := blueprint.ExtractedTitleInfo{
		Title:    title,
		Artists:  artistes,
		Subtitle: subtitle,
	}
	return res
}

func parseFeaturedArtistes(feat string) []string {
	if feat == "" {
		return nil
	}
	result := regexp.MustCompile(`\s*&\s*|\s*,\s*|\s+and\s+`).Split(feat, -1)
	// Trim whitespace from each artist
	for i, artist := range result {
		result[i] = strings.TrimSpace(artist)
	}
	return result
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

func DecryptIntegrationCredentials(encryptedCredentials []byte) (*blueprint.IntegrationCredentials, error) {
	if len(encryptedCredentials) == 0 {
		return nil, blueprint.ErrNoCredentials
	}
	decrypted, err := Decrypt(encryptedCredentials, []byte(os.Getenv("ENCRYPTION_SECRET")))
	if err != nil {
		return nil, err
	}
	var credentials blueprint.IntegrationCredentials
	err = json.Unmarshal(decrypted, &credentials)
	if err != nil {
		return nil, err
	}
	return &credentials, nil
}

// FetchIdentifierOption returns the type of identifier being used to fetch a user info. An identifier is either
// email or id. id is the user's Orchdio id.
// it returns two values — a boolean and a byte. if the identifier could be fetched, it returns a true and byte
// of strings of the option. In the other case, it will return a false and a byte of strings containing the error message.
func FetchIdentifierOption(identifier string) (bool, []byte) {
	log.Printf("[db][FetchUserByIdentifier] - fetching user profile by identifier")
	var opt string
	isUUID := IsValidUUID(identifier)
	parsedEmail, err := mail.ParseAddress(identifier)
	if err != nil {
		log.Printf("[db][FetchUserByIdentifier] warning - invalid email used as identifier for fetching user info %s\n", identifier)
		opt = "id"
	}

	isValidEmail := parsedEmail != nil
	if !isUUID && !isValidEmail {
		log.Printf("[db][FetchUserByIdentifier] - invalid identifier '%s'\n", identifier)
		return false, []byte(errors.New("invalid identifier").Error())
	}

	if isUUID {
		opt = "id"
	} else {
		opt = "email"
	}
	return true, []byte(opt)
}

// SumUpResultLength sums up the length of all the tracks in a slice of TrackSearchResult
func SumUpResultLength(tracks *[]blueprint.TrackSearchResult) int {
	if tracks == nil {
		return 0
	}

	var length int
	for _, track := range *tracks {
		length += track.DurationMilli
	}
	return length
}

func FormatPlaylistTrackByCacheKeyID(platform, trackID string) string {
	return fmt.Sprintf("%s:track:%s", platform, trackID)
}

// fixme: perhaps pass "artists" and join

func FormatTargetPlaylistTrackByCacheKeyTitle(platform, artist string, title string) string {
	return fmt.Sprintf("%s:track:title-artist-%s:%s", platform, artist, title)
}

func FormatPlatformConversionCacheKey(playlistID, platform string) string {
	return fmt.Sprintf("%s:playlist:%s", platform, playlistID)
}

func FormatPlatformPlaylistSnapshotID(platform, playlistID string) string {
	return fmt.Sprintf("%s:snapshot:%s", platform, playlistID)
}

func CacheTrackByID(track *blueprint.TrackSearchResult, red *redis.Client, identifier string) bool {
	key := FormatPlaylistTrackByCacheKeyID(identifier, track.ID)
	value, err := json.Marshal(track)
	if err != nil {
		log.Printf("ERROR [services][spotify][CachePlaylistTracksWithID] json.Marshal error: %v\n", err)
		return false
	}

	// fixme(note): setting without expiry
	mErr := red.Set(context.Background(), key, value, 0).Err()
	if mErr != nil {
		log.Printf("ERROR [services][%s][CachePlaylistTracksWithID] Set error: %v\n", identifier, mErr)
		return false
	}
	return true
}

func CacheTrackByArtistTitle(track *blueprint.TrackSearchResult, red *redis.Client, identifier string) bool {
	key := FormatTargetPlaylistTrackByCacheKeyTitle(identifier, track.Artists[0], track.Title)
	value, err := json.Marshal(track)
	if err != nil {
		log.Printf("ERROR [services][spotify][CachePlaylistTracksWithID] json.Marshal error: %v\n", err)
		return false
	}

	// fixme(note): setting without expiry
	mErr := red.Set(context.Background(), key, value, 0).Err()
	if mErr != nil {
		log.Printf("ERROR [services][spotify][CachePlaylistTracksWithID] Set error: %v\n", err)
		return false
	}
	return true
}

func ContainsElement(collections []string, element string) bool {
	cpy := collections
	for _, elem := range cpy {
		if strings.Contains(element, elem) {
			return true
		}
		continue
	}
	return false
}

const possible = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func GenerateCodeVerifierAndChallenge() (string, string, error) {
	randomBytes := make([]byte, 64)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", err
	}

	var codeVerifier strings.Builder
	codeVerifier.Grow(64)
	for _, b := range randomBytes {
		codeVerifier.WriteByte(possible[b%byte(len(possible))])
	}

	hash := sha256.New()
	// write the code verifier bytes to the hash
	hash.Write([]byte(codeVerifier.String()))

	// get the final hash sum
	hashed := hash.Sum(nil)
	codeChallenge := base64.RawURLEncoding.EncodeToString(hashed)

	return codeVerifier.String(), codeChallenge, nil
}

func HasTokenExpired(expiresIn string) (bool, error) {
	expiryTime, err := time.Parse(time.RFC3339, expiresIn)
	if err != nil {
		return true, fmt.Errorf("failed to parse expiry time: %v", err)
	}
	now := time.Now()
	return now.After(expiryTime), nil
}
