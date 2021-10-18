package queries

const CreateUserQuery = `WITH user_rec as ( INSERT INTO "user"(email) VALUES($1) ON CONFLICT("email") DO UPDATE SET email=EXCLUDED.email RETURNING id, email ),
     	platforms_rec as (INSERT INTO platform("user", platform_id, display_name, href, identifier, token) VALUES ( (SELECT user_rec.id FROM user_rec), $2, $3, $4, $5, $6) ON CONFLICT("identifier") DO NOTHING)
			SELECT * from user_rec;`

const FindUserByEmail = `SELECT u.email, jsonb_object_agg(platform.identifier, platform.display_name) usernames, jsonb_object_agg(platform.identifier, platform.platform_id) ids
	FROM platform JOIN "user" u ON u.id = platform.user WHERE u.email = $1 group by u.email;`
