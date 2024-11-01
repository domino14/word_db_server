BEGIN;

-- This is a django table, part of the aerolith app. We don't need to create it here,
-- but just tell sqlc that it exists.

CREATE table IF NOT EXISTS auth_user(id BIGINT NOT NULL, username TEXT);

COMMIT;