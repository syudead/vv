-- +goose Up
-- Increment generation on replacement and revocation so stale fetched results remain invalid after reapproval of identical bytes.
alter table work_info_plugins add column generation integer not null default 0;

-- +goose Down
alter table work_info_plugins drop column generation;
