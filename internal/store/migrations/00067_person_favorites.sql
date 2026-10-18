-- +goose Up
-- Favorited time is Unix milliseconds, shared with the other favorites; no smart rule reads person favorites.
create table person_favorites (
    person_id    integer primary key references people (id) on delete cascade,
    favorited_at integer not null
);

-- +goose Down
drop table person_favorites;
