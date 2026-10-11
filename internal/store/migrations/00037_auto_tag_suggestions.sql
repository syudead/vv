-- +goose Up
-- 自動タグ付けの提案（docs/design-docs/auto-tagging.md）。
--
-- video_tags.tentative は、自動タグ付けが確率の中間の帯で仮に付けた付与を 1 にする。手で付けた分と
-- 確定した分は 0 である。この列は一時、適用済みの 00036 を書き換えて足していたので、列を持つ
-- データベースと持たないデータベースがある。どちらでも通るよう、列を足す代わりに表を作り直す。
-- 作り直しでは仮の状態を引き継がず、すべて確定した付与（0）になる。
create table video_tags_new (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    created_at  integer not null,
    tentative   integer not null default 0 check (tentative in (0, 1)),
    primary key (content_key, tag_id)
) without rowid;
insert into video_tags_new (content_key, tag_id, created_at)
select content_key, tag_id, created_at from video_tags;
drop table video_tags;
alter table video_tags_new rename to video_tags;
create index video_tags_tag_idx on video_tags (tag_id, content_key);

-- 所有者が却下した提案。鍵は video_tags と同じ利用者データの鍵で、この動画にこのタグを自動タグ付けで
-- 付け直さない。作り直せない利用者データで、videos への外部キーを張らない。
create table if not exists auto_tag_rejections (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    rejected_at integer not null,
    primary key (content_key, tag_id)
) without rowid;

-- +goose Down
drop table if exists auto_tag_rejections;
alter table video_tags drop column tentative;
