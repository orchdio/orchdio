alter table if exists users
    alter column created_at drop default;

alter table if exists users
    alter column created_at type timestamp using created_at::timestamp;

alter table  if exists users
    alter column updated_at drop default;

alter table if exists users
    alter column updated_at type timestamp using updated_at::timestamp;