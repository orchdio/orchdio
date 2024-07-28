alter table if exists user_apps
    drop constraint if exists user_apps_pk;

alter table if exists user_apps
    drop column if exists id;

alter table if exists user_apps
    drop column if exists username;
