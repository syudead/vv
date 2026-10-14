-- +goose Up
-- お気に入りとプレイリストの項目のグループを、フォルダの鍵からグループの id で名指すように替える
-- （specs/048-groups/research.md R-10、data-model.md「Migration」）。今の自動グループの
-- origin_folder に一致するフォルダの鍵の行だけをそのグループの行に写し、ほかは落とす。
-- どちらもグループの解散（groups の行の削除）で外部キーの on delete cascade により消える。

-- グループのお気に入り。favorited_at は Unix ミリ秒。
create table group_favorites (
    group_id     integer primary key references groups (id) on delete cascade,
    favorited_at integer not null
);

insert into group_favorites (group_id, favorited_at)
select g.id, ff.favorited_at
  from folder_favorites ff
  join groups g on g.origin_folder = ff.path;

drop table folder_favorites;

-- 項目は動画の利用者データの鍵（content_key）かグループの id（group_id）のどちらか一方を持つ。
create table playlist_items_new (
    id          integer primary key autoincrement,
    playlist_id integer not null references playlists (id) on delete cascade,
    content_key text,
    group_id    integer references groups (id) on delete cascade,
    -- プレイリストの中で 0 から詰めた位置（specs/044-playlists/research.md R-3）。
    position    integer not null,
    -- Unix ミリ秒。
    added_at    integer not null,
    check ((content_key is null) <> (group_id is null)),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, group_id)
);

-- どの origin_folder にも一致しないグループの項目は落とし、残りの位置を今の並びのまま詰め直す。
insert into playlist_items_new (id, playlist_id, content_key, group_id, position, added_at)
select i.id, i.playlist_id, i.content_key, g.id,
       row_number() over (partition by i.playlist_id order by i.position, i.id) - 1,
       i.added_at
  from playlist_items i
  left join groups g on i.folder_key is not null and g.origin_folder = i.folder_key
 where i.content_key is not null or g.id is not null;

-- 項目を落としたプレイリストは更新日時を前の値より後へ進める。同じ更新日時なら同じ並びという約束
-- （specs/044-playlists/ui-design.md「One order」）を保つため。
update playlists
   set updated_at = max(cast((julianday('now') - 2440587.5) * 86400000 as integer), updated_at + 1)
 where id in (select i.playlist_id
                from playlist_items i
               where i.folder_key is not null
                 and not exists (select 1 from groups g where g.origin_folder = i.folder_key));

-- 採番を前の表の続きから行う。末尾の項目を落としても、消えた項目の id を新しい項目に使い回さない。
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
-- 落とした folder_favorites は空で戻す。プレイリストのグループの項目は、自動グループのものだけ
-- そのフォルダの鍵へ戻し、手で作ったグループの項目は落として詰め直す。
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
