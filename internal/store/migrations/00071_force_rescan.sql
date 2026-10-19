-- +goose Up
-- A forced job regenerates its artifact without adopting what is on disk; the stage states stay as they are.
alter table jobs add column force integer not null default 0;
-- A force that arrived while this unforced row was running; finishing the row queues a forced row of its kind.
alter table jobs add column force_requested integer not null default 0;
-- A force rescan is a manual scan with a scope; every column is null for an ordinary scan.
alter table scans add column rescan_scope text check (rescan_scope in ('library', 'folder', 'video'));
-- No foreign keys: a folder cannot change while a scan runs, and a finished scan keeps a deleted video's id as history.
alter table scans add column rescan_folder_id integer;
alter table scans add column rescan_path text;
alter table scans add column rescan_video_id integer;
-- The force pass queued this video's jobs in this scan; a resumed scan copies the marks to skip those videos.
alter table scan_videos add column forced integer not null default 0;
-- Forced rebuilds of each artifact; artifact URLs carry the count so screens load the rebuilt file.
alter table videos add column thumbnail_build integer not null default 0;
alter table videos add column seek_thumbnail_build integer not null default 0;
alter table videos add column preview_build integer not null default 0;

-- +goose Down
alter table videos drop column preview_build;
alter table videos drop column seek_thumbnail_build;
alter table videos drop column thumbnail_build;
alter table scan_videos drop column forced;
alter table scans drop column rescan_video_id;
alter table scans drop column rescan_path;
alter table scans drop column rescan_folder_id;
alter table scans drop column rescan_scope;
alter table jobs drop column force_requested;
alter table jobs drop column force;
