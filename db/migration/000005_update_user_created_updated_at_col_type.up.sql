alter table if exists users
    alter column created_at type timestamptz using created_at::timestamptz;

alter table if exists users
    alter column created_at set default '2022-09-08'::timestamptz;

alter table if exists users
    alter column updated_at type timestamptz using updated_at::timestamptz;

alter table if exists users
    alter column updated_at set default '2022-09-08'::timestamptz;

