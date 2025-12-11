package tidal_v2

import (
	"context"
	"log"
)

// UserData represents the main user data object
type UserData struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Attributes UserAttributes `json:"attributes"`
}

// UserAttributes contains the user metadata
type UserAttributes struct {
	Username      string `json:"username"`
	Country       string `json:"country"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	FirstName     string `json:"firstName"`
	LastName      string `json:"lastName"`
}

// UserLinks represents the links object in the user response
type UserLinks struct {
	Self string `json:"self"`
}

// UserResponse is the complete API response type for GET /users/me
type UserProfile = SuccessResponse[UserData, any, UserLinks]

func (tc *TidalClient) CurrentUser(ctx context.Context) (*UserProfile, error) {
	userURL := tc.baseURL + "/users/me"
	var currUser UserProfile
	err := tc.get(ctx, userURL, &currUser)
	if err != nil {
		log.Println("ERROR: could not fetch current user")
		return nil, err
	}
	return &currUser, nil
}
