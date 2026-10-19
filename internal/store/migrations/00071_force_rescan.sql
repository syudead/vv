-- +goose Up
-- A force rescan is a manual scan with a scope; every column is null for an ordinary scan.
alter table scans add column rescan_scope text check (rescan_scope in ('library', 'folder', 'video'));
-- No foreign keys: a folder cannot change while a scan runs, and a finished scan keeps a deleted video's id as history.
alter table scans add column rescan_folder_id integer;
alter table scans add column rescan_path text;
alter table scans add column rescan_video_id integer;
-- The force pass queued this video's jobs in this scan; a resumed scan copies the marks to skip those videos.
alter table scan_videos add column forced integer not null default 0;

-- +goose Down
alter table scan_videos drop column forced;
alter table scans drop column rescan_video_id;
alter table scans drop column rescan_path;
alter table scans drop column rescan_folder_id;
alter table scans drop column rescan_scope;
