-- Migration 1
drop table if exists public.users;

-- Migration 2
drop table if exists public.organizations;

drop table if exists public.apps;

drop table if exists public.user_apps;

drop table if exists public.tasks;

drop table if exists public.follows;

drop table if exists public.waitlists;


-- Migration 3
-- add old columns
alter table if exists public.users
    add column username text;

alter table if exists public.users
    add column usernames text;

alter table if exists public.users
    add column refresh_token text;

alter table if exists public.users
    add column platform_id text;

alter table if exists public.users
    add column spotify_token text;

alter table if exists public.users
    add column applemusic_token text;

alter table if exists public.users
    add column deezer_token text;

alter table if exists public.users
    add column tidal_token text;

alter table if exists public.users
    add column platform_ids text;

-- drop new columns
alter table if exists public.users
    drop column password;

alter table if exists public.users
    drop column reset_token;

alter table if exists public.users
    drop column reset_token_expiry;

alter table if exists public.users
    drop column reset_token_created_at;


-- Migration 4
-- add old columns
alter table if exists public.users
    add column username text;

alter table if exists public.users
    add column usernames text;

alter table if exists public.users
    add column refresh_token text;

alter table if exists public.users
    add column platform_id text;

alter table if exists public.users
    add column spotify_token text;

alter table if exists public.users
    add column applemusic_token text;

alter table if exists public.users
    add column deezer_token text;

alter table if exists public.users
    add column tidal_token text;

alter table if exists public.users
    add column platform_ids text;

-- drop new columns
alter table if exists public.users
    drop column password;

alter table if exists public.users
    drop column reset_token;

alter table if exists public.users
    drop column reset_token_expiry;

alter table if exists public.users
    drop column reset_token_created_at;


-- Migration 5
alter table if exists users
    alter column created_at drop default;

alter table if exists users
    alter column created_at type timestamp using created_at::timestamp;

alter table  if exists users
    alter column updated_at drop default;

alter table if exists users
    alter column updated_at type timestamp using updated_at::timestamp;


-- Migration 6
alter table if exists user_apps
    drop constraint if exists user_apps_pk;

alter table if exists user_apps
    drop column if exists id;

alter table if exists user_apps
    drop column if exists username;


-- Migration 7
alter table if exists user_apps
    drop column if exists platform_ids;


-- Migration 8
ALTER TABLE IF EXISTS apps
    ALTER COLUMN created_at TYPE date USING created_at::date;

ALTER TABLE IF EXISTS apps
    ALTER COLUMN updated_at TYPE date USING updated_at::date;

-- Migration 9
alter table if exists apps drop column webhook_app_id;

-- Migration 10
alter table follows
    alter column created_at type date using created_at::date;

alter table follows
    alter column created_at set default '2022-09-08'::date;

alter table tasks
    alter column created_at type timestamp using created_at::timestamp;

alter table tasks
    alter column updated_at type timestamp using updated_at::timestamp;

alter table user_apps
    alter column authed_at type timestamp using authed_at::timestamp;

alter table user_apps
    alter column last_authed_at type timestamp using last_authed_at::timestamp;

alter table follows
    alter column updated_at type date using updated_at::date;

alter table follows
    alter column updated_at set default '2022-09-08'::date;