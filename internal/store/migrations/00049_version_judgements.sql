-- +goose Up
-- Candidate judgements are rebuildable; rebuilding resets state to waiting and attempts to zero.
alter table video_version_candidates add column judgement text;
-- Failure count survives claims and startup recovery.
alter table video_version_candidates add column attempts integer not null default 0;
-- Earliest retry time in Unix seconds for failed rows; otherwise NULL.
alter table video_version_candidates add column retry_at integer;
-- Provider failure detail.
alter table video_version_candidates add column judgement_error text;
-- Model probability for undecided results; rule conflicts use NULL.
alter table video_version_candidates add column probability real;

-- Persistent sorted unbundled pairs must not be automatically bundled again.
create table video_version_unbundles (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- +goose Down
drop table if exists video_version_unbundles;
alter table video_version_candidates drop column probability;
alter table video_version_candidates drop column judgement_error;
alter table video_version_candidates drop column retry_at;
alter table video_version_candidates drop column attempts;
alter table video_version_candidates drop column judgement;
