-- +goose Up
-- Tag kinds are persistent user data; each tag has zero or one kind.

-- Never reuse deleted IDs; normalized kind names have a separate namespace from tag names.
create table tag_kinds (
    id         integer primary key autoincrement,
    name       text    not null unique,
    created_at integer not null
);

-- Seed in this ID order, then treat presets as ordinary renameable/deletable kinds.
insert into tag_kinds (name, created_at) values ('Genre', unixepoch());
insert into tag_kinds (name, created_at) values ('Work', unixepoch());
insert into tag_kinds (name, created_at) values ('Place', unixepoch());
insert into tag_kinds (name, created_at) values ('Character', unixepoch());

-- Deleting a kind preserves its tags as unclassified.
alter table tags add column kind_id integer references tag_kinds (id) on delete set null;

create index tags_kind_id on tags (kind_id);

-- +goose Down
drop index if exists tags_kind_id;
alter table tags drop column kind_id;
drop table if exists tag_kinds;
