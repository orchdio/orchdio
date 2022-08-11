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
const CreateWebhook = `INSERT INTO webhooks(url, "user", verify_token, created_at, updated_at, uuid) values ($1, $2, $3, now(), now(), $4);`

const FetchUserWithWebhook = `SELECT u.* FROM webhooks wh join users u ON u.uuid = wh.user where wh.url = $1;`
const FetchUserWithApiKey = `SELECT u.* FROM apiKeys api join users u ON u.uuid = api.user where api.key = $1 and revoked = false;`
const UpdateUserWebhook = `UPDATE webhooks SET url = $1, verify_token = $3, updated_at = now() FROM users AS u WHERE u.uuid = $2;`
const DeleteUserWebhook = `DELETE FROM webhooks wh WHERE wh.user = $1;`

const CreateOrUpdateTask = `INSERT INTO tasks(uuid, "user", entity_id, created_at, updated_at, type) values ($1, $2, $3, now(), now(), 'conversion') ON CONFLICT("uuid") 
DO UPDATE SET status = 'pending', updated_at = now() RETURNING uuid;`

const UpdateTaskStatus = `UPDATE tasks SET status = $2, updated_at = now() WHERE uuid = $1 RETURNING uuid;`
const UpdateTask = `UPDATE tasks SET result = $2, updated_at = now() WHERE uuid = $1 RETURNING result;`

const FetchTask = `SELECT id, uuid, entity_id, created_at, updated_at, "user", status, coalesce(result, '{}') result FROM tasks WHERE uuid = $1;`
const DeleteTask = `DELETE FROM tasks WHERE uuid = $1;`

const CreateOrAddSubscriberFollow = `INSERT INTO follows(uuid, developer, entity_id, subscribers, created_at, updated_at) values ($1, $2, $3, $4 , now(), now())
ON CONFLICT("entity_id") DO UPDATE SET updated_at = NOW() RETURNING uuid;`
const UpdateFollowSubscriber = `UPDATE follows SET subscribers = ARRAY [$1], updated_at = now() WHERE entity_id = $2 AND $1::text <> ANY (subscribers::text[]) RETURNING uuid;`
const FetchFollowedTask = `SELECT * FROM  follows where entity_id = $1;`

const FetchTaskByEntityIdAndType = `SELECT * FROM tasks WHERE entity_id = $1 and type = $2;`

const FetchPlaylistFollowsToProcess = `SELECT DISTINCT on(follow.id) follow.id, follow.created_at, follow.updated_at, follow.developer, follow.entity_id, follow.entity_url, json_agg("user".*) subscribers FROM follows follow JOIN users "user" ON "user"::text <> ANY (subscribers::text[]) WHERE entity_id IS NOT NULL AND entity_url IS NOT NULL group by follow.id
  -- AND follow.updated_at > CURRENT_DATE - interval '10 minutes'
`

// FetchFollowByEntityId query is used to fetch a follow and the subscribers to it.
const FetchFollowByEntityId = `SELECT DISTINCT on(follow.id) follow.id, follow.created_at, follow.updated_at, follow.developer, follow.entity_id, follow.entity_url, json_agg("user".*) subscribers FROM follows follow JOIN users "user" ON "user".uuid::text = ANY (subscribers::text[]) WHERE entity_id = $1 GROUP BY follow.id`
const CreateFollowNotification = `INSERT INTO notifications(created_at, updated_at, "user", UUID, status, "data") VALUES (now(), now(), :subscriber, :notification_id, 'unread', :data)`

//const FetchPlaylistFollowsToProcess = `SELECT task.*, COALESCE(follow.entity_url, '') entity_url FROM follows follow JOIN tasks task ON task.uuid = follow.task WHERE task IS NOT NULL
//--  	AND task.updated_at > CURRENT_DATE - interval '10 minutes'
