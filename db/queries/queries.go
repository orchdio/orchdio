package queries

const CreateUserQuery = `WITH user_rec as ( INSERT INTO "users"(email, username, uuid) VALUES($1, $2, $3) ON CONFLICT("email") DO UPDATE SET email=EXCLUDED.email RETURNING id, email)
			SELECT * from user_rec;`

const FindUserByEmail = `SELECT u.email, jsonb_object_agg(platform.identifier, platform.display_name) usernames, jsonb_object_agg(platform.identifier, platform.platform_id) ids
	FROM platform JOIN "user" u ON u.id = platform.user WHERE u.email = $1 group by u.email;`
