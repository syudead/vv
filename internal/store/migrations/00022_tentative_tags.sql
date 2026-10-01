-- +goose Up
-- 仮のタグ（specs/031-tentative-tags/research.md R-1）。自動の付与で新しく作られたタグは 1、
-- 手で作ったタグと導入前のタグは 0。確定は 0 に書き換えるだけで、名前と付与は変えない。
alter table tags add column tentative integer not null default 0 check (tentative in (0, 1));

-- 却下した名前（R-2）。仮のタグを却下したとき、そのタグの元の名前を覚え、以後の仮の作成で
-- 飛ばす。タグでも付与でもない利用者データで、名前の照合は tag_names.name と同じ完全一致。
create table rejected_tag_names (
    name       text    primary key,
    created_at integer not null
) without rowid;

-- +goose Down
drop table if exists rejected_tag_names;
alter table tags drop column tentative;
