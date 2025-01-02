package blueprint

type ConvoyWebhookCreate struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Description string `json:"description"`
}
