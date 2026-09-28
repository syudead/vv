-- +goose Up
-- 所有者が設定画面で選ぶ値。設定のデータで、走査では戻らない
-- （specs/025-hardware-encoding/data-model.md）。行が無いキーは「一度も選んでいない」。
create table settings (
    key        text primary key,
    value      text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists settings;
