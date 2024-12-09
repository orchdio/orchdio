ALTER TABLE IF EXISTS apps
    ALTER COLUMN created_at TYPE pg_catalog.date USING created_at::pg_catalog.date;

ALTER TABLE IF EXISTS apps
    ALTER COLUMN updated_at TYPE pg_catalog.date USING updated_at::pg_catalog.date;