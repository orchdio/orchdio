package queries

const CreateNewApp = `INSERT INTO apps (uuid, name, description, redirect_url, 
                  webhook_url, public_key, developer, 
                  secret_key, verify_token, organization, deezer_state, created_at,
                  updated_at) 
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now()) RETURNING uuid`

const UpdateAppIntegrationCredentials = `UPDATE apps SET 
deezer_credentials = (CASE WHEN $3 = 'deezer' AND length($1::bytea) > 0 
    THEN  $1::bytea ELSE deezer_credentials END),
applemusic_credentials = (CASE WHEN $3 = 'applemusic' AND length($1::bytea) > 0 
    THEN $1::bytea ELSE applemusic_credentials END),
spotify_credentials = (CASE WHEN $3 = 'spotify' 
        AND length($1::bytea) > 0 THEN $1::bytea ELSE spotify_credentials END),
tidal_credentials = (CASE WHEN $3 = 'tidal' AND length($3::bytea) > 0 THEN $1::bytea ELSE tidal_credentials END),

webhook_url = $4, redirect_url = $5, authorized = true, convoy_endpoint_id = $6, updated_at = now() WHERE uuid = $2`

//const UpdateAppRedirect = `UPDATE apps SET spotify_redirect_url = (CASE WHEN $2 = 'spotify' THEN $2 END),
//tidal_redirect_url = (CASE WHEN $2 = 'tidal' THEN $2 END),
//deezer_redirect_url = (CASE WHEN $2 = 'deezer' THEN $2 END),
//applemusic_redirect_url = (CASE WHEN $2 = 'applemusic' THEN $2 END) WHERE uuid = $1`

const FetchAppByAppID = `SELECT Id, uuid, name, description, developer, secret_key, public_key,  coalesce(webhook_url, '') as webhook_url,
       coalesce(redirect_url, '') as redirect_url, coalesce(verify_token, '') as verify_token, created_at, updated_at, coalesce(authorized, false) as authorized, organization,
       COALESCE(spotify_credentials, '') AS spotify_credentials, COALESCE(applemusic_credentials, '') AS applemusic_credentials, COALESCE(deezer_credentials, '') AS deezer_credentials, COALESCE(tidal_credentials, '') AS tidal_credentials,
--        COALESCE(spotify_redirect_url, '') AS spotify_redirect_url, COALESCE(applemusic_redirect_url, '') AS applemusic_redirect_url, COALESCE(deezer_redirect_url, '') AS deezer_redirect_url, COALESCE(tidal_redirect_url, '') AS tidal_redirect_url
		coalesce(deezer_state, '') AS deezer_state, COALESCE(convoy_endpoint_id, '') AS endpoint_id FROM apps WHERE uuid = $1`

const FetchAppByAppIDWithoutDev = `SELECT Id, uuid, name, description, 
       developer, secret_key, public_key,
       spotify_credentials, applemusic_credentials, deezer_credentials, tidal_credentials,
--            COALESCE(spotify_redirect_url, '') AS spotify_redirect_url, COALESCE(applemusic_redirect_url, '') AS applemusic_redirect_url, COALESCE(deezer_redirect_url, '') AS deezer_redirect_url, COALESCE(tidal_redirect_url, '') AS tidal_redirect_url,
       coalesce(redirect_url, '') as redirect_url, coalesce(webhook_url, '') as webhook_url, coalesce(verify_token, '') as verify_token, 
    created_at, updated_at, coalesce(authorized, false) as authorized, organization, coalesce(deezer_state, '') AS deezer_state, coalesce(convoy_endpoint_id, '') as endpoint_id FROM apps WHERE uuid = $1;`

const FetchAppByPubKeyWithoutDev = `SELECT Id, uuid, name, description, developer, secret_key, public_key,
--        COALESCE(spotify_redirect_url, '') AS spotify_redirect_url, COALESCE(applemusic_redirect_url, '') AS applemusic_redirect_url, COALESCE(deezer_redirect_url, '') AS deezer_redirect_url, COALESCE(tidal_redirect_url, '') AS tidal_redirect_url,
       coalesce(redirect_url, '') as redirect_url, coalesce(webhook_url, '') as webhook_url, coalesce(verify_token, '') as verify_token,
       COALESCE(spotify_credentials, '') AS spotify_credentials, COALESCE(applemusic_credentials, '') AS applemusic_credentials, 
       COALESCE(deezer_credentials, '') AS deezer_credentials, COALESCE(tidal_credentials, '') AS tidal_credentials, 
       created_at, updated_at, coalesce(authorized, false) as authorized, organization, 
       coalesce(deezer_state, '') AS deezer_state, coalesce(convoy_endpoint_id, '') as endpoint_id FROM apps WHERE public_key = $1`

const FetchAppByPubKey = `SELECT Id, uuid, name, description, developer, secret_key, public_key,
--        COALESCE(spotify_redirect_url, '') AS spotify_redirect_url, COALESCE(applemusic_redirect_url, '') AS applemusic_redirect_url, COALESCE(deezer_redirect_url, '') AS deezer_redirect_url, COALESCE(tidal_redirect_url, '') AS tidal_redirect_url, webhook_url,
       coalesce(redirect_url, '') as redirect_url, coalesce(verify_token, '') as verify_token, coalesce(webhook_url, '') as webhook_url,
       COALESCE(spotify_credentials, '') AS spotify_credentials, COALESCE(applemusic_credentials, '') AS applemusic_credentials, COALESCE(deezer_credentials, '') AS deezer_credentials, COALESCE(tidal_credentials, '') AS tidal_credentials, created_at, updated_at, coalesce(authorized, false) as authorized, organization, coalesce(deezer_state, '') AS deezer_state, coalesce(convoy_endpoint_id, '') as endpoint_id FROM apps WHERE public_key = $1 AND developer = $2`

const FetchAppBySecretKey = `SELECT Id, uuid, name, description, developer, secret_key, public_key,
--        COALESCE(spotify_redirect_url, '') AS spotify_redirect_url, COALESCE(applemusic_redirect_url, '') AS applemusic_redirect_url, COALESCE(deezer_redirect_url, '') AS deezer_redirect_url, COALESCE(tidal_redirect_url, '') AS tidal_redirect_url, webhook_url,
       coalesce(redirect_url, '') as redirect_url, coalesce(verify_token, '') as verify_token, coalesce(webhook_url, '') as webhook_url,
       COALESCE(spotify_credentials, '') AS spotify_credentials, COALESCE(applemusic_credentials, '') 
           AS applemusic_credentials, COALESCE(deezer_credentials, '') AS deezer_credentials, COALESCE(tidal_credentials, '') 
               AS tidal_credentials, created_at, updated_at, coalesce(authorized, false) as authorized, organization, coalesce(deezer_state, '') 
                   AS deezer_state, coalesce(convoy_endpoint_id, '') as endpoint_id FROM apps WHERE secret_key = $1`

const FetchAuthorizedAppDeveloperByPublicKey = `SELECT u.email, u.id, u.uuid, u.created_at, u.updated_at FROM apps a JOIN users u on a.developer = u.uuid WHERE a.public_key = $1 AND a.authorized = true`
const FetchAuthorizedAppDeveloperBySecretKey = `SELECT u.email, u.id, u.uuid, u.created_at, u.updated_at FROM apps a JOIN users u on a.developer = u.uuid WHERE a.secret_key = $1 AND a.authorized = true`

// UpdateApp updates the developer app with data passed. If the values are empty, it falls back to what the original value of the column is
const UpdateApp = `UPDATE apps SET  description = (CASE WHEN $1 = '' THEN description ELSE $1 END),
                 name = (CASE WHEN $2 = '' THEN name ELSE $2 END),
webhook_url = (CASE WHEN $4 = '' THEN webhook_url ELSE $4 END),
redirect_url = (CASE WHEN $3 = '' THEN redirect_url ELSE $3 END),

deezer_credentials = (CASE WHEN $8 = 'deezer' AND length($7::bytea) > 0 THEN  $7::bytea ELSE deezer_credentials END),
applemusic_credentials = (CASE WHEN $8 = 'applemusic' AND length($7::bytea) > 0 THEN $7::bytea ELSE applemusic_credentials END),
spotify_credentials = (CASE WHEN $8 = 'spotify' AND length($7::bytea) > 0 THEN $7::bytea ELSE spotify_credentials END),
tidal_credentials = (CASE WHEN $8 = 'tidal' AND length($7::bytea) > 0 THEN $7::bytea ELSE tidal_credentials END),

updated_at = now() WHERE uuid = $5 AND developer = $6`

const DeleteApp = `DELETE FROM apps WHERE uuid = $1 AND developer = $2`

// App queries

const DisableApp = `UPDATE apps SET authorized = false WHERE uuid = $1 AND developer = $2;`
const EnableApp = `UPDATE apps SET authorized = true WHERE uuid = $1 AND developer = $2;`
const FetchAppKeysByID = `SELECT public_key, secret_key, verify_token FROM apps WHERE uuid = $1 AND developer = $2;`

const FetchAppsByDeveloper = `SELECT 
 id, uuid, name, description, developer, secret_key, public_key,
 redirect_url, webhook_url, verify_token, spotify_credentials,
 applemusic_credentials, tidal_credentials, deezer_credentials,
 created_at, updated_at, coalesce(authorized, false) as authorized, organization, coalesce(deezer_state, '') as deezer_state 
FROM apps WHERE developer = $1 and organization = $2`

const UpdateAppKeys = `UPDATE apps SET public_key = $1, secret_key = $2, verify_token = $3, deezer_state = $4 WHERE uuid = $5`
const RevokeSecretKey = `update apps set secret_key = $2 where uuid = $1`
const RevokeVerifySecret = `update apps set verify_token = $2 where uuid = $1`
const RevokeDeezerState = `update apps set deezer_state = $2 where uuid = $1`
const RevokePublicKey = `update apps set public_key = $2 where uuid = $1`

const CreateNewOrg = `INSERT INTO 
    organizations (uuid, name, description, created_at, updated_at, owner) 
VALUES ($1, $2, $3, now(), now(), $4) RETURNING uuid`

const DeleteOrg = `DELETE FROM organizations WHERE uuid = $1 AND owner = $2`
const UpdateOrg = `UPDATE organizations SET description = (CASE WHEN $1 = '' THEN description ELSE $1 END), name = (CASE WHEN $2 = '' THEN name ELSE $2 END), updated_at = now() WHERE uuid = $3 AND owner = $4 `
const FetchUserOrg = `SELECT * FROM organizations WHERE owner = $1 order by updated_at desc limit 1`

const FetchUserApp = `SELECT * FROM user_apps WHERE uuid = $1 AND "user" = $2`
const FetchUserAppByPlatform = `SELECT uuid, refresh_token, "user", coalesce(authed_at, now()) as authed_at, 
       coalesce(last_authed_at, now()) as last_authed_at, app, platform, coalesce(username, '') as username, coalesce(platform_id, '') as platform_id, coalesce(scopes, '{}') AS scopes FROM user_apps 
WHERE platform = $1 AND "user" = $2 and app = $3`

const CreateUserApp = `INSERT INTO user_apps (
                       uuid, refresh_token, scopes, "user", platform, app, last_authed_at
) VALUES ($1, $2, ARRAY[$3], $4, $5, $6, now()) RETURNING uuid`

const UpdateUserAppTokensAndScopes = `UPDATE user_apps SET 
                     refresh_token = $1::bytea,
                     scopes = (CASE WHEN $2 = '' THEN scopes ELSE ARRAY[$2] END)
                 where app = $3 AND "user" = $4 AND platform = $5 and uuid = $6 returning uuid`

const UpdateDeezerState = `UPDATE apps SET deezer_state = $1 WHERE uuid = $2;`

const FetchUserAppByPlatformAndApp = `SELECT uuid, scopes FROM user_apps WHERE platform = $1 AND app = $2`

const FetchAppByDeezerState = `SELECT Id, uuid, name, description, 
       developer, secret_key, public_key,
       coalesce(redirect_url, '') as redirect_url, coalesce(verify_token, '') as verify_token, coalesce(webhook_url, '') as webhook_url,
--        spotify_credentials, applemusic_credentials, deezer_credentials, tidal_credentials,
--            COALESCE(spotify_redirect_url, '') AS spotify_redirect_url, COALESCE(applemusic_redirect_url, '') AS applemusic_redirect_url, COALESCE(deezer_redirect_url, '') AS deezer_redirect_url, COALESCE(tidal_redirect_url, '') AS tidal_redirect_url,
--        redirect_url, webhook_url, verify_token,
       coalesce(spotify_credentials, '') as spotify_credentials, coalesce(applemusic_credentials, '') as applemusic_credentials, coalesce(deezer_credentials, '') as deezer_credentials, coalesce(tidal_credentials, '') as tidal_credentials,
    created_at, updated_at, coalesce(authorized, false) as authorized, organization, coalesce(deezer_state, '') AS deezer_state, coalesce(convoy_endpoint_id, '') as endpoint_id FROM apps WHERE deezer_state = $1`

const UpdateUserAppScopes = `UPDATE user_apps uap SET scopes = ARRAY(SELECT distinct unnest(uap.scopes || $1))
FROM apps ap WHERE ap.uuid = uap.app 
        AND uap.uuid = $2 AND "user" = $3 AND platform = $4 AND app = $5`

const FetchUserAppAndInfo = `SELECT uapps.uuid as app_id, uapps.platform, coalesce(uapps.platform_id, '') as platform_id, 
       uapps.refresh_token, coalesce(uapps.username, '') as username, u.email, "user" as user_id FROM user_apps uapps JOIN users u on uapps."user" = u.uuid
and uapps.app = $2
         WHERE ( CASE WHEN $3 = 'id' THEN u.uuid::text = $1 ELSE u.email = $1 END )
	AND app IS NOT NULL`

// since for a user app, there is only an app for each plaform, so we can narrow down using platform when filtering
// by user app's app's id

const FetchUserAppAndInfoByPlatform = `SELECT uapps.uuid as app_id, 
       uapps.platform, coalesce(uapps.platform_id, '') as platform_id,
uapps.refresh_token, coalesce(uapps.username, '') as username, u.email, "user" as user_id
	FROM user_apps uapps JOIN users u on uapps."user" = u.uuid AND uapps.app = $2 
	WHERE (CASE WHEN $3 = 'id' THEN u.uuid::text = $1 ELSE u.email = $1 END) AND platform = $4`

// update user platform token based on the streaming platform user provides

//const UpdateUserPlatformToken = `UPDATE user_apps SET refresh_token = $1 WHERE uuid = $2`

//const UpdatePlatformUsernamesAndIds = `UPDATE users SET
//                usernames = COALESCE(usernames::JSONB, '{}') || $2,
//                platform_ids = COALESCE(platform_ids::JSONB, '{}') || $3 WHERE email = $1;`

const UpdatePlatformUserNameIdAndToken = `UPDATE user_apps SET username = $1, 
platform_id = $2, refresh_token = $3, last_authed_at = now() WHERE "user" = $4 AND platform = $5 AND uuid = $6`

const DeletePlatformIntegrationCredentials = `UPDATE apps SET
deezer_credentials = ( CASE WHEN $2 = 'deezer' THEN NULL ELSE deezer_credentials END ),
tidal_credentials = ( CASE WHEN $2 = 'tidal' THEN NULL ELSE tidal_credentials END ),
spotify_credentials = ( CASE WHEN $2 = 'spotify' THEN NULL ELSE spotify_credentials END ),
applemusic_credentials = ( CASE WHEN $2 = 'applemusic' THEN NULL ELSE applemusic_credentials END ) WHERE uuid = $1 AND developer = $3`

const UpdateConvoyEndpointID = `UPDATE apps SET convoy_endpoint_id = $1 WHERE uuid = $2`
