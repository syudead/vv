-- +goose Up
-- Playback progress is persistent user data keyed by content, while videos, jobs and scans are rebuildable indexes.

-- Legacy rows may initially have empty content keys; the next scan fills them.
alter table videos add column content_key       text    not null default '';
-- Unknown duration stays NULL, never zero.
alter table videos add column duration_ms       integer;
alter table videos add column width             integer;
alter table videos add column height            integer;
alter table videos add column container         text;
alter table videos add column video_codec       text;
-- NULL when there is no audio.
alter table videos add column audio_codec       text;
-- Only completed probes may mark a video playable.
alter table videos add column playable          integer not null default 0;
alter table videos add column unplayable_reason text;
-- pending / done / failed。
alter table videos add column probe_state       text    not null default 'pending';
alter table videos add column probe_error       text;
-- pending / done / failed。
alter table videos add column thumbnail_state   text    not null default 'pending';
alter table videos add column updated_at        integer not null default 0;

-- Include ID as the keyset-pagination tie breaker.
create index videos_added_at_desc_idx on videos (added_at desc, id desc);
create index videos_title_asc_idx     on videos (title asc, id asc);
-- Enforce unique identified content while allowing legacy empty keys during migration.
create unique index videos_content_key_idx on videos (content_key) where content_key <> '';

-- No video foreign key: playback progress survives missing files and returns when the same content is restored.
create table playback_progress (
    content_key text    primary key,
    position_ms integer not null check (position_ms >= 0),
    -- Duration known at save time, used to recalculate completion.
    duration_ms integer,
    completed   integer not null default 0 check (completed in (0, 1)),
    updated_at  integer not null
);

-- Persistent queues and in-process workers avoid a separate queue service.
create table jobs (
    id         integer primary key,
    kind       text    not null check (kind in ('probe', 'thumbnail')),
    -- Rebuildable job rows cascade with their video.
    video_id   integer not null references videos (id) on delete cascade,
    state      text    not null check (state in ('queued', 'running', 'done', 'failed')),
    attempts   integer not null default 0,
    last_error text,
    created_at integer not null,
    updated_at integer not null
);

create index jobs_state_id_idx on jobs (state, id);
-- At most one unfinished job per target prevents repeated scans from growing the queue.
create unique index jobs_pending_kind_video_idx
    on jobs (kind, video_id)
    where state in ('queued', 'running');

create table scans (
    id          integer primary key,
    state       text    not null check (state in ('running', 'done', 'failed')),
    started_at  integer,
    finished_at integer,
    total       integer not null default 0,
    completed   integer not null default 0,
    failed      integer not null default 0,
    error       text
);

-- The single-running-scan constraint supports reusing the current scan for repeated requests.
create unique index scans_single_running_idx on scans (state) where state = 'running';

-- +goose Down
drop index if exists scans_single_running_idx;
drop table if exists scans;
drop index if exists jobs_pending_kind_video_idx;
drop index if exists jobs_state_id_idx;
drop table if exists jobs;
drop table if exists playback_progress;
drop index if exists videos_content_key_idx;
drop index if exists videos_title_asc_idx;
drop index if exists videos_added_at_desc_idx;

-- SQLite cannot drop indexed or trigger-referenced columns; existing FTS triggers reference only retained title/path columns.
alter table videos drop column updated_at;
alter table videos drop column thumbnail_state;
alter table videos drop column probe_error;
alter table videos drop column probe_state;
alter table videos drop column unplayable_reason;
alter table videos drop column playable;
alter table videos drop column audio_codec;
alter table videos drop column video_codec;
alter table videos drop column container;
alter table videos drop column height;
alter table videos drop column width;
alter table videos drop column duration_ms;
alter table videos drop column content_key;
