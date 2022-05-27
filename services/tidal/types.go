package tidal

import "github.com/golang-jwt/jwt/v4"

const IDENTIFIER = "tidal"

// Track represents a single track from Tidal.
type Track struct {
	ID                   int         `json:"id"`
	Title                string      `json:"title"`
	Duration             int         `json:"duration"`
	ReplayGain           float64     `json:"replayGain"`
	Peak                 float64     `json:"peak"`
	AllowStreaming       bool        `json:"allowStreaming"`
	StreamReady          bool        `json:"streamReady"`
	StreamStartDate      string      `json:"streamStartDate"`
	PremiumStreamingOnly bool        `json:"premiumStreamingOnly"`
	TrackNumber          int         `json:"trackNumber"`
	VolumeNumber         int         `json:"volumeNumber"`
	Version              interface{} `json:"version"`
	Popularity           int         `json:"popularity"`
	Copyright            string      `json:"copyright"`
	URL                  string      `json:"url"`
	Isrc                 string      `json:"isrc"`
	Editable             bool        `json:"editable"`
	Explicit             bool        `json:"explicit"`
	AudioQuality         string      `json:"audioQuality"`
	AudioModes           []string    `json:"audioModes"`
	Artist               struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Picture string `json:"picture"`
	} `json:"artist"`
	Artists []struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Picture string `json:"picture"`
	} `json:"artists"`
	Album struct {
		ID           int         `json:"id"`
		Title        string      `json:"title"`
		Cover        string      `json:"cover"`
		VibrantColor string      `json:"vibrantColor"`
		VideoCover   interface{} `json:"videoCover"`
	} `json:"album"`
	Mixes struct {
		TrackMix string `json:"TRACK_MIX"`
	} `json:"mixes"`
}

// JwtClaims represents the claims of a JWT.
type JwtClaims struct {
	jwt.RegisteredClaims
	Type  string `json:"type"`
	UID   int    `json:"uid"`
	Scope string `json:"scope"`
	GVer  int    `json:"gVer"`
	SVer  int    `json:"sVer"`
	Cid   int    `json:"cid"`
	Cuk   string `json:"cuk"`
	Exp   int    `json:"exp"`
	Sid   string `json:"sid"`
	Iss   string `json:"iss"`
}

// RefreshTokenResponse represents the response of a refresh token request.
type RefreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	User        struct {
		UserID      int         `json:"userId"`
		Email       interface{} `json:"email"`
		CountryCode string      `json:"countryCode"`
		// TODO: keep an eye on this and see if "omitempty" works when the value is null
		FullName     string      `json:"fullName,omitempty"`
		FirstName    string      `json:"firstName,omitempty"`
		LastName     interface{} `json:"lastName"`
		Nickname     interface{} `json:"nickname"`
		Username     string      `json:"username"`
		Address      interface{} `json:"address"`
		City         interface{} `json:"city"`
		Postalcode   interface{} `json:"postalcode"`
		UsState      interface{} `json:"usState"`
		PhoneNumber  interface{} `json:"phoneNumber"`
		Birthday     interface{} `json:"birthday"`
		Gender       interface{} `json:"gender"`
		ImageID      interface{} `json:"imageId"`
		ChannelID    int         `json:"channelId"`
		ParentID     int         `json:"parentId"`
		AcceptedEULA bool        `json:"acceptedEULA"`
		Created      int64       `json:"created"`
		Updated      int64       `json:"updated"`
		FacebookUID  int         `json:"facebookUid"`
		AppleUID     interface{} `json:"appleUid"`
		GoogleUID    interface{} `json:"googleUid"`
		NewUser      bool        `json:"newUser"`
	} `json:"user"`
}

type SearchResult struct {
	Artists struct {
		Limit              int `json:"limit"`
		Offset             int `json:"offset"`
		TotalNumberOfItems int `json:"totalNumberOfItems"`
		Items              []struct {
			Id          int      `json:"id"`
			Name        string   `json:"name"`
			ArtistTypes []string `json:"artistTypes"`
			Url         string   `json:"url"`
			Picture     *string  `json:"picture"`
			Popularity  int      `json:"popularity"`
			ArtistRoles []struct {
				CategoryId int    `json:"categoryId"`
				Category   string `json:"category"`
			} `json:"artistRoles"`
			Mixes struct {
				MASTERARTISTMIX string `json:"MASTER_ARTIST_MIX,omitempty"`
				ARTISTMIX       string `json:"ARTIST_MIX"`
			} `json:"mixes"`
		} `json:"items"`
	} `json:"artists"`
	Albums struct {
		Limit              int `json:"limit"`
		Offset             int `json:"offset"`
		TotalNumberOfItems int `json:"totalNumberOfItems"`
		Items              []struct {
			Id                   int         `json:"id"`
			Title                string      `json:"title"`
			Duration             int         `json:"duration"`
			StreamReady          bool        `json:"streamReady"`
			StreamStartDate      string      `json:"streamStartDate"`
			AllowStreaming       bool        `json:"allowStreaming"`
			PremiumStreamingOnly bool        `json:"premiumStreamingOnly"`
			NumberOfTracks       int         `json:"numberOfTracks"`
			NumberOfVideos       int         `json:"numberOfVideos"`
			NumberOfVolumes      int         `json:"numberOfVolumes"`
			ReleaseDate          string      `json:"releaseDate"`
			Copyright            string      `json:"copyright"`
			Type                 string      `json:"type"`
			Version              interface{} `json:"version"`
			Url                  string      `json:"url"`
			Cover                string      `json:"cover"`
			VibrantColor         string      `json:"vibrantColor"`
			VideoCover           interface{} `json:"videoCover"`
			Explicit             bool        `json:"explicit"`
			Upc                  string      `json:"upc"`
			Popularity           int         `json:"popularity"`
			AudioQuality         string      `json:"audioQuality"`
			AudioModes           []string    `json:"audioModes"`
			Artists              []struct {
				Id      int    `json:"id"`
				Name    string `json:"name"`
				Type    string `json:"type"`
				Picture string `json:"picture"`
			} `json:"artists"`
		} `json:"items"`
	} `json:"albums"`
	Playlists struct {
		Limit              int `json:"limit"`
		Offset             int `json:"offset"`
		TotalNumberOfItems int `json:"totalNumberOfItems"`
		Items              []struct {
			Uuid           string `json:"uuid"`
			Title          string `json:"title"`
			NumberOfTracks int    `json:"numberOfTracks"`
			NumberOfVideos int    `json:"numberOfVideos"`
			Creator        struct {
			} `json:"creator"`
			Description     string `json:"description"`
			Duration        int    `json:"duration"`
			LastUpdated     string `json:"lastUpdated"`
			Created         string `json:"created"`
			Type            string `json:"type"`
			PublicPlaylist  bool   `json:"publicPlaylist"`
			Url             string `json:"url"`
			Image           string `json:"image"`
			Popularity      int    `json:"popularity"`
			SquareImage     string `json:"squareImage"`
			PromotedArtists []struct {
				Id      int         `json:"id"`
				Name    string      `json:"name"`
				Type    string      `json:"type"`
				Picture interface{} `json:"picture"`
			} `json:"promotedArtists"`
			LastItemAddedAt string `json:"lastItemAddedAt"`
		} `json:"items"`
	} `json:"playlists"`
	Tracks struct {
		Limit              int `json:"limit"`
		Offset             int `json:"offset"`
		TotalNumberOfItems int `json:"totalNumberOfItems"`
		Items              []struct {
			Id                   int         `json:"id"`
			Title                string      `json:"title"`
			Duration             int         `json:"duration"`
			ReplayGain           float64     `json:"replayGain"`
			Peak                 float64     `json:"peak"`
			AllowStreaming       bool        `json:"allowStreaming"`
			StreamReady          bool        `json:"streamReady"`
			StreamStartDate      string      `json:"streamStartDate"`
			PremiumStreamingOnly bool        `json:"premiumStreamingOnly"`
			TrackNumber          int         `json:"trackNumber"`
			VolumeNumber         int         `json:"volumeNumber"`
			Version              interface{} `json:"version"`
			Popularity           int         `json:"popularity"`
			Copyright            string      `json:"copyright"`
			Url                  string      `json:"url"`
			Isrc                 string      `json:"isrc"`
			Editable             bool        `json:"editable"`
			Explicit             bool        `json:"explicit"`
			AudioQuality         string      `json:"audioQuality"`
			AudioModes           []string    `json:"audioModes"`
			Artists              []struct {
				Id      int     `json:"id"`
				Name    string  `json:"name"`
				Type    string  `json:"type"`
				Picture *string `json:"picture"`
			} `json:"artists"`
			Album struct {
				Id           int         `json:"id"`
				Title        string      `json:"title"`
				Cover        string      `json:"cover"`
				VibrantColor string      `json:"vibrantColor"`
				VideoCover   interface{} `json:"videoCover"`
				ReleaseDate  string      `json:"releaseDate"`
			} `json:"album"`
			Mixes struct {
				MASTERTRACKMIX string `json:"MASTER_TRACK_MIX,omitempty"`
				TRACKMIX       string `json:"TRACK_MIX,omitempty"`
			} `json:"mixes"`
		} `json:"items"`
	} `json:"tracks"`
	Videos struct {
		Limit              int `json:"limit"`
		Offset             int `json:"offset"`
		TotalNumberOfItems int `json:"totalNumberOfItems"`
		Items              []struct {
			Id                int         `json:"id"`
			Title             string      `json:"title"`
			VolumeNumber      int         `json:"volumeNumber"`
			TrackNumber       int         `json:"trackNumber"`
			ReleaseDate       string      `json:"releaseDate"`
			ImagePath         interface{} `json:"imagePath"`
			ImageId           string      `json:"imageId"`
			VibrantColor      string      `json:"vibrantColor"`
			Duration          int         `json:"duration"`
			Quality           string      `json:"quality"`
			StreamReady       bool        `json:"streamReady"`
			StreamStartDate   string      `json:"streamStartDate"`
			AllowStreaming    bool        `json:"allowStreaming"`
			Explicit          bool        `json:"explicit"`
			Popularity        int         `json:"popularity"`
			Type              string      `json:"type"`
			AdsUrl            interface{} `json:"adsUrl"`
			AdsPrePaywallOnly bool        `json:"adsPrePaywallOnly"`
			Artists           []struct {
				Id      int    `json:"id"`
				Name    string `json:"name"`
				Type    string `json:"type"`
				Picture string `json:"picture"`
			} `json:"artists"`
			Album interface{} `json:"album"`
		} `json:"items"`
	} `json:"videos"`
	Genres  []interface{} `json:"genres"`
	TopHits []struct {
		Value struct {
			Id                   int         `json:"id,omitempty"`
			Title                string      `json:"title,omitempty"`
			Duration             int         `json:"duration,omitempty"`
			ReplayGain           float64     `json:"replayGain,omitempty"`
			Peak                 float64     `json:"peak,omitempty"`
			AllowStreaming       bool        `json:"allowStreaming,omitempty"`
			StreamReady          bool        `json:"streamReady,omitempty"`
			StreamStartDate      string      `json:"streamStartDate,omitempty"`
			PremiumStreamingOnly bool        `json:"premiumStreamingOnly,omitempty"`
			TrackNumber          int         `json:"trackNumber,omitempty"`
			VolumeNumber         int         `json:"volumeNumber,omitempty"`
			Version              interface{} `json:"version"`
			Popularity           int         `json:"popularity"`
			Copyright            string      `json:"copyright,omitempty"`
			Url                  string      `json:"url,omitempty"`
			Isrc                 string      `json:"isrc,omitempty"`
			Editable             bool        `json:"editable,omitempty"`
			Explicit             bool        `json:"explicit,omitempty"`
			AudioQuality         string      `json:"audioQuality,omitempty"`
			AudioModes           []string    `json:"audioModes,omitempty"`
			Artists              []struct {
				Id      int     `json:"id"`
				Name    string  `json:"name"`
				Type    string  `json:"type"`
				Picture *string `json:"picture"`
			} `json:"artists,omitempty"`
			Album *struct {
				Id           int         `json:"id"`
				Title        string      `json:"title"`
				Cover        string      `json:"cover"`
				VibrantColor string      `json:"vibrantColor"`
				VideoCover   interface{} `json:"videoCover"`
				ReleaseDate  string      `json:"releaseDate"`
			} `json:"album,omitempty"`
			Mixes struct {
				MASTERTRACKMIX  string `json:"MASTER_TRACK_MIX,omitempty"`
				TRACKMIX        string `json:"TRACK_MIX,omitempty"`
				MASTERARTISTMIX string `json:"MASTER_ARTIST_MIX,omitempty"`
				ARTISTMIX       string `json:"ARTIST_MIX,omitempty"`
			} `json:"mixes,omitempty"`
			Name        string   `json:"name,omitempty"`
			ArtistTypes []string `json:"artistTypes,omitempty"`
			Picture     *string  `json:"picture,omitempty"`
			ArtistRoles []struct {
				CategoryId int    `json:"categoryId"`
				Category   string `json:"category"`
			} `json:"artistRoles,omitempty"`
			NumberOfTracks  int         `json:"numberOfTracks,omitempty"`
			NumberOfVideos  int         `json:"numberOfVideos,omitempty"`
			NumberOfVolumes int         `json:"numberOfVolumes,omitempty"`
			ReleaseDate     string      `json:"releaseDate,omitempty"`
			Type            string      `json:"type,omitempty"`
			Cover           string      `json:"cover,omitempty"`
			VibrantColor    string      `json:"vibrantColor,omitempty"`
			VideoCover      interface{} `json:"videoCover"`
			Upc             string      `json:"upc,omitempty"`
			Uuid            string      `json:"uuid,omitempty"`
			Creator         struct {
			} `json:"creator,omitempty"`
			Description     string `json:"description,omitempty"`
			LastUpdated     string `json:"lastUpdated,omitempty"`
			Created         string `json:"created,omitempty"`
			PublicPlaylist  bool   `json:"publicPlaylist,omitempty"`
			Image           string `json:"image,omitempty"`
			SquareImage     string `json:"squareImage,omitempty"`
			PromotedArtists []struct {
				Id      int         `json:"id"`
				Name    string      `json:"name"`
				Type    string      `json:"type"`
				Picture interface{} `json:"picture"`
			} `json:"promotedArtists,omitempty"`
			LastItemAddedAt   string      `json:"lastItemAddedAt,omitempty"`
			ImagePath         interface{} `json:"imagePath"`
			ImageId           string      `json:"imageId,omitempty"`
			Quality           string      `json:"quality,omitempty"`
			AdsUrl            interface{} `json:"adsUrl"`
			AdsPrePaywallOnly bool        `json:"adsPrePaywallOnly,omitempty"`
		} `json:"value"`
		Type string `json:"type"`
	} `json:"topHits"`
}
