-- +goose Up
alter table videos add column location_generation integer not null default 1;

-- +goose Down
alter table videos drop column location_generation;
