package queries

const CreateUserQuery = `WITH user_rec as ( INSERT INTO "users"(email, username, uuid) VALUES($1, $2, $3) ON CONFLICT("email") DO UPDATE
SET email=EXCLUDED.email, username=$2 RETURNING email, uuid)
			SELECT * from user_rec;`

const FindUserByEmail = `SELECT * FROM users where email = $1`

const FetchUserApiKey = `SELECT api.*
FROM apikeys api
JOIN users u ON u.uuid = api.user
WHERE api.user = $1`

const CreateNewKey = `INSERT INTO apiKeys(key, "user", revoked) values ($1, $2, true);`

const RevokeApiKey = `UPDATE apiKeys
							SET revoked = TRUE
							FROM users AS u
							WHERE u.uuid = $2
							  AND KEY = $1;`
const UnRevokeApiKey = `UPDATE apiKeys
							SET revoked = FALSE
							FROM users AS u
							WHERE u.uuid = $2
  AND KEY = $1;`
