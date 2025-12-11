package tidal_v2

type SuccessResponse[T any, K any, V any] struct {
	Data     T   `json:"data"`
	Included []K `json:"included"`
	Links    V   `json:"links"`
}

type LinksMeta struct {
	Description string `json:"description,omitempty"`
	NextCursor  string `json:"nextCursor"`
}

type Links struct {
	Meta LinksMeta `json:"meta"`
	Next string    `json:"next"`
	Self string    `json:"self"`
}
