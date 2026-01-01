alter table apps
    add soundcloud_credentials bytea;

comment on column apps.soundcloud_credentials is 'integration credentials for this app';
