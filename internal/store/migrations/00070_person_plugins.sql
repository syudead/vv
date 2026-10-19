-- +goose Up
-- A person plugin is a work_info_plugins row whose manifest says kind person; stored modules are not inspected again.
alter table work_info_plugins add column kind text not null default 'work';
-- JSON array of the manifest's sources; null for a work-info plugin and for an unreadable manifest.
alter table work_info_plugins add column sources text;
-- Whether work-info fetches may call this person plugin; a same-name replacement keeps it.
alter table work_info_plugins add column auto_lookup integer not null default 1;

-- +goose Down
alter table work_info_plugins drop column auto_lookup;
alter table work_info_plugins drop column sources;
alter table work_info_plugins drop column kind;
