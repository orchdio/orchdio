package queries

const CreateNewApp = `INSERT INTO apps (uuid, name, description, redirect_url, webhook_url, created_at, updated_at) 
	VALUES ($1, $2, $3, $4, $5, now(), now()) RETURNING uuid`

const FetchAppByAppID = `SELECT * FROM apps WHERE uuid = $1`
