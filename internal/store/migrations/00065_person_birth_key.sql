-- +goose Up
-- birth_key orders people by birth date as YYYYMMDD with missing parts as 00; the store writes it with birth_date.
alter table people add column birth_key integer;
update people
   set birth_key = cast(replace(substr(birth_date || '-00-00', 1, 10), '-', '') as integer)
 where birth_date is not null;

-- +goose Down
alter table people drop column birth_key;
