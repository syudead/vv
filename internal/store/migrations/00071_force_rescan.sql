-- +goose Up
-- A forced job regenerates its artifact without adopting what is on disk; the stage states stay as they are.
alter table jobs add column force integer not null default 0;
-- A force that arrived while this unforced row was running; finishing the row queues a forced row of its kind.
alter table jobs add column force_requested integer not null default 0;

-- +goose Down
alter table jobs drop column force_requested;
alter table jobs drop column force;
