package queries

const CreateNewApp = `INSERT INTO apps (uuid, name, description, redirect_url, webhook_url, created_at, updated_at, public_key, developer, secret_key, verify_token) 
	VALUES ($1, $2, $3, $4, $5, now(), now(), $6, $7, $8, $9) RETURNING uuid`

const FetchAppByAppID = `SELECT * FROM apps WHERE uuid = $1`
const FetchAppByPubKey = `SELECT * FROM apps WHERE public_key = $1`
const FetchAppBySecretKey = `SELECT * FROM apps WHERE secret_key = $1`

const FetchAuthorizedAppDeveloperByPublicKey = `SELECT u.email, u.usernames, u.username, u.id, u.uuid, u.created_at, u.updated_at, u.refresh_token, u.platform_id FROM apps a JOIN users u on a.developer = u.uuid WHERE a.public_key = $1 AND a.authorized = true`
const FetchAuthorizedAppDeveloperBySecretKey = `SELECT u.email, u.usernames, u.username, u.id, u.uuid, u.created_at, u.updated_at, u.refresh_token, u.platform_id FROM apps a JOIN users u on a.developer = u.uuid WHERE a.secret_key = $1 AND a.authorized = true`

// UpdateApp updates the developer app with data passed. If the values are empty, it falls back to what the original value of the column is
const UpdateApp = `UPDATE apps SET  description = (CASE WHEN $1 = '' THEN description ELSE $1 END),
                 name = (CASE WHEN $2 = '' THEN name ELSE $2 END),
redirect_url = (CASE WHEN $3 = '' THEN redirect_url ELSE $3 END),
webhook_url = (CASE WHEN $4 = '' THEN webhook_url ELSE $4 END), 
updated_at = now() WHERE uuid = $5`

const DeleteApp = `DELETE FROM apps WHERE uuid = $1`

// App queries

const DisableApp = `UPDATE apps SET authorized = false WHERE uuid = $1;`
const EnableApp = `UPDATE apps SET authorized = true WHERE uuid = $1;`
const FetchAppKeysByID = `SELECT COALESCE(public_key, uuid_nil()), COALESCE(secret_key, uuid_nil()), COALESCE(verify_token, uuid_nil()) FROM apps WHERE uuid = $1;`

const FetchAppsByDeveloper = `SELECT * FROM apps WHERE developer = $1`
const UpdateAppKeys = `UPDATE apps SET public_key = $1, secret_key = $2, verify_token = $3 WHERE uuid = $4`
