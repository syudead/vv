-- +goose Up
-- The account is persistent user data; sessions are temporary and recoverable through login.

-- Absence means setup is incomplete; only initial setup creates the singleton row.
create table account (
    id            integer primary key check (id = 1),
    -- BINARY comparison is exact and case-sensitive.
    username      text    not null,
    -- Store the Argon2id PHC string, never plaintext.
    password_hash text    not null,
    -- Increment on every username or password change.
    version       integer not null default 1 check (version >= 1),
    updated_at    integer not null
);

create table sessions (
    -- Store only the hexadecimal SHA-256 hash; the session token itself stays in the cookie.
    token_hash      text    primary key,
    -- The account version at issuance; mismatches invalidate the session.
    account_version integer not null,
    created_at      integer not null,
    -- Fixed 90-day lifetime, never extended.
    expires_at      integer not null
) without rowid;

create index sessions_expires_at on sessions (expires_at);

-- +goose Down
drop table if exists sessions;
drop table if exists account;
