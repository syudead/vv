-- +goose Up
-- Rebuildable media information comes from ingest probing.
create table video_media_info (
    video_id   integer primary key references videos (id) on delete cascade,
    -- A mismatched domain.MediaInfoVersion is read as absent.
    version    integer not null,
    -- Serialized domain.MediaInfo.
    info       text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists video_media_info;
