-- +goose Up
-- Persistent edit times have no video foreign key; absent rows fall back to video added time.
create table video_edits (
    content_key text    primary key,
    edited_at   integer not null
) without rowid;

-- +goose Down
drop table if exists video_edits;
