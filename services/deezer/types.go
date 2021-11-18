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

type FullTrack struct {
	Data []struct {
		ID                    int    `json:"id"`
		Readable              bool   `json:"readable"`
		Title                 string `json:"title"`
		TitleShort            string `json:"title_short"`
		TitleVersion          string `json:"title_version"`
		Link                  string `json:"link"`
		Duration              int    `json:"duration"`
		Rank                  int    `json:"rank"`
		ExplicitLyrics        bool   `json:"explicit_lyrics"`
		ExplicitContentLyrics int    `json:"explicit_content_lyrics"`
		ExplicitContentCover  int    `json:"explicit_content_cover"`
		Preview               string `json:"preview"`
		Md5Image              string `json:"md5_image"`
		Artist                Artist `json:"artist"`
		Album                 Album  `json:"album"`
		Type                  string `json:"type"`
	} `json:"data"`
	Total int `json:"total"`
}

type PlaylistOwner struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Avatar string `json:"avatar"`
}

type SingleTrack struct {
	Title       string `json:"title"`
	Duration    int    `json:"duration"`
	Artistes    Artist `json:"artist"`
	URL         string `json:"link"`
	Preview     string `json:"preview"`
	Cover       string `json:"cover"`
	ReleaseDate string `json:"release_date"`
	Explicit    bool   `json:"explicit_lyrics"`
	Platform    string `json:"platform"`
	ID          int    `json:"id"`
	PlayedAt    string `json:"played_at,omitempty"` // this is because this struct is also used for the single listening history object which contains (and needs) a "when was it played" body which is this.
	AddedAt     string `json:"added_at,omitempty"`  // similar situation above but in this case, its for Playlists. To know when a track was added to a playlist.
	Album       Album  `json:"album"`
}

type Album struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Cover       string `json:"cover"`
	CoverSmall  string `json:"cover_small"`
	CoverMedium string `json:"cover_medium"`
	CoverBig    string `json:"cover_big"`
	CoverXl     string `json:"cover_xl"`
	Md5Image    string `json:"md5_image"`
	Tracklist   string `json:"tracklist"`
	Type        string `json:"type"`
}

type Artist struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Link      string `json:"link"`
	Tracklist string `json:"tracklist"`
	Type      string `json:"type"`
}

type Playlist struct {
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	Duration      int           `json:"duration"`
	Collaborative bool          `json:"public"`
	TracksNumber  int           `json:"tracks_number"`
	Owner         PlaylistOwner `json:"owner"`
	Tracks        struct {
		Data []SingleTrack `json:"data"`
	}
	URL   string `json:"link"`
	Cover string `json:"picture"`
}

//type PlaylistTracksSearch struct {
//	Data []struct {
//		Id                    int    `json:"id"`
//		Readable              bool   `json:"readable"`
//		Title                 string `json:"title"`
//		TitleShort            string `json:"title_short"`
//		TitleVersion          string `json:"title_version,omitempty"`
//		Link                  string `json:"link"`
//		Duration              int    `json:"duration"`
//		Rank                  int    `json:"rank"`
//		ExplicitLyrics        bool   `json:"explicit_lyrics"`
//		ExplicitContentLyrics int    `json:"explicit_content_lyrics"`
//		ExplicitContentCover  int    `json:"explicit_content_cover"`
//		Preview               string `json:"preview"`
//		Md5Image              string `json:"md5_image"`
//		TimeAdd               int    `json:"time_add"`
//		Artist                Artist `json:"artist"`
//		Album                 Album  `json:"album"`
//		Type                  string `json:"type"`
//	} `json:"data"`
//	Checksum string `json:"checksum"`
//	Total    int    `json:"total"`
//	Next     string `json:"next"`
//	Previous string `json:"previous"`
//}

type PlaylistTracksSearch struct {
	Id            int64  `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Duration      int    `json:"duration"`
	Public        bool   `json:"public"`
	IsLovedTrack  bool   `json:"is_loved_track"`
	Collaborative bool   `json:"collaborative"`
	NbTracks      int    `json:"nb_tracks"`
	Fans          int    `json:"fans"`
	Link          string `json:"link"`
	Share         string `json:"share"`
	Picture       string `json:"picture"`
	PictureSmall  string `json:"picture_small"`
	PictureMedium string `json:"picture_medium"`
	PictureBig    string `json:"picture_big"`
	PictureXl     string `json:"picture_xl"`
	Checksum      string `json:"checksum"`
	Tracklist     string `json:"tracklist"`
	CreationDate  string `json:"creation_date"`
	Md5Image      string `json:"md5_image"`
	PictureType   string `json:"picture_type"`
	Creator       struct {
		Id        int    `json:"id"`
		Name      string `json:"name"`
		Tracklist string `json:"tracklist"`
		Type      string `json:"type"`
	} `json:"creator"`
	Type   string `json:"type"`
	Tracks struct {
		Data []struct {
			Id                    int    `json:"id"`
			Readable              bool   `json:"readable"`
			Title                 string `json:"title"`
			TitleShort            string `json:"title_short"`
			TitleVersion          string `json:"title_version,omitempty"`
			Link                  string `json:"link"`
			Duration              int    `json:"duration"`
			Rank                  int    `json:"rank"`
			ExplicitLyrics        bool   `json:"explicit_lyrics"`
			ExplicitContentLyrics int    `json:"explicit_content_lyrics"`
			ExplicitContentCover  int    `json:"explicit_content_cover"`
			Preview               string `json:"preview"`
			Md5Image              string `json:"md5_image"`
			TimeAdd               int    `json:"time_add"`
			Artist                struct {
				Id        int    `json:"id"`
				Name      string `json:"name"`
				Link      string `json:"link"`
				Tracklist string `json:"tracklist"`
				Type      string `json:"type"`
			} `json:"artist"`
			Album struct {
				Id          int    `json:"id"`
				Title       string `json:"title"`
				Cover       string `json:"cover"`
				CoverSmall  string `json:"cover_small"`
				CoverMedium string `json:"cover_medium"`
				CoverBig    string `json:"cover_big"`
				CoverXl     string `json:"cover_xl"`
				Md5Image    string `json:"md5_image"`
				Tracklist   string `json:"tracklist"`
				Type        string `json:"type"`
			} `json:"album"`
			Type string `json:"type"`
		} `json:"data"`
		Checksum string `json:"checksum"`
	} `json:"tracks"`
}
