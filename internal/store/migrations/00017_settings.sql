-- +goose Up
-- Persistent owner settings; a missing key means it has never been chosen.
create table settings (
    key        text primary key,
    value      text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists settings;
