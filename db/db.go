package db

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"log"
	"zoove/db/queries"
)

type NewDB struct {
	DB *sql.DB
}

type SingleUserByEmail struct {
	Email     string `json:"email"`
	Usernames Map `json:"usernames"`
	Tokens    Map `json:"tokens"`
	IDs       Map `json:"ids"`
}

type Map map[string]interface{}


func(s *Map) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to byte failed")
	}
	log.Printf("single %v", string(b))
	return json.Unmarshal(b, &s)
}

func (s Map) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// FindUserByEmail finds a user by their email
func (d *NewDB) FindUserByEmail(email string) (interface{}, error) {
	result := d.DB.QueryRow(queries.FindUserByEmail, email)
	user := SingleUserByEmail{}


	err := result.Scan(&user.Email, &user.Usernames, &user.IDs)
	if err != nil {
		log.Printf("Error scanning here %v", err)
	}
	return user, nil
}
