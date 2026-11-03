-- +goose Up
-- SQLite cannot alter a CHECK in place; rebuild assistant_review_runs to allow the uselessTags kind. The candidates
-- reference the runs with on delete cascade, so they wait in a plain table while the runs are rebuilt.
create table assistant_review_candidates_keep as select * from assistant_review_candidates;
drop table assistant_review_candidates;

create table assistant_review_runs_new (
    id          integer primary key autoincrement,
    kind        text    not null check (kind in ('tags', 'people', 'works', 'uselessTags')),
    request     text    not null,
    status      text    not null check (status in ('running', 'done', 'failed', 'interrupted')),
    total       integer not null default 0,
    judged      integer not null default 0,
    listed      integer not null default 0,
    error       text,
    -- Unix milliseconds.
    started_at  integer not null,
    finished_at integer
);
insert into assistant_review_runs_new select * from assistant_review_runs;
drop table assistant_review_runs;
alter table assistant_review_runs_new rename to assistant_review_runs;
create unique index assistant_review_runs_running_idx on assistant_review_runs (kind) where status = 'running';

-- Sets judged at or above the listing threshold. Members are tags and people by id and videos by user key, with no
-- foreign key, so a member deleted or merged later is left out when the list is read.
create table assistant_review_candidates (
    id          integer primary key autoincrement,
    run_id      integer not null references assistant_review_runs (id) on delete cascade,
    -- [{kind, id, key, name}] as JSON.
    members     text    not null,
    set_key     text    not null,
    probability real    not null,
    state       text    not null check (state in ('listed', 'adopted', 'rejected')),
    proposal_id integer
);
insert into assistant_review_candidates select * from assistant_review_candidates_keep;
drop table assistant_review_candidates_keep;
create index assistant_review_candidates_run_idx on assistant_review_candidates (run_id, probability desc, id);

-- +goose Down
delete from assistant_review_runs where kind = 'uselessTags';

create table assistant_review_candidates_keep as select * from assistant_review_candidates;
drop table assistant_review_candidates;

create table assistant_review_runs_old (
    id          integer primary key autoincrement,
    kind        text    not null check (kind in ('tags', 'people', 'works')),
    request     text    not null,
    status      text    not null check (status in ('running', 'done', 'failed', 'interrupted')),
    total       integer not null default 0,
    judged      integer not null default 0,
    listed      integer not null default 0,
    error       text,
    started_at  integer not null,
    finished_at integer
);
insert into assistant_review_runs_old select * from assistant_review_runs;
drop table assistant_review_runs;
alter table assistant_review_runs_old rename to assistant_review_runs;
create unique index assistant_review_runs_running_idx on assistant_review_runs (kind) where status = 'running';

create table assistant_review_candidates (
    id          integer primary key autoincrement,
    run_id      integer not null references assistant_review_runs (id) on delete cascade,
    members     text    not null,
    set_key     text    not null,
    probability real    not null,
    state       text    not null check (state in ('listed', 'adopted', 'rejected')),
    proposal_id integer
);
insert into assistant_review_candidates
    select * from assistant_review_candidates_keep where run_id in (select id from assistant_review_runs);
drop table assistant_review_candidates_keep;
create index assistant_review_candidates_run_idx on assistant_review_candidates (run_id, probability desc, id);
delete from assistant_review_dismissals where kind = 'uselessTags';
