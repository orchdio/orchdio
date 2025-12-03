package tidal_v2

import (
	"context"
	"log"
)

type User struct {
	Attributes UserAttributes `json:"attributes"`
	ID         string         `json:"id"`
	Type       string         `json:"type"`
}

type UserAttributes struct {
	Country        string `json:"country"`
	Email          string `json:"email"`
	EmailVerified  bool   `json:"emailVerified"`
	FirstName      string `json:"firstName"`
	LastName       string `json:"lastName"`
	NostrPublicKey string `json:"nostrPublicKey"`
	Username       string `json:"username"`
}

type UserIncluded struct {
	Attributes struct {
		AccessType   string   `json:"accessType"`
		Availability []string `json:"availability"`
		BarcodeID    string   `json:"barcodeId"`
		Copyright    struct {
			Text string `json:"text"`
		} `json:"copyright"`
		Duration      string `json:"duration"`
		Explicit      bool   `json:"explicit"`
		ExternalLinks []struct {
			Href string `json:"href"`
			Meta struct {
				Type string `json:"type"`
			} `json:"meta"`
		} `json:"externalLinks"`
		MediaTags       []string `json:"mediaTags"`
		NumberOfItems   int      `json:"numberOfItems"`
		NumberOfVolumes int      `json:"numberOfVolumes"`
		Popularity      float64  `json:"popularity"`
		ReleaseDate     string   `json:"releaseDate"`
		Title           string   `json:"title"`
		Type            string   `json:"type"`
		Version         string   `json:"version"`
	} `json:"included"`
}

func (tc *TidalClient) CurrentUser(ctx context.Context) (*User, error) {
	userURL := tc.baseURL + "/users/me"
	currUser := SuccessResponse[User, UserIncluded, Links]{}
	err := tc.get(ctx, userURL, &currUser)
	if err != nil {
		log.Println("ERROR: could not fetch current user")
		return nil, err
	}

	return &currUser.Data, nil
}
