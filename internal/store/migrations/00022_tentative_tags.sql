-- +goose Up
-- Automatic creation marks tags tentative; confirmation changes only this flag.
alter table tags add column tentative integer not null default 0 check (tentative in (0, 1));

-- Remember rejected canonical names by exact spelling so tentative creation can skip them.
create table rejected_tag_names (
    name       text    primary key,
    created_at integer not null
) without rowid;

-- +goose Down
drop table if exists rejected_tag_names;
alter table tags drop column tentative;
