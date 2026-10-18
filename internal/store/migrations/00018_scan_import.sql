-- +goose Up
-- Scan targets and completion are rebuildable; refreshScanSettled is the sole writer of settled_at in Unix seconds.
alter table scans add column settled_at integer;
alter table scans add column issues_revision integer not null default 0;

-- Queue transactions add targets, new scans replace the set, and deleted videos cascade out.
create table scan_videos (
    scan_id  integer not null references scans(id) on delete cascade,
    video_id integer not null references videos(id) on delete cascade,
    primary key (scan_id, video_id)
);
create index scan_videos_video_idx on scan_videos (video_id);

-- Accept both separator styles during migration; startup recomputes completion using the runtime OS registration rules.
insert or ignore into scan_videos (scan_id, video_id)
select (select max(id) from scans), j.video_id
  from jobs j
 where (select max(id) from scans) is not null
   and j.state in ('queued', 'running')
   and exists (
       select 1 from video_locations l join media_folders mf
        where l.video_id = j.video_id
          and (l.path = mf.path
               or instr(l.path, rtrim(mf.path, '/' || char(92)) || '/') = 1
               or instr(l.path, rtrim(mf.path, '/' || char(92)) || char(92)) = 1));


-- Closed scans settle at finish time unless the latest scan still has eligible work.
update scans
   set settled_at = finished_at
 where state <> 'running'
   and not (id = (select max(id) from scans) and exists (select 1 from scan_videos where scan_id = scans.id));

-- +goose Down
drop index if exists scan_videos_video_idx;
drop table if exists scan_videos;
alter table scans drop column issues_revision;
alter table scans drop column settled_at;
