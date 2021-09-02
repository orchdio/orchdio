package types

type (
	SpotifyUser struct {
		Name      string   `json:"name,omitempty"`
		Moniker   string   `json:"moniker"`
		Platforms []string `json:"platforms"`
		Email     string   `json:"email"`
	}
)

type ControllerError struct {
	Message string `json:"message"`
	Status int `json:"status"`
	Error interface{} `json:"error,omitempty"`
}

type ControllerResult struct {
	Message string `json:"message"`
	Data interface{} `json:"data,omitempty"`
	Status int `json:"status"`
}