-- +goose Up

-- Persistent overrides have no foreign keys to registrations or videos.
create table folder_group_overrides (
    -- Absolute folder path normalized by domain.FolderKey.
    path       text    primary key,
    mode       text    not null check (mode in ('ungroup', 'group_direct')),
    updated_at integer not null
) without rowid;

-- The following indexes are replaced completely by rebuildFolderIndex.
create table folder_groups (
    id              integer primary key,
    -- Stable folder identity from domain.FolderKey.
    path_key        text    not null unique,
    -- Use the spelling from the smallest location path beneath the folder.
    path            text    not null,
    name            text    not null,
    -- Natural-sort key from domain.NaturalSortKey(name).
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    position  integer not null,
    unique (group_id, position)
);

create table video_folder_names (
    video_id integer not null references videos (id) on delete cascade,
    name     text    not null,
    primary key (video_id, name)
) without rowid;
create index video_folder_names_name_idx on video_folder_names (name, video_id);

-- Rebuild at startup when either folder rules or search-key normalization versions change.
create table folder_index_state (
    id             integer primary key check (id = 1),
    version        integer not null,   -- domain.FolderIndexVersion
    search_version integer not null,   -- domain.SearchKeyVersion
    -- Set after a failed rebuild and cleared after success.
    stale          integer not null default 0 check (stale in (0, 1))
);

-- +goose Down
drop table if exists folder_index_state;
drop table if exists video_folder_names;
drop table if exists folder_group_members;
drop table if exists folder_groups;
drop table if exists folder_group_overrides;
