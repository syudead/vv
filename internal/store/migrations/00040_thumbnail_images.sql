-- +goose Up
-- Persist applied images by content key without video foreign keys so thumbnail files can be rebuilt.
create table thumbnail_images (
    content_key text    primary key,
    -- JPEG cropped to the video's display aspect ratio; bytes match the published thumbnail.
    image       blob    not null,
    -- Use the same revision rule as thumbnail positions for cache-busting URLs.
    revision    integer not null,
    -- Application time in Unix milliseconds.
    applied_at  integer not null
) without rowid;

-- +goose Down
drop table if exists thumbnail_images;
