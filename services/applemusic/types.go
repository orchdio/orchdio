package applemusic

const IDENTIFIER = "applemusic"

type UnlimitedPlaylist struct {
	Data []struct {
		Id         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			AlbumName        string   `json:"albumName"`
			GenreNames       []string `json:"genreNames"`
			TrackNumber      int      `json:"trackNumber"`
			ReleaseDate      string   `json:"releaseDate"`
			DurationInMillis int      `json:"durationInMillis"`
			Isrc             string   `json:"isrc"`
			Artwork          struct {
				Width      int    `json:"width"`
				Height     int    `json:"height"`
				Url        string `json:"url"`
				BgColor    string `json:"bgColor"`
				TextColor1 string `json:"textColor1"`
				TextColor2 string `json:"textColor2"`
				TextColor3 string `json:"textColor3"`
				TextColor4 string `json:"textColor4"`
			} `json:"artwork"`
			ComposerName string `json:"composerName,omitempty"`
			PlayParams   struct {
				Id   string `json:"id"`
				Kind string `json:"kind"`
			} `json:"playParams"`
			Url                  string `json:"url"`
			DiscNumber           int    `json:"discNumber"`
			HasLyrics            bool   `json:"hasLyrics"`
			IsAppleDigitalMaster bool   `json:"isAppleDigitalMaster"`
			Name                 string `json:"name"`
			Previews             []struct {
				Url string `json:"url"`
			} `json:"previews"`
			ArtistName     string `json:"artistName"`
			ContentRating  string `json:"contentRating,omitempty"`
			EditorialNotes struct {
				Short string `json:"short"`
			} `json:"editorialNotes,omitempty"`
		} `json:"attributes"`
	} `json:"data"`
}
