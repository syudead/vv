-- +goose Up
-- vv 上で動画の情報を最後に編集した日時（specs/033-video-dates/research.md R-1）。
-- 作り直せない利用者データで、video_overrides と同じく内容の識別子に結び、videos への外部キーを
-- 張らない。行が無い動画の更新日時は videos.added_at である。
create table video_edits (
    content_key text    primary key,
    edited_at   integer not null
) without rowid;

-- +goose Down
drop table if exists video_edits;
