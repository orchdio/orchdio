alter table users
    alter column created_at type timestamptz using created_at::timestamptz;

alter table users
    alter column created_at set default '2022-09-08'::timestamptz;

alter table users
    alter column updated_at type timestamptz using updated_at::timestamptz;

alter table users
    alter column updated_at set default '2022-09-08'::timestamptz;

