-- +goose Up
-- Retain completed content-keyed judgements to avoid reapplying tags removed by the owner; queue loss requires rejudging, not user-data loss.
create table auto_tag_queue (
    content_key text    primary key,
    state       text    not null check (state in ('queued', 'running', 'done', 'failed')),
    -- English failure detail, nonempty only for failed rows.
    error       text    not null default '',
    -- Queue time in Unix milliseconds.
    queued_at   integer not null,
    updated_at  integer not null
) without rowid;

create index auto_tag_queue_state_idx on auto_tag_queue (state, queued_at);

-- +goose Down
drop table if exists auto_tag_queue;
