package soundcloud_sdk

const (
	// API Base URLs
	APIBaseURL  = "https://api.soundcloud.com"
	AuthBaseURL = "https://secure.soundcloud.com"

	// API Endpoints
	EndpointMe        = "/me"
	EndpointTracks    = "/tracks"
	EndpointPlaylists = "/playlists"
	EndpointUsers     = "/users"
	EndpointSearch    = "/search"
	EndpointResolve   = "/resolve"
	EndpointComments  = "/comments"
	EndpointLikes     = "/likes"

	// Rate Limits
	// Client Credentials Flow has rate limits:
	// - 50 tokens per 12 hours per app
	// - 30 tokens per 1 hour per IP address
	MaxTokensPer12Hours = 50
	MaxTokensPerHour    = 30

	// Token Configuration
	DefaultTokenExpiry = 3600 // 1 hour in seconds

	// PKCE Configuration
	PKCEMethod         = "S256"
	CodeVerifierLength = 32  // bytes (results in 43 characters when base64url encoded)
	MinVerifierLength  = 43  // characters
	MaxVerifierLength  = 128 // characters

	// OAuth Grant Types
	GrantTypeAuthorizationCode = "authorization_code"
	GrantTypeClientCredentials = "client_credentials"
	GrantTypeRefreshToken      = "refresh_token"

	// Default Limits
	DefaultLimit = 50
	MaxLimit     = 200
)

// AccessLevel represents the level of access to a track
type AccessLevel string

const (
	AccessPlayable AccessLevel = "playable"
	AccessPreview  AccessLevel = "preview"
	AccessBlocked  AccessLevel = "blocked"
)
