-- +goose Up
-- Write proposals of the assistant. Videos are named by user key inside plan, with no foreign key to videos, so a
-- rebuild keeps them; activity_id refers to the assistant_run entry without a foreign key, as queue rows do.
create table assistant_proposals (
    id          integer primary key autoincrement,
    status      text    not null check (status in
        ('proposed', 'approved', 'running', 'done', 'partial', 'stale', 'failed', 'discarded')),
    request     text    not null,
    -- domain.AssistantPlan as JSON.
    plan        text    not null,
    total       integer not null default 0,
    applied     integer not null default 0,
    failed      integer not null default 0,
    -- domain.AssistantProposalError as JSON.
    error       text,
    activity_id integer,
    -- Unix milliseconds.
    created_at  integer not null,
    approved_at integer,
    started_at  integer,
    finished_at integer
);
create index assistant_proposals_status_idx on assistant_proposals (status, id);

-- SQLite cannot alter a CHECK in place; rebuild activity_entries to allow proposal subjects, keeping every row.
create table activity_entries_new (
    id              integer primary key,
    kind            text    not null,
    feature         text    not null,
    subject_kind    text    not null check (subject_kind in ('video', 'pair', 'scan', 'folder', 'person', 'proposal')),
    subject_key     text    not null,
    subject_key_b   text,
    subject_label   text    not null,
    subject_label_b text,
    status          text    not null check (status in ('running', 'attention', 'done')),
    reason          text,
    outcome         text,
    detail          text,
    stage           text,
    undoes_id       integer,
    decider         text    not null check (decider in ('rule', 'ai', 'owner')),
    probability     real,
    undo_keys       text,
    undone_by       integer,
    tag_id          integer,
    decided_mark    integer,
    created_at      integer not null,
    updated_at      integer not null,
    scan_id         integer,
    superseded_by   integer,
    evidence        text
);
insert into activity_entries_new
select id, kind, feature, subject_kind, subject_key, subject_key_b, subject_label, subject_label_b, status, reason,
       outcome, detail, stage, undoes_id, decider, probability, undo_keys, undone_by, tag_id, decided_mark,
       created_at, updated_at, scan_id, superseded_by, evidence
from activity_entries;
drop table activity_entries;
alter table activity_entries_new rename to activity_entries;
create index activity_entries_status_idx on activity_entries (status, id);
create index activity_entries_feature_idx on activity_entries (feature, id);
create index activity_entries_subject_idx on activity_entries (kind, subject_key, subject_key_b, id);

-- +goose Down
delete from activity_entries where subject_kind = 'proposal';
create table activity_entries_old (
    id              integer primary key,
    kind            text    not null,
    feature         text    not null,
    subject_kind    text    not null check (subject_kind in ('video', 'pair', 'scan', 'folder', 'person')),
    subject_key     text    not null,
    subject_key_b   text,
    subject_label   text    not null,
    subject_label_b text,
    status          text    not null check (status in ('running', 'attention', 'done')),
    reason          text,
    outcome         text,
    detail          text,
    stage           text,
    undoes_id       integer,
    decider         text    not null check (decider in ('rule', 'ai', 'owner')),
    probability     real,
    undo_keys       text,
    undone_by       integer,
    tag_id          integer,
    decided_mark    integer,
    created_at      integer not null,
    updated_at      integer not null,
    scan_id         integer,
    superseded_by   integer,
    evidence        text
);
insert into activity_entries_old select * from activity_entries;
drop table activity_entries;
alter table activity_entries_old rename to activity_entries;
create index activity_entries_status_idx on activity_entries (status, id);
create index activity_entries_feature_idx on activity_entries (feature, id);
create index activity_entries_subject_idx on activity_entries (kind, subject_key, subject_key_b, id);
drop table assistant_proposals;
