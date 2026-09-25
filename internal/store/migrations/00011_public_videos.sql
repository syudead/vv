-- +goose Up
-- 公開フラグ（specs/016-single-account-auth/data-model.md §1）。行があれば公開、
-- 無ければ非公開（既定）である。動画の行ではなく内容の識別子に結ぶので、再スキャン・
-- 移動・改名・メディアフォルダの変更で失われない。playback_progress・video_tags と
-- 同じく、videos への外部キーは張らない（ARCHITECTURE.md「Two kinds of data」）。
create table public_videos (
    content_key  text    primary key,
    published_at integer not null
) without rowid;

-- +goose Down
drop table if exists public_videos;
