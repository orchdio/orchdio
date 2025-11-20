alter table if exists user_apps
    add if not exists expires_in timestamptz;

alter table if exists user_apps
    add if not exists access_token text;
