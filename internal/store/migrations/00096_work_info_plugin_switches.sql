-- +goose Up
-- The plugin's switch: 0 keeps the plugin, its approval and settings but leaves it out of every run, fetch choice
-- and link fetch. Every existing plugin starts on.
alter table work_info_plugins add column enabled integer not null default 1 check (enabled in (0, 1));

-- +goose Down
alter table work_info_plugins drop column enabled;
