-- +goose Up
-- 動画の作品情報と、人物・団体の実体（specs/047-video-work-info/data-model.md「Migration」、
-- research.md R-1・R-2・R-3）。12 表はすべて利用者データで、videos への外部キーを持たない。

-- 実体（R-3）。autoincrement で、消した id を使い回さない。
create table people (
    id         integer primary key autoincrement,
    created_at integer not null
);
create table organizations (
    id         integer primary key autoincrement,
    parent_id  integer references organizations (id) on delete set null,
    created_at integer not null
);

-- 名前と別名。実体ごとに綴り 1 つにつき 1 行で、canonical = 1 が表示する名前。
-- 2 つの実体が同じ綴りを持ってよい（同名の別人）。
create table people_names (
    person_id      integer not null references people (id) on delete cascade,
    name           text    not null,
    canonical      integer not null check (canonical in (0, 1)),
    language       text,                            -- 別名だけ。無ければ null
    search_key     text    not null default '',     -- domain.FoldForMatch(name)
    sort_key       text    not null default '',     -- domain.NaturalSortKey(name)
    search_version integer not null default 0,
    primary key (person_id, name)
) without rowid;
create unique index people_names_canonical_idx on people_names (person_id) where canonical = 1;
create table organization_names (
    organization_id integer not null references organizations (id) on delete cascade,
    name            text    not null,
    canonical       integer not null check (canonical in (0, 1)),
    language        text,
    search_key      text    not null default '',
    sort_key        text    not null default '',
    search_version  integer not null default 0,
    primary key (organization_id, name)
) without rowid;
create unique index organization_names_canonical_idx on organization_names (organization_id) where canonical = 1;

create table people_external_ids (
    person_id integer not null references people (id) on delete cascade,
    source    text    not null,
    value     text    not null,
    primary key (person_id, source, value)
) without rowid;
-- 外部 ID は種類ごとに 1 つの実体を指すので、Lookup の答えは 1 つになる。
create unique index people_external_ids_lookup_idx on people_external_ids (source, value);
create table organization_external_ids (
    organization_id integer not null references organizations (id) on delete cascade,
    source          text    not null,
    value           text    not null,
    primary key (organization_id, source, value)
) without rowid;
create unique index organization_external_ids_lookup_idx on organization_external_ids (source, value);

-- 動画ごとの表（R-2）。content_key は利用者データの鍵。videos への外部キーも表どうしの外部キーも
-- 持たないので、束ねの経路は表ごとに写し、引き継ぎは 6 表をまとめて動かす。
create table video_work_info (
    content_key    text    primary key,
    code           text,
    code_key       text    not null default '',     -- domain.FoldForMatch(code)
    description    text,
    search_version integer not null default 0
) without rowid;
create table video_work_dates (
    content_key text not null,
    kind        text not null check (kind in ('release', 'recorded', 'published')),
    date        text not null,                      -- YYYY・YYYY-MM・YYYY-MM-DD
    primary key (content_key, kind)
) without rowid;
create table video_work_urls (
    content_key text    not null,
    position    integer not null,
    url         text    not null,
    primary key (content_key, position),
    unique (content_key, url)
) without rowid;
create table video_work_external_ids (
    content_key text not null,
    source      text not null,
    value       text not null,
    primary key (content_key, source, value)
) without rowid;
create table video_work_people (
    content_key text    not null,
    role        text    not null check (role in ('performer', 'director', 'photographer', 'streamer')),
    position    integer not null,
    person_id   integer not null references people (id) on delete cascade,
    primary key (content_key, role, position),
    unique (content_key, role, person_id)
) without rowid;
create index video_work_people_person_idx on video_work_people (person_id, content_key);
create table video_work_organizations (
    content_key     text    not null,
    role            text    not null check (role in ('maker', 'label')),
    position        integer not null,
    organization_id integer not null references organizations (id) on delete cascade,
    primary key (content_key, role, position),
    unique (content_key, role, organization_id)
) without rowid;
create index video_work_organizations_org_idx on video_work_organizations (organization_id, content_key);

-- +goose Down
drop index if exists video_work_organizations_org_idx;
drop table if exists video_work_organizations;
drop index if exists video_work_people_person_idx;
drop table if exists video_work_people;
drop table if exists video_work_external_ids;
drop table if exists video_work_urls;
drop table if exists video_work_dates;
drop table if exists video_work_info;
drop index if exists organization_external_ids_lookup_idx;
drop table if exists organization_external_ids;
drop index if exists people_external_ids_lookup_idx;
drop table if exists people_external_ids;
drop index if exists organization_names_canonical_idx;
drop table if exists organization_names;
drop index if exists people_names_canonical_idx;
drop table if exists people_names;
drop table if exists organizations;
drop table if exists people;
