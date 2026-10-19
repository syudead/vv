-- +goose Up
-- A forced job regenerates its artifact without adopting what is on disk; the stage states stay as they are.
alter table jobs add column force integer not null default 0;
-- A force that arrived while this unforced row was running; finishing the row queues a forced row of its kind.
alter table jobs add column force_requested integer not null default 0;
-- Forced rebuilds of each artifact; artifact URLs carry the count so screens load the rebuilt file.
alter table videos add column thumbnail_build integer not null default 0;
alter table videos add column seek_thumbnail_build integer not null default 0;
alter table videos add column preview_build integer not null default 0;

-- +goose Down
alter table videos drop column preview_build;
alter table videos drop column seek_thumbnail_build;
alter table videos drop column thumbnail_build;
alter table jobs drop column force_requested;
alter table jobs drop column force;
