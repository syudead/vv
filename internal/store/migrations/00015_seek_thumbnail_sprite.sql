-- +goose Up
-- Rebuild completed seek thumbnails as sprites, preserving other stages; artifacts removes old JPEGs after publication.
insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
select 'seek_thumbnail', id, 'queued', 0, unixepoch(), unixepoch()
from videos
where seek_thumbnail_state = 'done'
  and probe_state <> 'pending'
  and exists (select 1 from video_locations where video_id = videos.id)
  and not exists (
      select 1 from jobs
      where jobs.kind = 'seek_thumbnail' and jobs.video_id = videos.id
        and jobs.state in ('queued', 'running'));

update videos set seek_thumbnail_state = 'pending' where seek_thumbnail_state = 'done';

-- +goose Down
-- Keep rebuild jobs on rollback: old JPEGs may already be deleted, so restoring done would claim missing artifacts exist.
select 1;
