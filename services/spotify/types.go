package spotify

import "golang.org/x/oauth2"

type Config struct {
	config *oauth2.Config
}

const IDENTIFIER = "spotify"