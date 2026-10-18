-- +goose Up
-- Persistent smart folders store domain.SmartRule JSON and nonrandom ordering; deleting a parent cascades descendants.
create table smart_folders (
    id         integer primary key autoincrement,
    parent_id  integer references smart_folders (id) on delete cascade,
    name       text    not null check (length(name) between 1 and 100),
    -- domain.NaturalSortKey(name)。
    title_key  text    not null,
    rule       text    not null,
    sort       text    not null,
    -- Marks a folder created or edited by the owner.
    owner_kept integer not null default 1 check (owner_kept in (0, 1)),
    -- Unix milliseconds.
    created_at integer not null,
    -- Last name, rule, sort or parent edit in Unix milliseconds.
    updated_at integer not null,
    check (parent_id is null or parent_id <> id)
);
create index smart_folders_parent_idx on smart_folders (parent_id);

-- Favorited time is Unix milliseconds.
create table smart_folder_favorites (
    smart_folder_id integer primary key references smart_folders (id) on delete cascade,
    favorited_at    integer not null
);

-- Exactly one video user key, group ID or smart-folder ID is present.
create table playlist_items_new (
    id              integer primary key autoincrement,
    playlist_id     integer not null references playlists (id) on delete cascade,
    content_key     text,
    group_id        integer references groups (id) on delete cascade,
    smart_folder_id integer references smart_folders (id) on delete cascade,
    position        integer not null,
    -- Unix milliseconds.
    added_at        integer not null,
    check ((content_key is not null) + (group_id is not null) + (smart_folder_id is not null) = 1),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, group_id),
    unique (playlist_id, smart_folder_id)
);

insert into playlist_items_new (id, playlist_id, content_key, group_id, position, added_at)
select id, playlist_id, content_key, group_id, position, added_at from playlist_items;

-- Continue the previous table's ID sequence.
insert into sqlite_sequence (name, seq)
select 'playlist_items_new', 0
 where not exists (select 1 from sqlite_sequence where name = 'playlist_items_new');
update sqlite_sequence
   set seq = max(seq, coalesce((select seq from sqlite_sequence where name = 'playlist_items'), 0))
 where name = 'playlist_items_new';

drop index if exists playlist_items_group_idx;
drop index if exists playlist_items_content_idx;
drop index if exists playlist_items_order_idx;
drop table playlist_items;
alter table playlist_items_new rename to playlist_items;
create index playlist_items_order_idx on playlist_items (playlist_id, position);
create index playlist_items_content_idx on playlist_items (content_key);
create index playlist_items_group_idx on playlist_items (group_id);
create index playlist_items_smart_folder_idx on playlist_items (smart_folder_id);

-- +goose Down
-- Drop smart-folder entries, compact positions and advance changed playlist timestamps beyond their previous values.
create table playlist_items_old (
    id          integer primary key autoincrement,
    playlist_id integer not null references playlists (id) on delete cascade,
    content_key text,
    group_id    integer references groups (id) on delete cascade,
    position    integer not null,
    added_at    integer not null,
    check ((content_key is null) <> (group_id is null)),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, group_id)
);

insert into playlist_items_old (id, playlist_id, content_key, group_id, position, added_at)
select id, playlist_id, content_key, group_id,
       row_number() over (partition by playlist_id order by position, id) - 1,
       added_at
  from playlist_items
 where smart_folder_id is null;

update playlists
   set updated_at = max(cast((julianday('now') - 2440587.5) * 86400000 as integer), updated_at + 1)
 where id in (select playlist_id from playlist_items where smart_folder_id is not null);

insert into sqlite_sequence (name, seq)
select 'playlist_items_old', 0
 where not exists (select 1 from sqlite_sequence where name = 'playlist_items_old');
update sqlite_sequence
   set seq = max(seq, coalesce((select seq from sqlite_sequence where name = 'playlist_items'), 0))
 where name = 'playlist_items_old';

drop index if exists playlist_items_smart_folder_idx;
drop index if exists playlist_items_group_idx;
drop index if exists playlist_items_content_idx;
drop index if exists playlist_items_order_idx;
drop table playlist_items;
alter table playlist_items_old rename to playlist_items;
create index playlist_items_order_idx on playlist_items (playlist_id, position);
create index playlist_items_content_idx on playlist_items (content_key);
create index playlist_items_group_idx on playlist_items (group_id);

drop table if exists smart_folder_favorites;
drop index if exists smart_folders_parent_idx;
drop table if exists smart_folders;
