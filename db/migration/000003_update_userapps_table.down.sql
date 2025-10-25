alter table if exists user_apps
    drop column if exists expires_in;

alter table if exists user_apps
    drop column if exists access_token;
