-- +goose Up
-- Go-derived natural-sort keys support indexed ordering and keyset pagination without changing tag identity.
alter table tag_names add column sort_key text not null default '';

-- Version zero schedules existing rejected names for startup key backfill.
alter table rejected_tag_names add column sort_key text not null default '';
alter table rejected_tag_names add column search_version integer not null default 0;

-- Mark existing names stale so startup fills sort keys alongside search keys.
update tag_names set search_version = 0;

-- +goose Down
alter table rejected_tag_names drop column search_version;
alter table rejected_tag_names drop column sort_key;
alter table tag_names drop column sort_key;
