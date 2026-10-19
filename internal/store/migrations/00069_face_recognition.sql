-- +goose Up
-- Face data is keyed by content key or person id, never videos.id, so it survives a library rebuild.
-- Like auto_tag_queue, a done row stays so analysis after import skips content that was analysed before.
create table face_queue (
    content_key text    primary key,
    state       text    not null check (state in ('queued', 'running', 'done', 'failed')),
    -- The recognizer of the last finished analysis.
    recognizer  text,
    -- The helper's or ffmpeg's message, only for failed rows.
    reason      text,
    activity_id integer,
    updated_at  integer not null
) without rowid;
create index face_queue_state_idx on face_queue (state, updated_at);

-- One main person of one video; rows outlive the video so a returning file keeps its links.
create table face_appearances (
    id           integer primary key autoincrement,
    content_key  text    not null,
    recognizer   text    not null,
    -- Null only once the person is deleted.
    person_id    integer references people (id) on delete set null,
    -- 1 once the owner removed or undid the link or deleted the person; never linked again.
    dropped      integer not null default 0 check (dropped in (0, 1)),
    -- JSON [[start, end, x, y, w], ...] per track.
    tracks       text    not null,
    good_seconds real    not null,
    track_count  integer not null,
    -- Normalized little-endian float32 centroid; null once the person is deleted.
    centroid     blob,
    -- JPEG of the best good face, never rewritten; null once the person is deleted.
    crop         blob
);
create index face_appearances_content_idx on face_appearances (content_key);
create index face_appearances_person_idx on face_appearances (person_id);

-- A row whose revision or recognizer is not the current one is stale and computed again.
create table person_face_vectors (
    person_id      integer primary key references people (id) on delete cascade,
    image_revision integer not null,
    recognizer     text    not null,
    -- Null when the face image had no good face or more than one.
    vector         blob
);

create table face_candidates (
    provisional_id integer not null references people (id) on delete cascade,
    person_id      integer not null references people (id) on delete cascade,
    similarity     real    not null,
    dismissed      integer not null default 0 check (dismissed in (0, 1)),
    created_at     integer not null,
    primary key (provisional_id, person_id)
) without rowid;
create index face_candidates_person_idx on face_candidates (person_id);

-- No foreign key to face_appearances: a re-analysis keeps a carried-over id, and undo of a link whose appearance
-- is gone still removes the performer row.
create table activity_entry_face_links (
    entry_id       integer not null,
    appearance_id  integer not null,
    person_id      integer not null,
    user_key       text    not null,
    created_person integer not null check (created_person in (0, 1)),
    -- The owner's undo entry once the link is undone.
    undone_by      integer,
    primary key (entry_id, appearance_id)
) without rowid;

-- What a face_merge entry moved other than credits, as JSON.
create table activity_entry_person_merges (
    entry_id  integer primary key,
    source_id integer not null,
    target_id integer not null,
    snapshot  text    not null
);

create table activity_entry_merge_credits (
    entry_id      integer not null,
    user_key      text    not null,
    role          text    not null,
    target_had    integer not null check (target_had in (0, 1)),
    owner_changed integer not null default 0 check (owner_changed in (0, 1)),
    primary key (entry_id, user_key, role)
) without rowid;

-- SQLite cannot alter a CHECK in place; rebuild activity_entries to allow person subjects, keeping every row.
create table activity_entries_new (
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
    superseded_by   integer
);
insert into activity_entries_new
select id, kind, feature, subject_kind, subject_key, subject_key_b, subject_label, subject_label_b, status, reason,
       outcome, detail, stage, undoes_id, decider, probability, undo_keys, undone_by, tag_id, decided_mark,
       created_at, updated_at, scan_id, superseded_by
from activity_entries;
drop table activity_entries;
alter table activity_entries_new rename to activity_entries;
create index activity_entries_status_idx on activity_entries (status, id);
create index activity_entries_feature_idx on activity_entries (feature, id);
create index activity_entries_subject_idx on activity_entries (kind, subject_key, subject_key_b, id);

-- +goose Down
delete from activity_entries where subject_kind = 'person';
create table activity_entries_old (
    id              integer primary key,
    kind            text    not null,
    feature         text    not null,
    subject_kind    text    not null check (subject_kind in ('video', 'pair', 'scan', 'folder')),
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
    superseded_by   integer
);
insert into activity_entries_old select * from activity_entries;
drop table activity_entries;
alter table activity_entries_old rename to activity_entries;
create index activity_entries_status_idx on activity_entries (status, id);
create index activity_entries_feature_idx on activity_entries (feature, id);
create index activity_entries_subject_idx on activity_entries (kind, subject_key, subject_key_b, id);
drop table if exists activity_entry_merge_credits;
drop table if exists activity_entry_person_merges;
drop table if exists activity_entry_face_links;
drop table if exists face_candidates;
drop table if exists person_face_vectors;
drop table if exists face_appearances;
drop table if exists face_queue;
