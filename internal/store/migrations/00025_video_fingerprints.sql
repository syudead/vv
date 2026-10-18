-- +goose Up
-- Rebuildable fingerprints store nine bytes per frame: one marker and an eight-byte big-endian hash.
create table video_fingerprints (
    content_key text    primary key,
    version     integer not null,   -- domain.FingerprintVersion
    interval_ms integer not null,   -- Original sprite interval for matching frame timestamps.
    hashes      blob    not null,
    updated_at  integer not null
) without rowid;

-- SQLite cannot alter CHECK constraints in place; rebuild jobs while preserving rows and attempts.
alter table jobs rename to jobs_old;
create table jobs (
    id integer primary key,
    kind text not null check (kind in ('probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint')),
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

insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
select 'fingerprint', id, 'queued', 0, unixepoch(), unixepoch()
from videos
where seek_thumbnail_state = 'done'
  and exists (select 1 from video_locations where video_id = videos.id)
  and not exists (
      select 1 from jobs
      where jobs.kind = 'fingerprint' and jobs.video_id = videos.id
        and jobs.state in ('queued', 'running'));

-- +goose Down
delete from jobs where kind = 'fingerprint';
alter table jobs rename to jobs_new;
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
insert into jobs select * from jobs_new;
drop table jobs_new;
create index jobs_state_id_idx on jobs (state, id);
create unique index jobs_pending_kind_video_idx
    on jobs (kind, video_id) where state in ('queued', 'running');
drop table if exists video_fingerprints;
