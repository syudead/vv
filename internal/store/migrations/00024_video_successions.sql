-- +goose Up
-- 索引。内容の参照が無くなるときに消し、走査と取り込みで作り直せる。
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
