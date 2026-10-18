-- +goose Up
-- Playlists are persistent user data; content/folder keys survive index cleanup and registration changes.
create table playlists (
    id         integer primary key autoincrement,
    name       text    not null check (length(name) between 1 and 100),
    -- Unix milliseconds.
    created_at integer not null,
    -- Last name or item edit in Unix milliseconds.
    updated_at integer not null
);

create table playlist_items (
    id          integer primary key autoincrement,
    playlist_id integer not null references playlists (id) on delete cascade,
    -- Exactly one target is present.
    content_key text,
    folder_key  text,
    position    integer not null,
    -- Unix milliseconds.
    added_at    integer not null,
    check ((content_key is null) <> (folder_key is null)),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, folder_key)
);
create index playlist_items_order_idx on playlist_items (playlist_id, position);
create index playlist_items_content_idx on playlist_items (content_key);

-- +goose Down
drop index if exists playlist_items_content_idx;
drop index if exists playlist_items_order_idx;
drop table if exists playlist_items;
drop table if exists playlists;
