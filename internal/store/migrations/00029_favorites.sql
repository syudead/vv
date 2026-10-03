-- +goose Up
-- お気に入り（specs/035-favorites/research.md R-1、data-model.md §1）。どちらも作り直せない
-- 利用者データで、再スキャン・メディアフォルダの変更・フォルダの索引の作り直しで消えてはならない。
-- videos・folder_groups・media_folders への外部キーを張らない。

-- 動画のお気に入り。鍵は利用者データの鍵（集まりのメンバーなら 'bundle:<id>'、そうでなければ
-- content_key。public_videos と同じ）。favorited_at は Unix ミリ秒。
create table video_favorites (
    content_key  text    primary key,
    favorited_at integer not null
) without rowid;

-- グループのお気に入り。鍵はフォルダの絶対パスを domain.FolderKey で整えたもの
-- （folder_group_overrides.path と同じ）。favorited_at は Unix ミリ秒。
create table folder_favorites (
    path         text    primary key,
    favorited_at integer not null
) without rowid;

-- +goose Down
drop table if exists folder_favorites;
drop table if exists video_favorites;
