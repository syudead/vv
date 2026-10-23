-- +goose Up
-- Watch-state, playable, added-within and duration filters were removed; ParseSmartRule rejects their keys.
update smart_folders set rule = json_remove(rule, '$.watch', '$.playable', '$.addedWithin', '$.minDuration', '$.maxDuration')
 where json_type(rule, '$.watch') is not null or json_type(rule, '$.playable') is not null
    or json_type(rule, '$.addedWithin') is not null or json_type(rule, '$.minDuration') is not null
    or json_type(rule, '$.maxDuration') is not null;
-- These values served only the removed playable and added-within filters.
drop index smart_folder_values_expiry_idx;
alter table smart_folder_values drop column expires_at;
alter table group_values drop column playable;

-- +goose Down
-- The removed rule conditions cannot be restored; recompute the restored columns on the next refresh.
alter table group_values add column playable integer not null default 0;
alter table smart_folder_values add column expires_at integer;
create index smart_folder_values_expiry_idx on smart_folder_values(expires_at) where expires_at is not null;
insert into collection_changes values ('group', 0), ('smart', 0);
