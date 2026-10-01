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

-- +goose Down
drop table if exists video_version_dismissals;
drop index if exists video_bundle_members_bundle_idx;
drop table if exists video_bundle_members;
drop table if exists video_bundles;
