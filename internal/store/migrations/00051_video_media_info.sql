-- +goose Up
-- 取り込みの解析で読む動画の追加の情報。
-- 索引であり、消えても走査とそれが積む仕事で埋まる。
create table video_media_info (
    video_id   integer primary key references videos (id) on delete cascade,
    -- domain.MediaInfoVersion。違えば「無い」と読む。
    version    integer not null,
    -- domain.MediaInfo の JSON。
    info       text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists video_media_info;
