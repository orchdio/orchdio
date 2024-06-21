-- drop old columns
alter table if exists public.users
    drop column if exists username;

alter table if exists public.users
    drop column if exists usernames;

alter table if exists public.users
    drop column if exists refresh_token;

alter table if exists public.users
    drop column if exists platform_id;

alter table if exists public.users
    drop column if exists spotify_token;

alter table if exists public.users
    drop column if exists applemusic_token;

alter table if exists public.users
    drop column if exists deezer_token;

alter table if exists public.users
    drop column if exists tidal_token;

alter table if exists public.users
    drop column if exists platform_ids;


-- new columns

alter table if exists public.users
    add if not exists password text;

alter table if exists public.users
    add if not exists reset_token text;

alter table if exists public.users
    add if not exists reset_token_expiry timestamptz;

alter table if exists public.users
    add if not exists reset_token_created_at timestamptz;
