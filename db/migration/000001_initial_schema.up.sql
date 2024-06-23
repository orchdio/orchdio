create table if not exists public.users
(
    id               integer generated always as identity
        primary key,
    email            text
        unique,
    username         text,
    uuid             uuid
        unique,
    created_at       date default '2022-09-08'::date,
    updated_at       date default '2022-09-08'::date,
    usernames        json,
    refresh_token    bytea,
    platform_id      text
        unique,
    spotify_token    bytea,
    applemusic_token bytea,
    deezer_token     bytea,
    tidal_token      bytea,
    platform_ids     json
);

comment on column public.users.platform_ids is 'the json holding the platform and the ids for the user as key and value respectively.';

alter table public.users
    owner to postgres;

