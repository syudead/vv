-- +goose Up
-- File creation time is a rebuildable location fact in Unix seconds; unknown values remain NULL until a scan fills them.
alter table video_locations add column file_created_at integer;

-- +goose Down
alter table video_locations drop column file_created_at;
