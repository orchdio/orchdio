package db

import (
	"database/sql"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"log"
	"orchdio/db/queries"
)

type NewDB struct {
	DB *sqlx.DB
}

type User struct {
	Email    string    `json:"email"`
	Username string    `json:"username"`
	ID       int       `json:"id"`
	UUID     uuid.UUID `json:"uuid"`
}

type ApiKey struct {
	ID      int       `json:"id"`
	Key     uuid.UUID `json:"key"`
	User    uuid.UUID `json:"user"`
	Revoked bool      `json:"revoked"`
}

// FindUserByEmail finds a user by their email
func (d *NewDB) FindUserByEmail(email string) (*User, error) {
	result := d.DB.QueryRowx(queries.FindUserByEmail, email)
	user := &User{}

	err := result.StructScan(user)
	if err != nil {
		log.Printf("[controller][db] error scanning row result. %v\n", err)
		return nil, err
	}
	return user, nil
}

// FetchUserApikey fetches the user api key
func (d *NewDB) FetchUserApikey(uid uuid.UUID) (*ApiKey, error) {
	result := d.DB.QueryRowx(queries.FetchUserApiKey, uid)
	apiKey := &ApiKey{}

	err := result.StructScan(apiKey)

	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[controller][user][FetchUserApiKey] error - error scanning row. Something went wrong and this is not an expected error. %v\n", err)
			return nil, err
		}
		return nil, nil
	}
	return apiKey, nil
}

// RevokeApiKey sets the revoked column to true
func (d *NewDB) RevokeApiKey(key, user string) error {
	log.Printf("[db][RevokeApiKey] Running query %s %s %s\n", queries.RevokeApiKey, key, user)
	_, err := d.DB.Exec(queries.RevokeApiKey, key, user)
	if err != nil {
		log.Printf("[db][RevokeApiKey] error executing query %s.\n %v\n %s, %s \n", queries.RevokeApiKey, err, key, user)
		return err
	}
	log.Printf("[db][RevokeApiKey] Ran query %s\n", queries.RevokeApiKey)
	return nil
}

// UnRevokeApiKey sets the revoked column to true
func (d *NewDB) UnRevokeApiKey(key, user string) error {
	log.Printf("[db][UnRevokeApiKey] Running query %s %s %s\n", queries.UnRevokeApiKey, key, user)
	_, err := d.DB.Exec(queries.UnRevokeApiKey, key, user)
	if err != nil {
		log.Printf("[db][UnRevokeApiKey] error executing query %s.\n %v\n %s, %s\n", queries.RevokeApiKey, err, key, user)
		return err
	}
	log.Printf("[db][UnRevokeApiKey] Ran query %s\n", queries.UnRevokeApiKey)
	return nil
}

// DeleteApiKey deletes a user's api key
func (d *NewDB) DeleteApiKey(key, user string) ([]byte, error) {
	log.Printf("[db][DeleteKey] Ran Query: %s\n", queries.DeleteApiKey)
	result, err := d.DB.Queryx(queries.DeleteApiKey, key, user)
	if err != nil {
		log.Printf("[db][DeleteApikey] could not delete key. %v\n", err)
		return nil, err
	}

	deleteRes := struct {
		Key string
	}{}

	scanErr := result.StructScan(&deleteRes)
	if scanErr != nil {
		log.Printf("[db][DeleteApiKey] - could not scan query result %v\n", scanErr)
		return nil, err
	}

	log.Printf("[db][DeleteApiKey] - Deleted apiKey")
	return nil, nil
}
