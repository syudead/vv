-- +goose Up
-- Rebuild scans to accept the state paused, since SQLite cannot alter a check constraint. scan_videos and scan_issues
-- cascade from scans and connections enforce foreign keys, so their rows are set aside before the drop and written
-- back after the rename.
create table scan_videos_kept as select * from scan_videos;
create table scan_issues_kept as select * from scan_issues;

create table scans_paused (
    id               integer primary key,
    -- paused: a pause of background work stopped the scan, or it was requested during one; no scanner runs for it.
    state            text    not null check (state in ('running', 'paused', 'done', 'failed')),
    started_at       integer,
    finished_at      integer,
    total            integer not null default 0,
    completed        integer not null default 0,
    failed           integer not null default 0,
    error            text,
    error_code       text,
    error_path       text,
    settled_at       integer,
    issues_revision  integer not null default 0,
    origin           text    not null default 'manual' check (origin in ('manual', 'watch')),
    rescan_scope     text    check (rescan_scope in ('library', 'folder', 'video')),
    rescan_folder_id integer,
    rescan_path      text,
    rescan_video_id  integer
);
insert into scans_paused (id, state, started_at, finished_at, total, completed, failed, error, error_code, error_path,
        settled_at, issues_revision, origin, rescan_scope, rescan_folder_id, rescan_path, rescan_video_id)
    select id, state, started_at, finished_at, total, completed, failed, error, error_code, error_path,
        settled_at, issues_revision, origin, rescan_scope, rescan_folder_id, rescan_path, rescan_video_id
    from scans;
drop table scans;
alter table scans_paused rename to scans;
create unique index scans_single_running_idx on scans (state) where state = 'running';

insert into scan_videos select * from scan_videos_kept;
insert into scan_issues select * from scan_issues_kept;
drop table scan_videos_kept;
drop table scan_issues_kept;

-- The dirty directories of automatic import held through a pause, so a restart during it keeps them.
create table held_watch_directories (
    path      text    not null,
    recursive integer not null check (recursive in (0, 1)),
    primary key (path, recursive)
);

-- +goose Down
drop table held_watch_directories;

create table scan_videos_kept as select * from scan_videos;
create table scan_issues_kept as select * from scan_issues;

create table scans_unpaused (
    id               integer primary key,
    state            text    not null check (state in ('running', 'done', 'failed')),
    started_at       integer,
    finished_at      integer,
    total            integer not null default 0,
    completed        integer not null default 0,
    failed           integer not null default 0,
    error            text,
    error_code       text,
    error_path       text,
    settled_at       integer,
    issues_revision  integer not null default 0,
    origin           text    not null default 'manual' check (origin in ('manual', 'watch')),
    rescan_scope     text    check (rescan_scope in ('library', 'folder', 'video')),
    rescan_folder_id integer,
    rescan_path      text,
    rescan_video_id  integer
);
-- A paused scan goes back as interrupted, which startup resumes.
insert into scans_unpaused (id, state, started_at, finished_at, total, completed, failed, error, error_code,
        error_path, settled_at, issues_revision, origin, rescan_scope, rescan_folder_id, rescan_path, rescan_video_id)
    select id,
        case state when 'paused' then 'failed' else state end,
        started_at,
        case state when 'paused' then coalesce(finished_at, started_at) else finished_at end,
        total, completed, failed,
        case state when 'paused' then 'The application stopped before the scan finished.' else error end,
        case state when 'paused' then 'interrupted' else error_code end,
        case state when 'paused' then null else error_path end,
        settled_at, issues_revision, origin, rescan_scope, rescan_folder_id, rescan_path, rescan_video_id
    from scans;
drop table scans;
alter table scans_unpaused rename to scans;
create unique index scans_single_running_idx on scans (state) where state = 'running';

insert into scan_videos select * from scan_videos_kept;
insert into scan_issues select * from scan_issues_kept;
drop table scan_videos_kept;
drop table scan_issues_kept;
