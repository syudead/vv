-- +goose Up
-- A person's or organization's names become one ordered list: position 0 is the display name and the rest follow
-- in the owner's order (docs/design-docs/credit-names.md). The tables are altered, not recreated, so their primary
-- keys and foreign keys stay as migration 42 made them.
alter table people_names add column position integer not null default 0;
alter table organization_names add column position integer not null default 0;

-- The canonical row comes first and the aliases follow in the order the screens listed them, which the Go collation
-- vv_alias_order gives; sort_key cannot, since it folds width and kana.
update people_names set position = numbered.position
  from (select person_id, name,
               row_number() over (partition by person_id order by canonical desc, name collate vv_alias_order) - 1
                 as position
          from people_names) numbered
 where people_names.person_id = numbered.person_id and people_names.name = numbered.name;
update organization_names set position = numbered.position
  from (select organization_id, name,
               row_number() over (partition by organization_id order by canonical desc, name collate vv_alias_order) - 1
                 as position
          from organization_names) numbered
 where organization_names.organization_id = numbered.organization_id and organization_names.name = numbered.name;

create unique index people_names_position_idx on people_names (person_id, position);
create unique index organization_names_position_idx on organization_names (organization_id, position);
drop index people_names_canonical_idx;
drop index organization_names_canonical_idx;
alter table people_names drop column canonical;
alter table organization_names drop column canonical;

-- +goose Down
alter table people_names add column canonical integer not null default 0 check (canonical in (0, 1));
alter table organization_names add column canonical integer not null default 0 check (canonical in (0, 1));
update people_names set canonical = 1 where position = 0;
update organization_names set canonical = 1 where position = 0;
create unique index people_names_canonical_idx on people_names (person_id) where canonical = 1;
create unique index organization_names_canonical_idx on organization_names (organization_id) where canonical = 1;
drop index people_names_position_idx;
drop index organization_names_position_idx;
alter table people_names drop column position;
alter table organization_names drop column position;
