-- +goose Up
-- Favorites are persistent user data and must survive index rebuilds and registration changes.

-- Use a shared bundle key or content key; favorited_at is Unix milliseconds.
create table video_favorites (
    content_key  text    primary key,
    favorited_at integer not null
) without rowid;

-- Use domain.FolderKey for folder identity; favorited_at is Unix milliseconds.
create table folder_favorites (
    path         text    primary key,
    favorited_at integer not null
) without rowid;

-- +goose Down
drop table if exists folder_favorites;
drop table if exists video_favorites;
