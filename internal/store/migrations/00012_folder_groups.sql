-- +goose Up
-- フォルダのグループ（specs/017-folder-groups/data-model.md §1）。

-- 利用者データ。再スキャン・メディアフォルダの変更・再起動で消えてはならない。
-- 登録フォルダにも videos にも外部キーを張らない。
create table folder_group_overrides (
    -- フォルダの絶対パスを domain.FolderKey で整えたもの。
    path       text    primary key,
    mode       text    not null check (mode in ('ungroup', 'group_direct')),
    updated_at integer not null
) without rowid;

-- 以下は索引。作り直し（rebuildFolderIndex）が丸ごと置き換える。
create table folder_groups (
    id              integer primary key,
    -- フォルダを指す鍵。domain.FolderKey。グループの同一性はこれで決める。
    path_key        text    not null unique,
    -- フォルダの絶対パスの綴り（そのフォルダの下の所在のうちパスの最小のものから取る）。
    path            text    not null,
    -- フォルダ名（domain.FolderName と同じ最後の段）。
    name            text    not null,
    -- 題名順の並べ替えの鍵。domain.NaturalSortKey(name)。
    title_key       text    not null
);

create table folder_group_members (
    video_id  integer primary key references videos (id) on delete cascade,
    group_id  integer not null references folder_groups (id) on delete cascade,
    -- グループの中の並び（0 始まり）。
    position  integer not null,
    unique (group_id, position)
);

-- 動画ごとの祖先フォルダ名。
create table video_folder_names (
    video_id integer not null references videos (id) on delete cascade,
    name     text    not null,
    primary key (video_id, name)
) without rowid;
create index video_folder_names_name_idx on video_folder_names (name, video_id);

-- 索引を作った規則の版。どちらかが今の値と違えば起動時に作り直す。
-- title_key は domain.NaturalSortKey で作るので、その版（SearchKeyVersion）も持つ。
create table folder_index_state (
    id             integer primary key check (id = 1),
    version        integer not null,   -- domain.FolderIndexVersion
    search_version integer not null,   -- domain.SearchKeyVersion
    -- 1 は「前回の作り直しが失敗した」。作り直しが成功すると 0 に戻る。
    stale          integer not null default 0 check (stale in (0, 1))
);

-- +goose Down
drop table if exists folder_index_state;
drop table if exists video_folder_names;
drop table if exists folder_group_members;
drop table if exists folder_groups;
drop table if exists folder_group_overrides;
