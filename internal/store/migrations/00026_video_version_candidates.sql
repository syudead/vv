-- +goose Up
-- Rebuild candidate pairs in the fingerprint transaction; each sorted pair occupies one row.
create table video_version_candidates (
    key_a      text    not null,
    key_b      text    not null,
    distance   integer not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- +goose Down
drop table if exists video_version_candidates;
