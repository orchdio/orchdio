-- Version: 2
-- Organization table
create table public.organizations
(
    name        varchar,
    created_at  timestamp with time zone default now(),
    updated_at  timestamp with time zone default now(),
    owner       uuid
        constraint organizations_owner__fk
            references public.users (uuid)
            on update cascade on delete cascade,
    id          integer generated always as identity
        constraint organizations_pk
            primary key,
    description varchar,
    uuid        uuid
        constraint organizations_unique_key
            unique
);

comment on table public.organizations is 'the org table';

comment on column public.organizations.name is 'the name of this org';

comment on column public.organizations.created_at is 'the timestamp this org was created';

comment on column public.organizations.updated_at is 'the timestamp this org was updated';

comment on column public.organizations.owner is 'the owner of this org';

comment on column public.organizations.description is 'the description of this organization';

alter table public.organizations
    owner to postgres;


-- Apps (dev apps) table
create table public.apps
(
    uuid                   uuid not null
        constraint apps_unique_pk
            unique,
    id                     integer generated always as identity
        constraint apps_primary_key
            primary key,
    developer              uuid
        constraint apps_developer_fk
            references public.users (uuid),
    created_at             date,
    updated_at             date,
    secret_key             varchar,
    public_key             uuid
        constraint apps_public_key_unique_key
            unique,
    name                   varchar,
    redirect_url           varchar,
    webhook_url            varchar,
    verify_token           bytea,
    description            varchar,
    authorized             boolean default true,
    organization           uuid
        constraint apps_organization_fk
            references public.organizations (uuid)
            on update cascade on delete cascade,
    spotify_credentials    bytea,
    deezer_credentials     bytea,
    applemusic_credentials bytea,
    tidal_credentials      bytea,
    deezer_state           varchar
);

comment on table public.apps is 'the applications created by the user';

comment on column public.apps.uuid is 'the unique id of the app';

comment on constraint apps_primary_key on public.apps is 'apps table primary key ';

comment on column public.apps.developer is 'the developer associated with this app';

comment on constraint apps_developer_fk on public.apps is 'the fk for the user (developer) associated with this pk';

comment on column public.apps.secret_key is 'encrypted secret key for the app';

comment on column public.apps.public_key is 'the public key for this app';

comment on constraint apps_public_key_unique_key on public.apps is 'the unique key for the public for the app';

comment on column public.apps.name is 'the name of the app';

comment on column public.apps.redirect_url is 'the redirect url attached to this app';

comment on column public.apps.webhook_url is 'the webhook url associated with this app';

comment on column public.apps.description is 'the description of the application';

comment on column public.apps.authorized is 'a column that specifies if the user has authorized a platform';

comment on column public.apps.organization is 'the organization this app belongs to';

comment on column public.apps.spotify_credentials is 'the encrypted spotify credentials for this app';

comment on column public.apps.deezer_credentials is 'the encrypted deezer credentials for this app';

comment on column public.apps.applemusic_credentials is 'the encrypted apple music credentials for this app';

comment on column public.apps.tidal_credentials is 'the encrypted tidal credentials for this app';

comment on column public.apps.deezer_state is 'a 10bytes max shortid for deezer apps';

alter table public.apps
    owner to postgres;



-- User Apps table
create table public.user_apps
(
    uuid           uuid,
    refresh_token  bytea,
    "user"         uuid
        constraint user_apps_user_fk
            references public.users (uuid)
            on update cascade on delete cascade,
    authed_at      timestamp default now(),
    last_authed_at timestamp default now(),
    app            uuid
        constraint user_apps_app__fk
            references public.apps (uuid)
            on update cascade on delete cascade,
    platform       varchar,
    scopea         character varying[]
);

comment on table public.user_apps is 'the apps the user has authed on orchdio';

comment on column public.user_apps.uuid is 'unique uuid for this app';

comment on column public.user_apps.refresh_token is 'the refresh token for this user for this app';

comment on column public.user_apps."user" is 'the user associated with this app';

comment on constraint user_apps_user_fk on public.user_apps is 'the user that owns this app';

comment on column public.user_apps.authed_at is 'the time this app was first created';

comment on column public.user_apps.last_authed_at is 'the time this app was last updated';

comment on column public.user_apps.app is 'the app that this user authed and this user_app was created for';

comment on constraint user_apps_app__fk on public.user_apps is 'the foreign key to the app that this user_app was created/updated for';

comment on column public.user_apps.platform is 'the platform this app belongs to';

comment on column public.user_apps.scopea is 'the scopes the user authed for this app';

alter table public.user_apps
    owner to postgres;



-- Tasks table
create table public.tasks
(
    id          integer generated always as identity
        primary key,
    uuid        uuid
        unique,
    entity_id   text,
    created_at  timestamp,
    updated_at  timestamp,
    "user"      uuid
        constraint task_creator_key
            references public.users (uuid)
            on update cascade on delete cascade,
    status      text    default 'pending'::text,
    result      json,
    type        text,
    shortid     text
        unique,
    retry_count integer default 0,
    app         uuid
        constraint task_app
            references public.apps (uuid)
);

comment on column public.tasks.retry_count is 'the numbers of retries the task has had';

alter table public.tasks
    owner to postgres;


-- Follows table
create table public.follows
(
    id          serial
        primary key,
    created_at  date default '2022-09-08'::date,
    updated_at  date default '2022-09-08'::date,
    uuid        uuid,
    subscribers uuid
        constraint follow_subscriber_fk
            references public.users (uuid)
            on update cascade on delete cascade,
    entity_id   uuid
        unique,
    developer   uuid
        constraint follow_developer
            references public.users (uuid)
            on update cascade on delete cascade,
    entity_url  text,
    status      text default 'ready'::text
);

comment on constraint follow_subscriber_fk on public.follows is 'the follow subscriber user record relation fk';

alter table public.follows
    owner to postgres;


-- Waitlists table
create table public.waitlists
(
    id         integer generated always as identity
        primary key,
    email      text
        unique,
    created_at time,
    updated_at time,
    uuid       uuid
        unique,
    platform   text
);

comment on column public.waitlists.platform is 'the platform the user primarily uses for streaming';

alter table public.waitlists
    owner to postgres;



