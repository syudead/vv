-- +goose Up
alter table video_locations add column mtime_ns integer;

-- +goose Down
alter table video_locations drop column mtime_ns;
