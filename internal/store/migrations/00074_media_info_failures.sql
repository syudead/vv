-- +goose Up
-- A background media-information re-read that failed for this file; it is tried again once the file or the version changes.
create table video_media_info_failures (
    video_id   integer primary key references videos (id) on delete cascade,
    version    integer not null,
    size_bytes integer not null,
    mtime_ns   integer not null
) without rowid;

-- +goose Down
drop table if exists video_media_info_failures;
