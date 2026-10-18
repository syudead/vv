-- +goose Up
-- A row marks content public; no video foreign key preserves this owner setting across rescans and moves.
create table public_videos (
    content_key  text    primary key,
    published_at integer not null
) without rowid;

-- +goose Down
drop table if exists public_videos;
