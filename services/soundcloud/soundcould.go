package soundcloud

const AuthBase = "https://api.soundcloud.com/oauth2"

type Soundcloud struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

func (s *Soundcloud) FetchAuthURL() string {

	return ""
}

//func (s *Soundcloud) SearchTrackWithID(id string) {
//	var soundcloudAccessToken = os.Getenv("SOUNDCLOUD_ACCESS_TOKEN")
//
//}
//
//func fetchNewAccessToken() (string, error) {
//	refreshInstance := axios.NewInstance(&axios.InstanceConfig{
//		BaseURL: AuthBase,
//		Headers: map[string][]string{
//			"Content-Type": {"application/x-www-form-urlencoded"},
//		},
//	})
//	grantType := "refresh_token"
//	params := url.Values{}
//	params.Add("client_id", os.Getenv("SOUNDCLOUD_CLIENT_ID"))
//	params.Add("client_secret", os.Getenv("SOUNDCLOUD_CLIENT_SECRET"))
//	params.Add("refresh_token", os.Getenv("SOUNDCLOUD_REFRESH_TOKEN"))
//	return "", nil
//}
