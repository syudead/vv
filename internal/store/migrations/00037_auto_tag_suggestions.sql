-- +goose Up
-- Rebuild the table because released databases may already have the tentative column; migrate every existing attachment as confirmed.
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

-- Persistent rejections prevent automatic reattachment on the shared user key and have no video foreign key.
create table if not exists auto_tag_rejections (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    rejected_at integer not null,
    primary key (content_key, tag_id)
) without rowid;

-- +goose Down
drop table if exists auto_tag_rejections;
alter table video_tags drop column tentative;
