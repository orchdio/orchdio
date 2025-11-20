package blueprint

type UserListeningHistory struct {
	Data  []TrackSearchResult `json:"data"`
	Total int                 `json:"total"`
}
