-- +goose Up
-- スマートフォルダ。作り直せない
-- 利用者データで、videos への外部キーを張らない。条件（rule）は domain.SmartRule の JSON、並べ順
-- （sort）は random 以外の VideoSort である。親を消すと子孫も外部キーで消える。
create table smart_folders (
    id         integer primary key autoincrement,
    parent_id  integer references smart_folders (id) on delete cascade,
    name       text    not null check (length(name) between 1 and 100),
    -- domain.NaturalSortKey(name)。
    title_key  text    not null,
    rule       text    not null,
    sort       text    not null,
    -- 所有者が作ったか直したフォルダ。
    owner_kept integer not null default 1 check (owner_kept in (0, 1)),
    -- Unix ミリ秒。
    created_at integer not null,
    -- Unix ミリ秒。名前・条件・並べ順・親を最後に変えた時刻。
    updated_at integer not null,
    check (parent_id is null or parent_id <> id)
);
create index smart_folders_parent_idx on smart_folders (parent_id);

-- スマートフォルダのお気に入り。favorited_at は Unix ミリ秒。
create table smart_folder_favorites (
    smart_folder_id integer primary key references smart_folders (id) on delete cascade,
    favorited_at    integer not null
);

-- プレイリストの項目は、動画の利用者データの鍵（content_key）・グループの id（group_id）・スマート
-- フォルダの id（smart_folder_id）のちょうど 1 つを持つ。
create table playlist_items_new (
    id              integer primary key autoincrement,
    playlist_id     integer not null references playlists (id) on delete cascade,
    content_key     text,
    group_id        integer references groups (id) on delete cascade,
    smart_folder_id integer references smart_folders (id) on delete cascade,
    -- プレイリストの中で 0 から詰めた位置。
    position        integer not null,
    -- Unix ミリ秒。
    added_at        integer not null,
    check ((content_key is not null) + (group_id is not null) + (smart_folder_id is not null) = 1),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, group_id),
    unique (playlist_id, smart_folder_id)
);

insert into playlist_items_new (id, playlist_id, content_key, group_id, position, added_at)
select id, playlist_id, content_key, group_id, position, added_at from playlist_items;

-- 採番を前の表の続きから行う（00044_group_keys.sql と同じ）。
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
-- スマートフォルダの項目を落として位置を詰め直し、落としたプレイリストの更新日時を前の値より後へ
-- 進める（00044_group_keys.sql の Down と同じ約束）。
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
