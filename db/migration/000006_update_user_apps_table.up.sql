alter table if exists user_apps
    add id integer generated always as identity;

alter table if exists user_apps
    add username varchar;

alter table if exists user_apps
    add constraint user_apps_pk
        primary key (id);
