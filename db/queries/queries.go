package queries

const CreateUserQuery = `WITH user_rec as ( INSERT INTO "users"(email, uuid, created_at, updated_at) VALUES($1, $2, now(), now())
ON CONFLICT("email") DO UPDATE
SET email=EXCLUDED.email,updated_at=now() RETURNING email, uuid)
			SELECT * from user_rec;`

const CreateNewOrgUser = `INSERT INTO "users" (
                     email, uuid, password, created_at, updated_at
) VALUES ($1, $2, $3, now(), now()) ON CONFLICT DO NOTHING RETURNING id`
const UpdateUserPassword = `UPDATE "users" SET password = $1 WHERE uuid = $2`
const FetchUserEmailAndPassword = `SELECT id, email, password, uuid FROM users WHERE email = $1`

//const UpdateUserPlatformToken = `UPDATE "users" SET spotify_token =
//    (case when $1 = 'spotify' then spotify_token = $2 end),
//                   applemusic_token = (case when $1 = 'apple' then applemusic_token = $2 end),
//                   deezer_token = (case when $1 = 'deezer' then deezer_token = $2 end),
//                   tidal_token = (case when $1 = 'tidal' then tidal_token = $2 end) WHERE uuid = $3`

// FindUserByEmail returns a  user the email. it fetches the refresh token for the user based on platform passed. if no platform passed, it'll return refresh_token
//const FindUserByEmail = `SELECT id, email, coalesce(username, '') AS username,
//       uuid, created_at, updated_at, usernames, platform_id,
//       (case when $2 ILIKE '%spotify%' then spotify_token when $2 ILIKE '%deezer%'
//           then deezer_token when $2 ILIKE '%applemusic%' then applemusic_token else
//               refresh_token end) AS refresh_token  FROM users where email = $1`

const FindUserByEmail = `SELECT id, email, uuid FROM users where email = $1`

//const FindUserByUUID = `SELECT id, email, coalesce(username, '') AS username, uuid, created_at, updated_at, usernames, (case when $2 ILIKE '%spotify%' then spotify_token when $2 ILIKE '%deezer%' then deezer_token when $2 ILIKE '%applemusic%' then applemusic_token end) AS refresh_token, platform_ids  FROM users where uuid = $1 AND platform_id IS NOT NULL`

const FindUserByUUID = `SELECT id, email, uuid FROM users where uuid = $1`

// FindUserProfileByEmail is similar to FindUserByEmail with the fact that they both fetch profile info for a user except this one fetches just the user profile we want to return
// without including the refreshtoken and other fields. the above is currently used in the code and it has its own usecases. They are similar, but it seems there are more fields needed
// above for the places its used and its a lot of work amending to use that only. Maybe if it gets messier, it'll be refactored into 1.
const FindUserProfileByEmail = `SELECT email, usernames, uuid, created_at, updated_at FROM users WHERE email = $1`

const FetchUserApiKey = `SELECT api.* FROM apikeys api JOIN users u ON u.uuid = api.user WHERE u.email = $1;`

const CreateNewKey = `INSERT INTO apiKeys(key, "user", revoked, created_at, updated_at) values ($1, $2, false, now(), now());`

const RevokeApiKey = `UPDATE apiKeys SET revoked = TRUE, updated_at = now() FROM users AS u WHERE key = $1;`
const UnRevokeApiKey = `UPDATE apiKeys SET revoked = FALSE, updated_at = now() FROM users AS u WHERE key = $1;`
const DeleteApiKey = `DELETE FROM apiKeys api USING users u WHERE u.uuid = api.user AND api.key = $1 RETURNING key;`

const FetchUserWebhook = `SELECT wh.url FROM webhooks wh join users u ON u.uuid = wh.user where wh.user = $1;`
const CreateWebhook = `INSERT INTO webhooks(url, "user", verify_token, created_at, updated_at, uuid) values ($1, $2, $3, now(), now(), $4);`

const FetchUserWithWebhook = `SELECT u.* FROM webhooks wh join users u ON u.uuid = wh.user where wh.url = $1;`
const FetchUserWithApiKey = `SELECT u.id, u.email, coalesce(u.username, '') as username, u.uuid, u.created_at, u.updated_at, u.usernames FROM apiKeys api join users u ON u.uuid = api.user where api.key = $1 and revoked = false;`
const UpdateUserWebhook = `UPDATE webhooks SET url = $1, verify_token = $3, updated_at = now() FROM users AS u WHERE "user" = $2 RETURNING uuid;`
const DeleteUserWebhook = `DELETE FROM webhooks wh WHERE wh.user = $1;`

const CreateOrUpdateTask = `INSERT INTO tasks(uuid, shortid, app, entity_id, created_at, updated_at, type) values ($1, $2, $3, $4, now(), now(), 'conversion') ON CONFLICT("uuid") 
DO UPDATE SET status = 'pending', updated_at = now() RETURNING uuid;`

const UpdateTaskStatus = `UPDATE tasks SET status = $2, updated_at = now() WHERE uuid = $1 RETURNING uuid;`
const UpdateTaskResult = `UPDATE tasks SET result = $2, updated_at = now() WHERE uuid = $1 RETURNING result;`

const FetchTask = `SELECT id, uuid, entity_id, created_at, updated_at, app, status, coalesce(result, '{}') as result FROM tasks WHERE uuid= $1;`
const FetchTaskByShortID = `SELECT id, uuid, entity_id, created_at, updated_at, app, status, coalesce(result, '{}') as result FROM tasks WHERE shortid = $1;`
const DeleteTask = `DELETE FROM tasks WHERE uuid = $1;`

const CreateOrAddSubscriberFollow = `INSERT INTO follows(uuid, developer, entity_id, subscribers, entity_url, created_at, updated_at, app) values ($1, $2, $3, $4, $5, now(), now(), $6)
ON CONFLICT("entity_id") DO UPDATE SET updated_at = NOW() RETURNING uuid;`

const CreateNewTrackTaskRecord = `INSERT INTO tasks(uuid, shortid, entity_id, result, status, type, created_at, updated_at, app) values ($1, $2, $3, $4, 'completed', 'track', now(), now(), $5) RETURNING uuid;`
const UpdateFollowSubscriber = `UPDATE follows SET subscribers = ARRAY [$1], updated_at = now() WHERE entity_id = $2 AND $1::text <> ANY (subscribers::text[]) RETURNING uuid;`
const FetchFollowedTask = `SELECT * FROM  follows where entity_id = $1;`

const FetchTaskByEntityIdAndType = `SELECT * FROM tasks WHERE entity_id = $1 and type = $2;`

const FetchPlaylistFollowsToProcess = `SELECT DISTINCT on(follow.id) follow.id, 
follow.created_at, follow.updated_at, follow.developer, 
follow.entity_id, follow.entity_url, json_agg("user".*), follow.app
as subscribers FROM follows follow JOIN users "user" 
    ON "user"::text <> ANY (subscribers::text[]) WHERE 
		entity_id IS NOT NULL AND entity_url IS NOT NULL 
			AND (status <> 'ERROR' OR follow.updated_at > CURRENT_DATE - interval '10 minutes') 
			AND entity_url IS NOT NULL GROUP BY follow.id;
`

// FetchFollowByEntityId query is used to fetch a follow and the subscribers to it.
const FetchFollowByEntityId = `SELECT DISTINCT on(follow.id) follow.id, follow.created_at, follow.updated_at, follow.developer, follow.entity_id, follow.entity_url, json_agg("user".*) as subscribers FROM follows follow JOIN users "user" ON "user".uuid::text = ANY (subscribers::text[]) WHERE entity_id = $1 GROUP BY follow.id`
const CreateFollowNotification = `INSERT INTO notifications(created_at, updated_at, "user", UUID, status, "data") VALUES (now(), now(), :subscriber, :notification_id, 'unread', :data)`

const UpdateFollowLatUpdated = `UPDATE follows SET updated_at = now() where entity_id = $1;`

const UpdateFollowStatus = `UPDATE follows SET updated_at = now(), status = $1 where entity_id = $2;`

// create a new waitlist entry and update updated_at if email already exists

const CreateWaitlistEntry = `INSERT INTO waitlists(uuid, email, platform, created_at,  updated_at) VALUES ($1, $2, $3, now(), now()) ON CONFLICT(email) DO UPDATE SET updated_at = now() RETURNING email;`

//const UpdateRedirectURL = `UPDATE users SET redirect_url = $2 WHERE uuid = $1;`

const FetchUserFromWaitlist = `SELECT uuid FROM waitlists WHERE email = $1;`

const SaveUserResetToken = `UPDATE users SET reset_token = $2, reset_token_expiry = $3 WHERE uuid = $1;`

const FindUserByResetToken = `SELECT * FROM users WHERE reset_token = $1 AND reset_token_expiry > now();`

//const FindUserByResetToken = `SELECT uuid FROM users WHERE reset_token = $1 AND reset_token_expiry > ;`

//const FetchPlaylistFollowsToProcess = `SELECT task.*, COALESCE(follow.entity_url, '') entity_url FROM follows follow JOIN tasks task ON task.uuid = follow.task WHERE task IS NOT NULL
//--  	AND task.updated_at > CURRENT_DATE - interval '10 minutes'
