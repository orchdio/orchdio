package blueprint

type UserAuthCredentials struct {
	Username   string `json:"username,omitempty"`
	Platform   string `json:"platform,omitempty"`
	PlatformId string `json:"platform_id,omitempty"`
	Token      []byte `json:"token,omitempty"`
}
