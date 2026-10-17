-- +goose Up
-- 所有者が代表サムネイルとして適用した画像。作り直せない利用者データで、video_overrides と同じく内容の識別子に結び、
-- videos への外部キーを張らない。代表サムネイルのファイルは、この行から作り直す。
create table thumbnail_images (
    content_key text    primary key,
    -- 動画の表示上の縦横比に合わせた JPEG（R-7）。公開したサムネイルのファイルと同じバイト列。
    image       blob    not null,
    -- 適用するたびに書く改版番号。video_overrides.thumbnail_revision と同じ規則で、thumbnailUrl の
    -- 版に使う。
    revision    integer not null,
    -- 適用した時刻（Unix ミリ秒）。
    applied_at  integer not null
) without rowid;

-- +goose Down
drop table if exists thumbnail_images;
