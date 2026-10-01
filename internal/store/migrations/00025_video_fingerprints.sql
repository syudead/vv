-- +goose Up
-- 索引。内容の参照が無くなるときに消し、走査と取り込みで作り直せる。
-- 映像の指紋（specs/030-video-versions/research.md R-6、data-model.md §1・§6）。hashes はコマごとの
-- 9 バイト（印 1 バイト + ハッシュ 8 バイト big endian）。
create table video_fingerprints (
    content_key text    primary key,
    version     integer not null,   -- domain.FingerprintVersion
    interval_ms integer not null,   -- 作ったときのスプライトの配置の間隔。コマの時刻を合わせるのに使う
    hashes      blob    not null,
    updated_at  integer not null
) without rowid;

-- 00014 と同じ手順で jobs を作り直し、kind の検査に 'fingerprint' を足す。SQLite は CHECK を
-- その場で変えられない。行と試行の履歴はそのまま写す。
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

-- 完成したスプライトの動画に指紋の仕事を積む（00015 と同じ積み方）。
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
