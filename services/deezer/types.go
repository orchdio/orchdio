package deezer

type AuthResponse struct {
	AccessToken string `json:"access_token"`
}

const AuthBase = "https://connect.deezer.com/oauth"
const ApiBase = "https://api.deezer.com"
const IDENTIFIER = "deezer"

//goland:noinspection GoNameStartsWithPackageName
type DeezerUser struct {
	ID                             int      `json:"id"`
	Name                           string   `json:"name"`
	Lastname                       string   `json:"lastname"`
	Firstname                      string   `json:"firstname"`
	Email                          string   `json:"email"`
	Status                         int      `json:"status"`
	Birthday                       string   `json:"birthday"`
	InscriptionDate                string   `json:"inscription_date"`
	Gender                         string   `json:"gender"`
	Link                           string   `json:"link"`
	Picture                        string   `json:"picture"`
	PictureSmall                   string   `json:"picture_small"`
	PictureMedium                  string   `json:"picture_medium"`
	PictureBig                     string   `json:"picture_big"`
	PictureXl                      string   `json:"picture_xl"`
	Country                        string   `json:"country"`
	Lang                           string   `json:"lang"`
	IsKid                          bool     `json:"is_kid"`
	ExplicitContentLevel           string   `json:"explicit_content_level"`
	ExplicitContentLevelsAvailable []string `json:"explicit_content_levels_available"`
	Tracklist                      string   `json:"tracklist"`
	Type                           string   `json:"type"`
}

type Track struct {
	ID                    int      `json:"id"`
	Readable              bool     `json:"readable"`
	Title                 string   `json:"title"`
	TitleShort            string   `json:"title_short"`
	TitleVersion          string   `json:"title_version"`
	Isrc                  string   `json:"isrc"`
	Link                  string   `json:"link"`
	Share                 string   `json:"share"`
	Duration              int      `json:"duration"`
	TrackPosition         int      `json:"track_position"`
	DiskNumber            int      `json:"disk_number"`
	Rank                  int      `json:"rank"`
	ReleaseDate           string   `json:"release_date"`
	ExplicitLyrics        bool     `json:"explicit_lyrics"`
	ExplicitContentLyrics int      `json:"explicit_content_lyrics"`
	ExplicitContentCover  int      `json:"explicit_content_cover"`
	Preview               string   `json:"preview"`
	Bpm                   float64  `json:"bpm"`
	Gain                  float64  `json:"gain"`
	AvailableCountries    []string `json:"available_countries"`
	Contributors          []struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Link          string `json:"link"`
		Share         string `json:"share"`
		Picture       string `json:"picture"`
		PictureSmall  string `json:"picture_small"`
		PictureMedium string `json:"picture_medium"`
		PictureBig    string `json:"picture_big"`
		PictureXl     string `json:"picture_xl"`
		Radio         bool   `json:"radio"`
		Tracklist     string `json:"tracklist"`
		Type          string `json:"type"`
		Role          string `json:"role"`
	} `json:"contributors"`
	Md5Image string `json:"md5_image"`
	Artist   struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Link          string `json:"link"`
		Share         string `json:"share"`
		Picture       string `json:"picture"`
		PictureSmall  string `json:"picture_small"`
		PictureMedium string `json:"picture_medium"`
		PictureBig    string `json:"picture_big"`
		PictureXl     string `json:"picture_xl"`
		Radio         bool   `json:"radio"`
		Tracklist     string `json:"tracklist"`
		Type          string `json:"type"`
	} `json:"artist"`
	Album struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Link        string `json:"link"`
		Cover       string `json:"cover"`
		CoverSmall  string `json:"cover_small"`
		CoverMedium string `json:"cover_medium"`
		CoverBig    string `json:"cover_big"`
		CoverXl     string `json:"cover_xl"`
		Md5Image    string `json:"md5_image"`
		ReleaseDate string `json:"release_date"`
		Tracklist   string `json:"tracklist"`
		Type        string `json:"type"`
	} `json:"album"`
	Type string `json:"type"`
}
