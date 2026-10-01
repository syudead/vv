-- +goose Up
-- 索引。内容の参照が無くなるときに消し、取り込みで作り直せる。
-- 「同じ動画かもしれない」候補（specs/030-video-versions/research.md R-7、data-model.md §1・§7）。
-- 指紋を書く取引が作り直す。key_a < key_b に並べて 1 行。
create table video_version_candidates (
    key_a      text    not null,
    key_b      text    not null,
    distance   integer not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- +goose Down
drop table if exists video_version_candidates;
