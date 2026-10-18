-- +goose Up
-- SQLite cannot alter CHECK in place; rebuild jobs with metadata support while preserving rows and attempts.
alter table jobs rename to jobs_old;
create table jobs (
    id integer primary key,
    kind text not null check (kind in ('probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint', 'metadata')),
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

-- Queue completed registered videos using OS-independent registration checks, skipping existing unfinished jobs.
insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
select 'metadata', id, 'queued', 0, unixepoch(), unixepoch()
from videos
where probe_state = 'done'
  and exists (
      select 1 from video_locations l join media_folders mf
       where l.video_id = videos.id
         and (l.path = mf.path
              or instr(l.path, rtrim(mf.path, '/' || char(92)) || '/') = 1
              or instr(l.path, rtrim(mf.path, '/' || char(92)) || char(92)) = 1))
  and not exists (
      select 1 from jobs
      where jobs.kind = 'metadata' and jobs.video_id = videos.id
        and jobs.state in ('queued', 'running'));

-- +goose Down
delete from jobs where kind = 'metadata';
alter table jobs rename to jobs_new;
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
insert into jobs select * from jobs_new;
drop table jobs_new;
create index jobs_state_id_idx on jobs (state, id);
create unique index jobs_pending_kind_video_idx
    on jobs (kind, video_id) where state in ('queued', 'running');
