-- +goose Up
-- Rebuildable same-path succession candidates are consumed after a successful scan and replacement probing.
create table video_successions (
    new_key         text    primary key,
    old_key         text    not null,
    old_duration_ms integer not null,
    -- Only successful scan completion makes succession ready; old content may reappear elsewhere before then.
    ready           integer not null default 0 check (ready in (0, 1)),
    created_at      integer not null
) without rowid;

-- +goose Down
drop table if exists video_successions;
