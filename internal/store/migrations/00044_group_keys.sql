-- +goose Up
-- Map only existing automatic folder groups to IDs; references cascade when those groups dissolve.

-- Favorited time is Unix milliseconds.
create table group_favorites (
    group_id     integer primary key references groups (id) on delete cascade,
    favorited_at integer not null
);

insert into group_favorites (group_id, favorited_at)
select g.id, ff.favorited_at
  from folder_favorites ff
  join groups g on g.origin_folder = ff.path;

drop table folder_favorites;

-- Exactly one content user key or group ID is present.
create table playlist_items_new (
    id          integer primary key autoincrement,
    playlist_id integer not null references playlists (id) on delete cascade,
    content_key text,
    group_id    integer references groups (id) on delete cascade,
    position    integer not null,
    -- Unix milliseconds.
    added_at    integer not null,
    check ((content_key is null) <> (group_id is null)),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, group_id)
);

-- Drop unresolved folder groups and compact remaining positions without changing order.
insert into playlist_items_new (id, playlist_id, content_key, group_id, position, added_at)
select i.id, i.playlist_id, i.content_key, g.id,
       row_number() over (partition by i.playlist_id order by i.position, i.id) - 1,
       i.added_at
  from playlist_items i
  left join groups g on i.folder_key is not null and g.origin_folder = i.folder_key
 where i.content_key is not null or g.id is not null;

-- Advance timestamps for changed playlists so equal timestamps continue to imply equal order.
update playlists
   set updated_at = max(cast((julianday('now') - 2440587.5) * 86400000 as integer), updated_at + 1)
 where id in (select i.playlist_id
                from playlist_items i
               where i.folder_key is not null
                 and not exists (select 1 from groups g where g.origin_folder = i.folder_key));

-- Continue the old sequence even when trailing items were dropped; deleted IDs must not be reused.
insert into sqlite_sequence (name, seq)
select 'playlist_items_new', 0
 where not exists (select 1 from sqlite_sequence where name = 'playlist_items_new');
update sqlite_sequence
   set seq = max(seq, coalesce((select seq from sqlite_sequence where name = 'playlist_items'), 0))
 where name = 'playlist_items_new';

drop index if exists playlist_items_content_idx;
drop index if exists playlist_items_order_idx;
drop table playlist_items;
alter table playlist_items_new rename to playlist_items;
create index playlist_items_order_idx on playlist_items (playlist_id, position);
create index playlist_items_content_idx on playlist_items (content_key);
create index playlist_items_group_idx on playlist_items (group_id);

-- +goose Down
-- Restore favorites empty; only automatic group items can map back to folder keys, so discard manual groups and compact positions.
create table folder_favorites (
    path         text    primary key,
    favorited_at integer not null
) without rowid;

drop table group_favorites;

create table playlist_items_old (
    id          integer primary key autoincrement,
    playlist_id integer not null references playlists (id) on delete cascade,
    content_key text,
    folder_key  text,
    position    integer not null,
    added_at    integer not null,
    check ((content_key is null) <> (folder_key is null)),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, folder_key)
);

insert into playlist_items_old (id, playlist_id, content_key, folder_key, position, added_at)
select i.id, i.playlist_id, i.content_key, g.origin_folder,
       row_number() over (partition by i.playlist_id order by i.position, i.id) - 1,
       i.added_at
  from playlist_items i
  left join groups g on g.id = i.group_id
 where i.content_key is not null or g.origin_folder is not null;

update playlists
   set updated_at = max(cast((julianday('now') - 2440587.5) * 86400000 as integer), updated_at + 1)
 where id in (select i.playlist_id
                from playlist_items i
                join groups g on g.id = i.group_id
               where g.origin_folder is null);

insert into sqlite_sequence (name, seq)
select 'playlist_items_old', 0
 where not exists (select 1 from sqlite_sequence where name = 'playlist_items_old');
update sqlite_sequence
   set seq = max(seq, coalesce((select seq from sqlite_sequence where name = 'playlist_items'), 0))
 where name = 'playlist_items_old';

drop index if exists playlist_items_group_idx;
drop index if exists playlist_items_content_idx;
drop index if exists playlist_items_order_idx;
drop table playlist_items;
alter table playlist_items_old rename to playlist_items;
create index playlist_items_order_idx on playlist_items (playlist_id, position);
create index playlist_items_content_idx on playlist_items (content_key);
