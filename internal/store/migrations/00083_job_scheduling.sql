-- +goose Up
-- SQLite cannot change a check or a not null in place, so one rebuild writes the scheduler's end state: every
-- column, kind and subject rule. Later migrations only move their queues' rows in.
create table jobs_new (
    id               integer primary key,
    kind             text    not null check (kind in (
        'probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint', 'keyframes', 'media_info',
        'queue_keyframes', 'queue_media_info', 'virtual_thumbnails',
        'auto_tag', 'face', 'face_image', 'face_cast', 'queue_face_images', 'version_judge')),
    -- Set for kinds that read one video; still deleted with its video.
    video_id         integer references videos (id) on delete cascade,
    state            text    not null check (state in ('queued', 'running', 'done', 'failed')),
    attempts         integer not null default 0,
    last_error       text,
    created_at       integer not null,
    updated_at       integer not null,
    location_id      integer,
    location_version integer,
    location_path    text,
    activity_id      integer,
    force            integer not null default 0,
    force_requested  integer not null default 0,
    -- A content key, a person id, or the first key of a judged pair.
    subject_key      text,
    -- The second key of a judged pair, greater than subject_key.
    subject_key_b    text,
    -- 0 owner request, 1 import, 2 backfill; a lower class is claimed first.
    priority         integer not null default 1 check (priority in (0, 1, 2)),
    -- Unix seconds before which the job is not claimed.
    run_after        integer not null default (unixepoch()),
    check (case
        when kind in ('probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint', 'keyframes', 'media_info',
                      'face_cast')
            then video_id is not null and subject_key is null and subject_key_b is null
        when kind in ('auto_tag', 'face', 'face_image')
            then video_id is null and subject_key is not null and subject_key_b is null
        when kind = 'version_judge'
            then video_id is null and subject_key is not null and subject_key_b is not null
                 and subject_key_b > subject_key
        else video_id is null and subject_key is null and subject_key_b is null
    end)
);
insert into jobs_new (id, kind, video_id, state, attempts, last_error, created_at, updated_at,
                      location_id, location_version, location_path, activity_id, force, force_requested,
                      priority, run_after)
select id, kind, video_id, state, attempts, last_error, created_at, updated_at,
       location_id, location_version, location_path, activity_id, force, force_requested,
       1, created_at
from jobs
where kind <> 'metadata';
drop table jobs;
alter table jobs_new rename to jobs;
-- River and Oban claim in this order: class, run-after time, then age.
create index jobs_claim_idx on jobs (state, priority, run_after, id);
-- One unfinished job per subject; coalescing nulls keeps a singleton kind to one unfinished row.
create unique index jobs_unfinished_subject_idx
    on jobs (kind, coalesce(video_id, 0), coalesce(subject_key, ''), coalesce(subject_key_b, ''))
    where state in ('queued', 'running');

-- +goose Down
create table jobs_old (
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
    location_path text,
    activity_id integer,
    force integer not null default 0,
    force_requested integer not null default 0
);
insert into jobs_old (id, kind, video_id, state, attempts, last_error, created_at, updated_at,
                      location_id, location_version, location_path, activity_id, force, force_requested)
select id, kind, video_id, state, attempts, last_error, created_at, updated_at,
       location_id, location_version, location_path, activity_id, force, force_requested
from jobs
where kind in ('probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint');
drop table jobs;
alter table jobs_old rename to jobs;
create index jobs_state_id_idx on jobs (state, id);
create unique index jobs_pending_kind_video_idx
    on jobs (kind, video_id) where state in ('queued', 'running');
