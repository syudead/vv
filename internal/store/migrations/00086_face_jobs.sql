-- +goose Up
-- Face analysis work moves into jobs; this table keeps only the result per content key, which "Analyse missing" and
-- the Settings counts read after the job and even the video are gone.
create table face_results (
    content_key text    primary key,
    state       text    not null check (state in ('done', 'failed')),
    -- The recognizer of the last finished analysis.
    recognizer  text,
    -- The helper's or ffmpeg's message, only for failed rows.
    reason      text,
    -- Unix milliseconds.
    updated_at  integer not null
) without rowid;
insert into face_results (content_key, state, recognizer, reason, updated_at)
select content_key, state, recognizer, reason, updated_at from face_queue where state in ('done', 'failed');
-- An interrupted row is queued again, as startup recovery would do.
insert into jobs (kind, subject_key, state, attempts, created_at, updated_at, activity_id, priority, run_after)
select 'face', content_key, 'queued', 0, updated_at / 1000, updated_at / 1000, activity_id, 1, updated_at / 1000
from face_queue where state in ('queued', 'running')
order by updated_at, content_key;
drop table face_queue;
-- The face loop found people whose face had no current vector; one job queues them now, when any person has a face.
insert into jobs (kind, state, attempts, created_at, updated_at, priority, run_after)
select 'queue_face_images', 'queued', 0, unixepoch(), unixepoch(), 1, unixepoch()
where exists (select 1 from person_images);

-- +goose Down
create table face_queue (
    content_key text    primary key,
    state       text    not null check (state in ('queued', 'running', 'done', 'failed')),
    recognizer  text,
    reason      text,
    activity_id integer,
    updated_at  integer not null
) without rowid;
create index face_queue_state_idx on face_queue (state, updated_at);
insert into face_queue (content_key, state, recognizer, reason, updated_at)
select content_key, state, recognizer, reason, updated_at from face_results;
-- Unfinished work replaces a result, as queueing did before.
insert into face_queue (content_key, state, updated_at, activity_id)
select subject_key, 'queued', created_at * 1000, activity_id
from jobs where kind = 'face' and state in ('queued', 'running')
on conflict (content_key) do update set state = 'queued', reason = null, updated_at = excluded.updated_at,
    activity_id = excluded.activity_id;
delete from jobs where kind in ('face', 'face_image', 'face_cast', 'queue_face_images');
drop table face_results;
