-- +goose Up
-- Content-keyed display and thumbnail overrides are persistent user data without video foreign keys.
create table video_overrides (
    content_key           text    primary key,
    -- Normalized display name; unset is NULL, never an empty string.
    display_name          text,
    -- Thumbnail frame position in milliseconds, or NULL for automatic selection.
    thumbnail_position_ms integer check (thumbnail_position_ms is null or thumbnail_position_ms >= 0),
    -- Revision used in thumbnail URLs instead of exposing position; NULL for automatic selection.
    thumbnail_revision    integer,
    updated_at            integer not null
) without rowid;

-- +goose Down
drop table if exists video_overrides;
