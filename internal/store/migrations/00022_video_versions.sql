-- +goose Up
-- 同じ動画の別バージョンの集まり（specs/030-video-versions/research.md R-1、data-model.md §1）。
-- 作り直せない利用者データで、videos への外部キーを張らない。集まりのタグ・再生位置・公開の設定は
-- video_tags・playback_progress・public_videos に user_key を鍵として置く。
create table video_bundles (
    id                 integer primary key autoincrement,
    -- 集まりの値の鍵。'bundle:' || id を書く側が作る。content_key（<16進>:<サイズ>）と重ならない。
    user_key           text    not null unique,
    -- 代表のバージョンの content_key。メンバーの1つである（不変条件）。
    representative_key text    not null,
    created_at         integer not null,
    updated_at         integer not null
);

create table video_bundle_members (
    content_key text    primary key,
    bundle_id   integer not null references video_bundles (id) on delete cascade,
    added_at    integer not null
) without rowid;
create index video_bundle_members_bundle_idx on video_bundle_members (bundle_id, content_key);

-- 「違う動画」と判断した組（要件 10）。利用者データ。key_a < key_b に並べて 1 行。
create table video_version_dismissals (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- 以下は索引。内容の参照が無くなるときに消し、走査と取り込みで作り直せる。

-- 同じパスの中身が変わった後継の候補（research.md R-5、data-model.md §5）。記録した走査が
-- 閉じ、新しい中身の解析が終わったときに判定して消す。
create table video_successions (
    new_key         text    primary key,
    old_key         text    not null,
    old_duration_ms integer not null,
    -- 記録のあとに走査が done で閉じたら 1。0 の間は前の中身が別のパスに現れうるので判定しない。
    ready           integer not null default 0 check (ready in (0, 1)),
    created_at      integer not null
) without rowid;

-- +goose Down
drop table if exists video_successions;
drop table if exists video_version_dismissals;
drop index if exists video_bundle_members_bundle_idx;
drop table if exists video_bundle_members;
drop table if exists video_bundles;
