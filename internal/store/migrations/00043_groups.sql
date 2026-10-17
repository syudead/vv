-- +goose Up
-- 任意のグループ。作り直せない
-- 利用者データで、動画の項目は利用者データの鍵（集まりのメンバーなら 'bundle:<id>'）、子グループの
-- 項目はグループの id で持ち、videos への外部キーを張らない。生成物の片付け・releaseContentIndex・
-- メディアフォルダの削除はどの表にも触れない。
create table groups (
    id            integer primary key autoincrement,
    name          text    not null check (length(name) between 1 and 100),
    -- domain.NaturalSortKey(name)。
    title_key     text    not null,
    -- 自動グループのフォルダの domain.FolderKey。手で作ったグループでは null。
    origin_folder text    unique,
    -- Unix ミリ秒。
    created_at    integer not null,
    -- Unix ミリ秒。名前か項目を最後に変えた時刻。
    updated_at    integer not null
);

create table group_items (
    id          integer primary key autoincrement,
    group_id    integer not null references groups (id) on delete cascade,
    -- どちらか一方だけを持つ（R-2）。
    content_key text,
    child_id    integer references groups (id) on delete cascade,
    -- グループの中で 0 から詰めた位置。
    position    integer not null,
    -- 同期が足した項目（R-3）。
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

-- 所有者が自動グループから外したフォルダ由来のメンバー（R-3）。
create table group_exclusions (
    group_id    integer not null references groups (id) on delete cascade,
    content_key text    not null check (content_key <> ''),
    primary key (group_id, content_key)
) without rowid;

-- 今のフォルダの索引のグループを、そのまま自動グループにする。作成と更新の時刻は移行の時刻。
-- folder_groups.name には長さの上限が無いので、同期が新しく作るときと同じく先頭 100 文字に切り
-- （domain.cutGroupName）、title_key は切った名前から vv_natural_sort_key（internal/store/groups.go）で作り直す。
insert into groups (name, title_key, origin_folder, created_at, updated_at)
select substr(name, 1, 100), vv_natural_sort_key(substr(name, 1, 100)), path_key,
       cast((julianday('now') - 2440587.5) * 86400000 as integer),
       cast((julianday('now') - 2440587.5) * 86400000 as integer)
  from folder_groups
 order by id;

-- メンバーは利用者データの鍵（internal/store/user_keys.go の userKeyExpr と同じ式）で、
-- フォルダ由来として写す。空の content_key の動画は除き、位置は 0 から詰め直す。
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
