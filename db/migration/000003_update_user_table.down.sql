-- add old columns
alter table public.users
    add column username text;

alter table public.users
    add column usernames text;

alter table public.users
    add column refresh_token text;

alter table public.users
    add column platform_id text;

alter table public.users
    add column spotify_token text;

alter table public.users
    add column applemusic_token text;

alter table public.users
    add column deezer_token text;

alter table public.users
    add column tidal_token text;

alter table public.users
    add column platform_ids text;

-- drop new columns
alter table public.users
    drop column password;

alter table public.users
    drop column reset_token;

alter table public.users
    drop column reset_token_expiry;

alter table public.users
    drop column reset_token_created_at;