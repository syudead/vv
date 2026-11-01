-- +goose Up
-- The folder's switch: 0 leaves its videos out of this node's lists, searches and counts, while scans, watching,
-- ingest and jobs keep treating the folder as registered. Every existing folder starts on.
alter table media_folders add column enabled integer not null default 1 check (enabled in (0, 1));

-- +goose Down
alter table media_folders drop column enabled;
