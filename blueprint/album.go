package blueprint

type LibraryAlbum struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	ReleaseDate string   `json:"release_date"`
	Explicit    bool     `json:"explicit"`
	TrackCount  int      `json:"nb_tracks"`
	Artists     []string `json:"artists"`
	Cover       string   `json:"cover"`
}

type UserLibraryAlbums struct {
	Payload []LibraryAlbum `json:"payload"`
	Total   int            `json:"total"`
}
