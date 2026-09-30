-- +goose Up
-- 所有者が付けた表示名と代表サムネイルの位置（specs/029-video-overrides/research.md R-1）。
-- 作り直せない利用者データで、playback_progress・video_tags・public_videos と同じく
-- 内容の識別子に結び、videos への外部キーを張らない。
create table video_overrides (
    content_key           text    primary key,
    -- domain.NormalizeDisplayName を通した表示名。未設定は null（空文字は置かない）。
    display_name          text,
    -- 代表サムネイルにする場面の位置（ミリ秒）。未設定は null。
    thumbnail_position_ms integer check (thumbnail_position_ms is null or thumbnail_position_ms >= 0),
    -- 位置を記録するたびに書く改版番号（R-6）。thumbnailUrl の版に使い、位置の値を URL に出さない。
    -- 位置が null のときは null。
    thumbnail_revision    integer,
    updated_at            integer not null
) without rowid;

-- +goose Down
drop table if exists video_overrides;
