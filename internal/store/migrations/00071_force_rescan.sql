-- +goose Up
-- A forced job regenerates its artifact without adopting what is on disk; the stage states stay as they are.
alter table jobs add column force integer not null default 0;
-- A force that arrived while this unforced row was running; finishing the row queues a forced row of its kind.
alter table jobs add column force_requested integer not null default 0;
-- The automatic decision (activity entry id) this candidate judges again; such a pair stays claimable while bundled or dismissed.
alter table video_version_candidates add column rejudge_of integer;

-- +goose Down
alter table video_version_candidates drop column rejudge_of;
alter table jobs drop column force_requested;
alter table jobs drop column force;
