-- +goose Up
-- 取り込み済みの動画を解析し直し、追加の情報（video_media_info）を埋める仕事
-- （specs/056-media-info/research.md R-5、data-model.md「Migration」）。
--
-- 00025 と同じ手順で jobs を作り直し、kind の検査に 'metadata' を足す。SQLite は CHECK を
-- その場で変えられない。行と試行の履歴はそのまま写す。
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

-- 解析が done で、登録したメディアフォルダの下に所在がある動画に解析し直しの仕事を積む。
-- 登録フォルダの下かは 00018 と同じく OS に依らない条件で判定する。待ちか実行中の仕事が
-- 既にある動画は飛ばす。
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
