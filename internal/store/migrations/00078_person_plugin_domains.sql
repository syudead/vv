-- +goose Up
-- JSON array of a person plugin manifest's domains; null when the manifest declares none or is not a person plugin's.
alter table work_info_plugins add column domains text;

-- +goose Down
alter table work_info_plugins drop column domains;
