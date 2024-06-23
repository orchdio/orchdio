-- drop old columns
alter table public.users
    drop column if exists username;

alter table public.users
    drop column if exists usernames;

alter table public.users
    drop column if exists refresh_token;

alter table public.users
    drop column if exists platform_id;

alter table public.users
    drop column if exists spotify_token;

alter table public.users
    drop column if exists applemusic_token;

alter table public.users
    drop column if exists deezer_token;

alter table public.users
    drop column if exists tidal_token;

alter table public.users
    drop column if exists platform_ids;


-- new columns

alter table if exists public.users
    add password text;

alter table if exists public.users
    add reset_token text;

alter table if exists public.users
    add reset_token_expiry timestamptz;

alter table if exists public.users
    add reset_token_created_at timestamptz;
