package ytmusic

type TrackSearchResult struct {
	Tracks []struct {
		VideoId    string `json:"videoId"`
		PlaylistId string `json:"playlistId"`
		Title      string `json:"title"`
		Artists    []struct {
			Name string `json:"name"`
			Id   string `json:"id"`
		} `json:"artists"`
		Album struct {
			Name string `json:"name"`
			Id   string `json:"id"`
		} `json:"album"`
		Duration   int  `json:"duration"`
		IsExplicit bool `json:"isExplicit"`
		Thumbnails []struct {
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnails"`
	} `json:"tracks"`
	Artists []struct {
		BrowseId   string `json:"browseId"`
		Artist     string `json:"artist"`
		ShuffleId  string `json:"shuffleId"`
		RadioId    string `json:"radioId"`
		Thumbnails []struct {
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnails"`
	} `json:"artists"`
	Albums []struct {
		BrowseId string `json:"browseId"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Artists  []struct {
			Name string `json:"name"`
			Id   string `json:"id"`
		} `json:"artists"`
		Year       string `json:"year"`
		IsExplicit bool   `json:"isExplicit"`
		Thumbnails []struct {
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnails"`
	} `json:"albums"`
	Playlists []struct {
		BrowseId   string `json:"browseId"`
		Title      string `json:"title"`
		Author     string `json:"author"`
		ItemCount  string `json:"itemCount"`
		Thumbnails []struct {
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnails"`
	} `json:"playlists"`
	Videos []struct {
		VideoId    string `json:"videoId"`
		PlaylistId string `json:"playlistId"`
		Title      string `json:"title"`
		Artists    []struct {
			Name string `json:"name"`
			Id   string `json:"id"`
		} `json:"artists"`
		Views      string `json:"views"`
		Duration   int    `json:"duration"`
		Thumbnails []struct {
			Url    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnails"`
	} `json:"videos"`
}
