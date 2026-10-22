-- +goose Up
-- User data: a video with no file. Its user key is the content key of a location-less videos row that startup recreates.
create table virtual_videos (
    -- Autoincrement never reuses an id, so the user key stays unique for the life of the database.
    id             integer primary key autoincrement,
    -- virtual:<id>; the key of the projected videos row and of its user data.
    user_key       text    not null unique,
    -- Trimmed and not empty.
    title          text    not null,
    -- Normalized as video_locations.title_key and search_key; startup refreshes rows below the current version.
    title_key      text    not null default '',
    search_key     text    not null default '',
    search_version integer not null default 0,
    -- Unix seconds; creation is the added, created and modified sort value.
    created_at     integer not null,
    updated_at     integer not null
);

-- +goose Down
delete from videos where content_key in (select user_key from virtual_videos);
drop table virtual_videos;
