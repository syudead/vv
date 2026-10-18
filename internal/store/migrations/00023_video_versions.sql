-- +goose Up
-- Persistent bundles have no video foreign keys and store shared user data under their user_key.
create table video_bundles (
    id                 integer primary key autoincrement,
    -- The writer derives bundle:<id>, which cannot collide with hexadecimal content keys.
    user_key           text    not null unique,
    -- The representative content key must belong to this bundle.
    representative_key text    not null,
    created_at         integer not null,
    updated_at         integer not null
);

create table video_bundle_members (
    content_key text    primary key,
    bundle_id   integer not null references video_bundles (id) on delete cascade,
    added_at    integer not null
) without rowid;
create index video_bundle_members_bundle_idx on video_bundle_members (bundle_id, content_key);

-- Persistent different-video decisions use one sorted key pair per row.
create table video_version_dismissals (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- +goose Down
drop table if exists video_version_dismissals;
drop index if exists video_bundle_members_bundle_idx;
drop table if exists video_bundle_members;
drop table if exists video_bundles;
