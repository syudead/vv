-- +goose Up
-- Auto-tagging work moves into jobs; this table keeps only the result per content key, which "tag videos not yet
-- tagged" and the Settings counts read after the job and even the video are gone.
create table auto_tag_results (
    content_key text    primary key,
    state       text    not null check (state in ('done', 'failed')),
    -- English failure detail, nonempty only for failed rows.
    error       text    not null default '',
    -- Unix milliseconds.
    updated_at  integer not null
) without rowid;
insert into auto_tag_results (content_key, state, error, updated_at)
select content_key, state, error, updated_at from auto_tag_queue where state in ('done', 'failed');
-- An interrupted row is queued again, as startup recovery would do.
insert into jobs (kind, subject_key, state, attempts, created_at, updated_at, activity_id, priority, run_after)
select 'auto_tag', content_key, 'queued', 0, queued_at / 1000, updated_at / 1000, activity_id, 1, queued_at / 1000
from auto_tag_queue where state in ('queued', 'running')
order by queued_at, content_key;
drop table auto_tag_queue;

-- +goose Down
create table auto_tag_queue (
    content_key text    primary key,
    state       text    not null check (state in ('queued', 'running', 'done', 'failed')),
    error       text    not null default '',
    queued_at   integer not null,
    updated_at  integer not null,
    activity_id integer
) without rowid;
create index auto_tag_queue_state_idx on auto_tag_queue (state, queued_at);
insert into auto_tag_queue (content_key, state, error, queued_at, updated_at)
select content_key, state, error, updated_at, updated_at from auto_tag_results;
-- Unfinished work replaces a result, as queueing did before.
insert into auto_tag_queue (content_key, state, error, queued_at, updated_at, activity_id)
select subject_key, 'queued', '', created_at * 1000, updated_at * 1000, activity_id
from jobs where kind = 'auto_tag' and state in ('queued', 'running')
on conflict (content_key) do update set state = 'queued', error = '', queued_at = excluded.queued_at,
    updated_at = excluded.updated_at, activity_id = excluded.activity_id;
delete from jobs where kind = 'auto_tag';
drop table auto_tag_results;
