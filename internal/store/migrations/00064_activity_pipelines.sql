-- +goose Up
-- A pipeline is an activity entry of kind 'pipeline'; earlier entries stay standalone, so nothing is backfilled.
-- The scan whose queueing opened a pipeline.
alter table activity_entries add column scan_id integer;
-- The entry that closed this one as superseded.
alter table activity_entries add column superseded_by integer;

-- An entry can belong to several pipelines; like the rest of the log, rows are never deleted.
create table activity_pipeline_members (
    pipeline_id integer not null,
    entry_id    integer not null,
    joined_at   integer not null,
    primary key (pipeline_id, entry_id)
) without rowid;
create index activity_pipeline_members_entry_idx on activity_pipeline_members (entry_id, pipeline_id);

-- One row per ingest stage an import entry held; no video foreign key, so the rows outlive the video.
create table activity_entry_stages (
    entry_id   integer not null,
    stage      text    not null check (stage in ('probe', 'thumbnail', 'seek_thumbnail', 'preview', 'fingerprint')),
    state      text    not null check (state in ('waiting', 'processing', 'done', 'failed', 'gone')),
    -- The stage's last error message, untranslated.
    detail     text,
    updated_at integer not null,
    primary key (entry_id, stage)
) without rowid;

-- +goose Down
drop table if exists activity_entry_stages;
drop table if exists activity_pipeline_members;
alter table activity_entries drop column superseded_by;
alter table activity_entries drop column scan_id;
