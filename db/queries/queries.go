package queries

const CreateUserQuery = `WITH user_rec as ( INSERT INTO "users"(email, username, uuid, created_at, updated_at) VALUES($1, $2, $3, now(), now()) ON CONFLICT("email") DO UPDATE
SET email=EXCLUDED.email, username=$2 RETURNING email, uuid)
			SELECT * from user_rec;`

const FindUserByEmail = `SELECT * FROM users where email = $1`

const FetchUserApiKey = `SELECT api.*
FROM apikeys api
JOIN users u ON u.uuid = api.user
WHERE api.user = $1;`

const CreateNewKey = `INSERT INTO apiKeys(key, "user", revoked, created_at, updated_at) values ($1, $2, true, now(), now());`

const RevokeApiKey = `UPDATE apiKeys SET revoked = TRUE, updated_at = now() FROM users AS u WHERE u.uuid = $2 AND KEY = $1;`
const UnRevokeApiKey = `UPDATE apiKeys SET revoked = FALSE, updated_at = now() FROM users AS u WHERE u.uuid = $2 AND KEY = $1;`
const DeleteApiKey = `DELETE FROM apiKeys api USING users u WHERE u.uuid = api.user AND api.key = $1 AND api.user = $2 RETURNING key;`

const FetchUserWebhook = `SELECT wh.url FROM webhooks wh join users u ON u.uuid = wh.user where wh.user = $1;`
const CreateWebhook = `INSERT INTO webhooks(url, "user", created_at, updated_at) values ($1, $2, now(), now());`

const FetchUserWithWebhook = `SELECT u.* FROM webhooks wh join users u ON u.uuid = wh.user where wh.url = $1;`
const FetchUserWithApiKey = `SELECT u.* FROM apiKeys api join users u ON u.uuid = api.user where api.key = $1 and revoked = false;`
const UpdateUserWebhook = `UPDATE webhooks SET url = $1, updated_at = now() FROM users AS u WHERE u.uuid = $2;`
const DeleteUserWebhook = `DELETE FROM webhooks wh WHERE wh.user = $1;`

const CreateOrUpdateTask = `INSERT INTO tasks(uuid, "user", entity_id, created_at, updated_at) values ($1, $2, $3, now(), now()) ON CONFLICT("uuid") 
DO UPDATE SET status = 'pending', updated_at = now() RETURNING uuid;`

const UpdateTaskStatus = `UPDATE tasks SET status = $2, updated_at = now() WHERE uuid = $1 RETURNING uuid;`
const UpdateTask = `UPDATE tasks SET result = $2, updated_at = now() WHERE uuid = $1 RETURNING result;`

const FetchTask = `SELECT id, uuid, entity_id, created_at, updated_at, "user", status, coalesce(result, '{}') result FROM tasks WHERE uuid = $1;`
const DeleteTask = `DELETE FROM tasks WHERE uuid = $1;`
