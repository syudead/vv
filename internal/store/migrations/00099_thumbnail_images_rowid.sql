-- +goose Up
-- Rebuild as a rowid table with the blobs last. In the WITHOUT ROWID table every key comparison read the whole
-- overflowing row, so each per-video revision lookup cost about a millisecond even when it found nothing.
create table thumbnail_images_rowid (
    content_key text    primary key,
    -- Use the same revision rule as thumbnail positions for cache-busting URLs.
    revision    integer not null,
    -- Application time in Unix milliseconds.
    applied_at  integer not null,
    origin      text    not null default 'generated',
    -- JPEG cropped to the video's display aspect ratio; bytes match the published thumbnail.
    image       blob    not null,
    -- The same crop at up to 2048 pixels for future editing; NULL falls back to image.
    source      blob
);
insert into thumbnail_images_rowid (content_key, revision, applied_at, origin, image, source)
    select content_key, revision, applied_at, origin, image, source from thumbnail_images;
drop table thumbnail_images;
alter table thumbnail_images_rowid rename to thumbnail_images;

-- +goose Down
create table thumbnail_images_without_rowid (
    content_key text    primary key,
    image       blob    not null,
    revision    integer not null,
    applied_at  integer not null,
    source      blob,
    origin      text    not null default 'generated'
) without rowid;
insert into thumbnail_images_without_rowid (content_key, image, revision, applied_at, source, origin)
    select content_key, image, revision, applied_at, source, origin from thumbnail_images;
drop table thumbnail_images;
alter table thumbnail_images_without_rowid rename to thumbnail_images;
