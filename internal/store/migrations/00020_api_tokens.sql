-- +goose Up
-- API tokens are persistent settings; lost tokens must be reissued, not rebuilt by scanning.
create table api_tokens (
    -- Never reuse revoked token IDs.
    id              integer primary key autoincrement,
    -- Normalized purpose label; duplicates are allowed.
    name            text    not null,
    -- Store only the hexadecimal SHA-256 hash, never plaintext.
    token_hash      text    not null unique,
    -- The account version at issuance; mismatches invalidate the token.
    account_version integer not null,
    created_at      integer not null,
    -- Last use in Unix seconds, NULL until used; write no more often than once per minute.
    last_used_at    integer
);

-- +goose Down
drop table if exists api_tokens;
