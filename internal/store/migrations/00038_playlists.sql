-- +goose Up
-- プレイリスト。作り直せない利用者データで、
-- 項目は動画の利用者データの鍵（集まりのメンバーなら 'bundle:<id>'）かグループのフォルダの鍵
-- （domain.FolderKey）で持ち、videos・folder_groups への外部キーを張らない。生成物の片付け・
-- releaseContentIndex・rebuildFolderIndex・メディアフォルダの削除はどちらの表にも触れない。
create table playlists (
    id         integer primary key autoincrement,
    name       text    not null check (length(name) between 1 and 100),
    -- Unix ミリ秒。
    created_at integer not null,
    -- Unix ミリ秒。名前か項目を最後に変えた時刻。
    updated_at integer not null
);

create table playlist_items (
    id          integer primary key autoincrement,
    playlist_id integer not null references playlists (id) on delete cascade,
    -- どちらか一方だけを持つ。
    content_key text,
    folder_key  text,
    -- プレイリストの中で 0 から詰めた位置（R-3）。
    position    integer not null,
    -- Unix ミリ秒。
    added_at    integer not null,
    check ((content_key is null) <> (folder_key is null)),
    check (content_key is null or content_key <> ''),
    unique (playlist_id, content_key),
    unique (playlist_id, folder_key)
);
create index playlist_items_order_idx on playlist_items (playlist_id, position);
create index playlist_items_content_idx on playlist_items (content_key);

-- +goose Down
drop index if exists playlist_items_content_idx;
drop index if exists playlist_items_order_idx;
drop table if exists playlist_items;
drop table if exists playlists;
