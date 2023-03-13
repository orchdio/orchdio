package applemusic

import "time"

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

type PlaylistCatalogInfoResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			CuratorName      string    `json:"curatorName"`
			LastModifiedDate time.Time `json:"lastModifiedDate"`
			Name             string    `json:"name"`
			IsChart          bool      `json:"isChart"`
			PlaylistType     string    `json:"playlistType"`
			Description      struct {
				Standard string `json:"standard"`
				Short    string `json:"short"`
			} `json:"description"`
			Artwork struct {
				Width      int    `json:"width"`
				Height     int    `json:"height"`
				URL        string `json:"url"`
				BgColor    string `json:"bgColor"`
				TextColor1 string `json:"textColor1"`
				TextColor2 string `json:"textColor2"`
				TextColor3 string `json:"textColor3"`
				TextColor4 string `json:"textColor4"`
			} `json:"artwork"`
			PlayParams struct {
				ID          string `json:"id"`
				Kind        string `json:"kind"`
				VersionHash string `json:"versionHash"`
			} `json:"playParams"`
			Url string `json:"url"`
		} `json:"attributes"`
	} `json:"data"`
}

type PlaylistInfoResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			CanEdit     bool   `json:"canEdit"`
			Name        string `json:"name"`
			IsPublic    bool   `json:"isPublic"`
			Description struct {
				Standard string `json:"standard"`
			} `json:"description"`
			HasCatalog bool `json:"hasCatalog"`
			PlayParams struct {
				ID        string `json:"id"`
				Kind      string `json:"kind"`
				IsLibrary bool   `json:"isLibrary"`
				GlobalID  string `json:"globalId"`
			} `json:"playParams"`
			DateAdded time.Time `json:"dateAdded"`
		} `json:"attributes"`
	} `json:"data"`
}

type PlaylistTracksResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			DiscNumber       int      `json:"discNumber"`
			AlbumName        string   `json:"albumName"`
			GenreNames       []string `json:"genreNames"`
			TrackNumber      int      `json:"trackNumber"`
			HasLyrics        bool     `json:"hasLyrics"`
			ReleaseDate      string   `json:"releaseDate"`
			DurationInMillis int      `json:"durationInMillis"`
			Name             string   `json:"name"`
			ArtistName       string   `json:"artistName"`
			ContentRating    string   `json:"contentRating"`
			Artwork          struct {
				Width  int    `json:"width"`
				Height int    `json:"height"`
				URL    string `json:"url"`
			} `json:"artwork"`
			PlayParams struct {
				ID          string `json:"id"`
				Kind        string `json:"kind"`
				IsLibrary   bool   `json:"isLibrary"`
				Reporting   bool   `json:"reporting"`
				CatalogID   string `json:"catalogId"`
				ReportingID string `json:"reportingId"`
			} `json:"playParams"`
		} `json:"attributes,omitempty"`
	} `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

type UserPlaylistResponse struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Public        bool   `json:"public"`
	Collaborative bool   `json:"collaborative"`
	NbTracks      int    `json:"nb_tracks"`
	URL           string `json:"url"`
	Cover         string `json:"cover"`
	CreatedAt     string `json:"created_at"`
	Owner         string `json:"owner"`
	Description   string `json:"description"`
}

type UserArtistsResponse struct {
	Data []struct {
		Id         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"data"`
	Meta struct {
		Total int `json:"total"`
		Sorts []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"sorts"`
		CurrentSort string `json:"currentSort"`
	} `json:"meta"`
	Next string `json:"next,omitempty"`
}

type UserArtistInfoResponse struct {
	Data []struct {
		Id         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			GenreNames []string `json:"genreNames"`
			Name       string   `json:"name"`
			Artwork    struct {
				Width      int    `json:"width"`
				Height     int    `json:"height"`
				Url        string `json:"url"`
				BgColor    string `json:"bgColor"`
				TextColor1 string `json:"textColor1"`
				TextColor2 string `json:"textColor2"`
				TextColor3 string `json:"textColor3"`
				TextColor4 string `json:"textColor4"`
			} `json:"artwork"`
			Url string `json:"url"`
		} `json:"attributes"`
		Relationships struct {
			Albums struct {
				Href string `json:"href"`
				Next string `json:"next"`
				Data []struct {
					Id   string `json:"id"`
					Type string `json:"type"`
					Href string `json:"href"`
				} `json:"data"`
			} `json:"albums"`
		} `json:"relationships"`
	} `json:"data"`
}

type UserAlbumsResponse struct {
	Data []struct {
		Id         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			TrackCount  int      `json:"trackCount"`
			GenreNames  []string `json:"genreNames"`
			ReleaseDate string   `json:"releaseDate"`
			Name        string   `json:"name"`
			ArtistName  string   `json:"artistName"`
			Artwork     struct {
				Width  int    `json:"width"`
				Height int    `json:"height"`
				Url    string `json:"url"`
			} `json:"artwork"`
			DateAdded  time.Time `json:"dateAdded"`
			PlayParams struct {
				Id        string `json:"id"`
				Kind      string `json:"kind"`
				IsLibrary bool   `json:"isLibrary"`
			} `json:"playParams"`
		} `json:"attributes"`
		Relationships struct {
			Tracks struct {
				Href string `json:"href"`
				Data []struct {
					Id         string `json:"id"`
					Type       string `json:"type"`
					Href       string `json:"href"`
					Attributes struct {
						DiscNumber       int      `json:"discNumber"`
						AlbumName        string   `json:"albumName"`
						GenreNames       []string `json:"genreNames"`
						HasLyrics        bool     `json:"hasLyrics"`
						TrackNumber      int      `json:"trackNumber"`
						ReleaseDate      string   `json:"releaseDate"`
						DurationInMillis int      `json:"durationInMillis"`
						Name             string   `json:"name"`
						ContentRating    string   `json:"contentRating"`
						ArtistName       string   `json:"artistName"`
						Artwork          struct {
							Width  int    `json:"width"`
							Height int    `json:"height"`
							Url    string `json:"url"`
						} `json:"artwork"`
						PlayParams struct {
							Id          string `json:"id"`
							Kind        string `json:"kind"`
							IsLibrary   bool   `json:"isLibrary"`
							Reporting   bool   `json:"reporting"`
							CatalogId   string `json:"catalogId"`
							ReportingId string `json:"reportingId"`
						} `json:"playParams"`
					} `json:"attributes"`
				} `json:"data"`
				Meta struct {
					Total int `json:"total"`
				} `json:"meta"`
			} `json:"tracks"`
		} `json:"relationships"`
	} `json:"data"`
	Meta struct {
		Total int `json:"total"`
		Sorts []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"sorts"`
		CurrentSort string `json:"currentSort"`
	} `json:"meta"`
}

type UserAlbumsCatalogResponse struct {
	Data []struct {
		Id         string `json:"id"`
		Type       string `json:"type"`
		Href       string `json:"href"`
		Attributes struct {
			Copyright           string   `json:"copyright"`
			GenreNames          []string `json:"genreNames"`
			ReleaseDate         string   `json:"releaseDate"`
			Upc                 string   `json:"upc"`
			IsMasteredForItunes bool     `json:"isMasteredForItunes"`
			Artwork             struct {
				Width      int    `json:"width"`
				Height     int    `json:"height"`
				Url        string `json:"url"`
				BgColor    string `json:"bgColor"`
				TextColor1 string `json:"textColor1"`
				TextColor2 string `json:"textColor2"`
				TextColor3 string `json:"textColor3"`
				TextColor4 string `json:"textColor4"`
			} `json:"artwork"`
			PlayParams struct {
				Id   string `json:"id"`
				Kind string `json:"kind"`
			} `json:"playParams"`
			Url           string `json:"url"`
			RecordLabel   string `json:"recordLabel"`
			IsCompilation bool   `json:"isCompilation"`
			TrackCount    int    `json:"trackCount"`
			IsSingle      bool   `json:"isSingle"`
			Name          string `json:"name"`
			ContentRating string `json:"contentRating"`
			ArtistName    string `json:"artistName"`
			IsComplete    bool   `json:"isComplete"`
		} `json:"attributes"`
	} `json:"data"`
}
