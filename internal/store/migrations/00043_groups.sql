-- +goose Up
-- Groups persist independently of videos and index cleanup, using user keys for videos and IDs for child groups.
create table groups (
    id            integer primary key autoincrement,
    name          text    not null check (length(name) between 1 and 100),
    -- domain.NaturalSortKey(name)。
    title_key     text    not null,
    -- Automatic folder identity from domain.FolderKey; manual groups use NULL.
    origin_folder text    unique,
    -- Unix milliseconds.
    created_at    integer not null,
    -- Last name or item edit in Unix milliseconds.
    updated_at    integer not null
);

create table group_items (
    id          integer primary key autoincrement,
    group_id    integer not null references groups (id) on delete cascade,
    -- Exactly one target is present.
    content_key text,
    child_id    integer references groups (id) on delete cascade,
    position    integer not null,
    -- Marks items added by folder synchronization.
    from_folder integer not null default 0 check (from_folder in (0, 1)),
    check ((content_key is null) <> (child_id is null)),
    check (content_key is null or content_key <> ''),
    check (from_folder = 0 or content_key is not null),
    check (child_id is null or child_id <> group_id),
    unique (group_id, content_key),
    unique (group_id, child_id)
);
create index group_items_order_idx on group_items (group_id, position);
create index group_items_content_idx on group_items (content_key);
create index group_items_child_idx on group_items (child_id);

-- Remember folder-derived members removed by the owner.
create table group_exclusions (
    group_id    integer not null references groups (id) on delete cascade,
    content_key text    not null check (content_key <> ''),
    primary key (group_id, content_key)
) without rowid;

-- Migrate indexed groups as automatic groups, truncating names to 100 characters and recomputing natural-sort keys from the truncated name.
insert into groups (name, title_key, origin_folder, created_at, updated_at)
select substr(name, 1, 100), vv_natural_sort_key(substr(name, 1, 100)), path_key,
       cast((julianday('now') - 2440587.5) * 86400000 as integer),
       cast((julianday('now') - 2440587.5) * 86400000 as integer)
  from folder_groups
 order by id;

-- Copy identified members by shared user key and compact positions from zero.
insert into group_items (group_id, content_key, position, from_folder)
select group_id, content_key, row_number() over (partition by group_id order by first_position) - 1, 1
  from (select g.id as group_id,
               coalesce((select ukb.user_key from video_bundle_members ukm
                           join video_bundles ukb on ukb.id = ukm.bundle_id
                          where ukm.content_key = v.content_key), v.content_key) as content_key,
               min(m.position) as first_position
          from folder_group_members m
          join folder_groups fg on fg.id = m.group_id
          join groups g on g.origin_folder = fg.path_key
          join videos v on v.id = m.video_id
         where v.content_key <> ''
         group by 1, 2);

-- +goose Down
drop index if exists group_items_child_idx;
drop index if exists group_items_content_idx;
drop index if exists group_items_order_idx;
drop table if exists group_exclusions;
drop table if exists group_items;
drop table if exists groups;
