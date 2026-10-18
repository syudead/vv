-- +goose Up
-- Tags are persistent user data without video foreign keys.

-- AUTOINCREMENT prevents stale URLs or tabs from resolving deleted IDs to unrelated new tags.
create table tags (
    id         integer primary key autoincrement,
    created_at integer not null
);

-- Names and synonyms share one namespace, compared by exact BINARY spelling.
create table tag_names (
    name      text    primary key,
    tag_id    integer not null references tags (id) on delete cascade,
    -- One denotes the canonical display name; zero denotes a synonym.
    canonical integer not null check (canonical in (0, 1)),
    -- Go writes domain.FoldForMatch(name).
    search_key     text    not null default '',
    -- The domain.SearchKeyVersion used to derive this key.
    search_version integer not null default 0
) without rowid;

create unique index tag_names_canonical_idx on tag_names (tag_id) where canonical = 1;
create index tag_names_tag_idx on tag_names (tag_id);

-- Persistent attachments intentionally have no video foreign key.
create table video_tags (
    content_key text    not null,
    tag_id      integer not null references tags (id) on delete cascade,
    created_at  integer not null,
    primary key (content_key, tag_id)
) without rowid;

create index video_tags_tag_idx on video_tags (tag_id, content_key);

-- +goose Down
drop table if exists video_tags;
drop table if exists tag_names;
drop table if exists tags;
