-- +goose Up
-- Activity is persistent and unpruned, retaining content keys and label snapshots without video foreign keys; do not invent historical entries.
create table activity_entries (
    id              integer primary key,
    kind            text    not null,
    -- Undo inherits the original entry's feature.
    feature         text    not null,
    subject_kind    text    not null check (subject_kind in ('video', 'pair', 'scan', 'folder')),
    -- Content key, smaller pair key, scan ID or removed media-folder path.
    subject_key     text    not null,
    -- The larger pair key.
    subject_key_b   text,
    subject_label   text    not null,
    subject_label_b text,
    status          text    not null check (status in ('running', 'attention', 'done')),
    -- Reason for running or attention state.
    reason          text,
    -- Outcome for done state.
    outcome         text,
    detail          text,
    -- Terminally failed import stage, using jobs.kind.
    stage           text,
    -- Entry reversed by this undo.
    undoes_id       integer,
    decider         text    not null check (decider in ('rule', 'ai', 'owner')),
    probability     real,
    -- JSON containing the automatically joined members and their original representative.
    undo_keys       text,
    -- Owner entry that reversed this activity.
    undone_by       integer,
    -- Stable tag ID for owner edits, independent of renaming.
    tag_id          integer,
    -- Maximum activity ID at automatic result time; only later owner IDs count as subsequent edits for undo.
    decided_mark    integer,
    created_at      integer not null,
    updated_at      integer not null
);

create index activity_entries_status_idx on activity_entries (status, id);
create index activity_entries_feature_idx on activity_entries (feature, id);
create index activity_entries_subject_idx on activity_entries (kind, subject_key, subject_key_b, id);

-- Retain applied tags for per-tag undo; later owner activity IDs protect subsequent owner edits.
create table activity_entry_tags (
    entry_id    integer not null,
    tag_id      integer not null,
    tag_name    text    not null,
    suggested   integer not null default 0,
    -- User-data key at attachment time, either content or bundle key.
    user_key    text    not null,
    probability real    not null,
    undone_by   integer,
    primary key (entry_id, tag_id)
) without rowid;

-- Track only surviving automatic attachment copies; undo removes these rows and yields already_reverted when the current primary key is not tracked.
create table activity_entry_tag_keys (
    entry_id    integer not null,
    tag_id      integer not null,
    user_key    text    not null,
    primary key (entry_id, tag_id, user_key)
) without rowid;
create index activity_entry_tag_keys_key_idx on activity_entry_tag_keys (user_key);

-- Record all content keys touched by owner tag edits, including every current bundle member, for later undo checks.
create table activity_entry_keys (
    entry_id    integer not null,
    content_key text    not null,
    primary key (entry_id, content_key)
) without rowid;
create index activity_entry_keys_key_idx on activity_entry_keys (content_key, entry_id);

-- Owner-reversed automatic dismissals are persistent holds excluded from judging.
create table video_version_judging_holds (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- Claiming a queue row whose activity is no longer running opens a new activity.
alter table jobs add column activity_id integer;
alter table auto_tag_queue add column activity_id integer;
alter table video_version_candidates add column activity_id integer;

-- +goose Down
alter table video_version_candidates drop column activity_id;
alter table auto_tag_queue drop column activity_id;
alter table jobs drop column activity_id;
drop table if exists video_version_judging_holds;
drop table if exists activity_entry_keys;
drop table if exists activity_entry_tag_keys;
drop table if exists activity_entry_tags;
drop table if exists activity_entries;
