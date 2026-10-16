-- +goose Up
-- タグの種類（specs/051-tag-kinds/data-model.md、research.md R-1・R-2）。種類は利用者データで、
-- 再スキャンで作り直さない。タグは tags.kind_id で種類を 0 か 1 つ持つ。

-- autoincrement で、消した種類の id を使い回さない（tags.id と同じ）。name はタグの名前とは別の
-- 名前空間で、domain.NormalizeTagName で整えた形を保存する（R-6）。
create table tag_kinds (
    id         integer primary key autoincrement,
    name       text    not null unique,
    created_at integer not null
);

-- プリセットの種類（R-2）。この id の順に入る。あとは普通の種類で、名前の変更も削除もできる。
insert into tag_kinds (name, created_at) values ('Genre', unixepoch());
insert into tag_kinds (name, created_at) values ('Work', unixepoch());
insert into tag_kinds (name, created_at) values ('Place', unixepoch());
insert into tag_kinds (name, created_at) values ('Character', unixepoch());

-- null は種類の無いタグ。種類を消すと、その種類のタグは種類なしで残る（要件 4）。
alter table tags add column kind_id integer references tag_kinds (id) on delete set null;

create index tags_kind_id on tags (kind_id);

-- +goose Down
drop index if exists tags_kind_id;
alter table tags drop column kind_id;
drop table if exists tag_kinds;
