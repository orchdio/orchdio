alter table if exists apps
    alter column created_at type pg_catalog.timestamptz using created_at::pg_catalog.timestamptz;

alter table apps
    alter column updated_at type timestamptz using updated_at::timestamptz;
