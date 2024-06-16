-- drop old columns
alter table public.users
    drop column username;

alter table public.users
    drop column usernames;

alter table public.users
    drop column refresh_token;

alter table public.users
    drop column platform_id;

alter table public.users
    drop column spotify_token;

alter table public.users
    drop column applemusic_token;

alter table public.users
    drop column deezer_token;

alter table public.users
    drop column tidal_token;

alter table public.users
    drop column platform_ids;


-- new columns

alter table public.users
    add password text;

alter table public.users
    add reset_token text;

alter table public.users
    add reset_token_expiry timestamptz;

alter table public.users
    add reset_token_created_at timestamptz;
