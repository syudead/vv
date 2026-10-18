-- +goose Up
-- groups and group_items replace these obsolete index tables.

drop table folder_group_members;
drop table folder_groups;

-- +goose Down
-- Restore empty index tables and mark them stale so the previous application rebuilds them at startup.
create table folder_groups (
    id              integer primary key,
    path_key        text    not null unique,
    path            text    not null,
    name            text    not null,
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    position  integer not null,
    unique (group_id, position)
);

update folder_index_state set stale = 1;
