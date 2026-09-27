-- +goose Up
-- シーク用サムネイルを代表サムネイルから独立した段階にする
-- （specs/020-seek-thumbnail-stage/data-model.md §5）。
alter table videos add column seek_thumbnail_state text not null default 'pending'
    check (seek_thumbnail_state in ('pending', 'done', 'failed'));

-- SQLite cannot alter a CHECK constraint in place. Rebuild the small queue table
-- while preserving existing jobs and their retry history.
alter table jobs rename to jobs_old;
create table jobs (
    id integer primary key,
    kind text not null check (kind in ('probe', 'thumbnail', 'seek_thumbnail', 'preview')),
    video_id integer not null references videos (id) on delete cascade,
    state text not null check (state in ('queued', 'running', 'done', 'failed')),
    attempts integer not null default 0,
    last_error text,
    created_at integer not null,
    updated_at integer not null,
    location_id integer,
    location_version integer,
    location_path text
);
insert into jobs (id, kind, video_id, state, attempts, last_error, created_at, updated_at,
                 location_id, location_version, location_path)
select id, kind, video_id, state, attempts, last_error, created_at, updated_at,
       location_id, location_version, location_path
from jobs_old;
drop table jobs_old;
create index jobs_state_id_idx on jobs (state, id);
create unique index jobs_pending_kind_video_idx
    on jobs (kind, video_id) where state in ('queued', 'running');

-- thumbnail_state = done はシーク用の完成を意味しないので、既存の動画は全部
-- pending のまま積む。完成済みのものは置き場の確認だけで終わる。
insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
select 'seek_thumbnail', id, 'queued', 0, unixepoch(), unixepoch()
from videos where probe_state <> 'pending'
  and exists (select 1 from video_locations where video_id = videos.id);

-- +goose Down
delete from jobs where kind = 'seek_thumbnail';
alter table jobs rename to jobs_new;
create table jobs (
    id integer primary key,
    kind text not null check (kind in ('probe', 'thumbnail', 'preview')),
    video_id integer not null references videos (id) on delete cascade,
    state text not null check (state in ('queued', 'running', 'done', 'failed')),
    attempts integer not null default 0,
    last_error text,
    created_at integer not null,
    updated_at integer not null,
    location_id integer,
    location_version integer,
    location_path text
);
insert into jobs select * from jobs_new where kind <> 'seek_thumbnail';
drop table jobs_new;
create index jobs_state_id_idx on jobs (state, id);
create unique index jobs_pending_kind_video_idx
    on jobs (kind, video_id) where state in ('queued', 'running');
alter table videos drop column seek_thumbnail_state;
