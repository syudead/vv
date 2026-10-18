-- +goose Up
-- Windows queries lower(path), so an expression index avoids full location scans; other systems use the existing path index.
create index video_locations_lower_path_idx on video_locations (lower(path));

-- +goose Down
drop index if exists video_locations_lower_path_idx;
