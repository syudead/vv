-- +goose Up
-- Existing representative thumbnails predate the seek cache. Requeue their
-- thumbnail work once so the background worker builds both artifacts.
delete from jobs where kind = 'thumbnail';
insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
select 'thumbnail', id, 'queued', 0, unixepoch(), unixepoch() from videos order by id desc;

-- +goose Down
-- Generated media is rebuildable and the previous per-request implementation
-- had no persistent state to restore.
delete from jobs where kind = 'thumbnail' and state = 'queued';
