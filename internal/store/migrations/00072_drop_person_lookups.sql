-- +goose Up
-- Work-info fetches no longer look people up with person plugins; the person page fetches each person instead.
alter table work_info_plugins drop column auto_lookup;

-- +goose Down
alter table work_info_plugins add column auto_lookup integer not null default 1;
