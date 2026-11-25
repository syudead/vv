-- +goose Up
-- Rebuildable keyframe hashes for image search, released with the content like video_fingerprints.
create table video_search_index (
    content_key text    primary key,
    version     integer not null,   -- domain.SearchIndexVersion
    -- Twelve bytes per kept keyframe in time order: a four-byte big-endian time in milliseconds, then the
    -- eight-byte big-endian hash.
    entries     blob    not null,
    updated_at  integer not null
) without rowid;

-- SQLite cannot change a check in place, so jobs is rebuilt as 00084 built it, with the two search index kinds. The
-- candidate trigger names jobs in its body, so it is dropped across the rename and created again.
drop trigger video_version_candidates_job_delete;
create table jobs_new (
    id               integer primary key,
    kind             text    not null check (kind in (
        'probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint', 'keyframes', 'media_info',
        'queue_keyframes', 'queue_media_info', 'virtual_thumbnails',
        'auto_tag', 'face', 'face_image', 'face_cast', 'queue_face_images', 'version_judge',
        'frame_index', 'queue_frame_index')),
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
                      'face_cast', 'frame_index')
            then video_id is not null and subject_key is null and subject_key_b is null
        when kind in ('auto_tag', 'face', 'face_image')
            then video_id is null and subject_key is not null and subject_key_b is null
        when kind = 'version_judge'
            then video_id is null and subject_key is not null and subject_key_b is not null
                 and subject_key_b > subject_key
        else video_id is null and subject_key is null and subject_key_b is null
    end)
);
insert into jobs_new select * from jobs;
drop table jobs;
alter table jobs_new rename to jobs;
create index jobs_claim_idx on jobs (state, priority, run_after, id);
create unique index jobs_unfinished_subject_idx
    on jobs (kind, coalesce(video_id, 0), coalesce(subject_key, ''), coalesce(subject_key_b, ''))
    where state in ('queued', 'running');
-- +goose StatementBegin
create trigger video_version_candidates_job_delete after delete on video_version_candidates
begin
    delete from jobs where kind = 'version_judge' and subject_key = old.key_a and subject_key_b = old.key_b;
end;
-- +goose StatementEnd

-- +goose Down
drop trigger video_version_candidates_job_delete;
create table jobs_old (
    id               integer primary key,
    kind             text    not null check (kind in (
        'probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint', 'keyframes', 'media_info',
        'queue_keyframes', 'queue_media_info', 'virtual_thumbnails',
        'auto_tag', 'face', 'face_image', 'face_cast', 'queue_face_images', 'version_judge')),
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
    subject_key      text,
    subject_key_b    text,
    priority         integer not null default 1 check (priority in (0, 1, 2)),
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
insert into jobs_old select * from jobs where kind not in ('frame_index', 'queue_frame_index');
drop table jobs;
alter table jobs_old rename to jobs;
create index jobs_claim_idx on jobs (state, priority, run_after, id);
create unique index jobs_unfinished_subject_idx
    on jobs (kind, coalesce(video_id, 0), coalesce(subject_key, ''), coalesce(subject_key_b, ''))
    where state in ('queued', 'running');
-- +goose StatementBegin
create trigger video_version_candidates_job_delete after delete on video_version_candidates
begin
    delete from jobs where kind = 'version_judge' and subject_key = old.key_a and subject_key_b = old.key_b;
end;
-- +goose StatementEnd
drop table video_search_index;
