-- +goose Up
-- image_revision lives on people, not on the image row, so a face uploaded after a removal never reuses a revision.
alter table people add column image_revision integer not null default 0;
create table person_images (
    person_id  integer primary key references people (id) on delete cascade,
    image      blob    not null,
    width      integer not null,
    height     integer not null,
    updated_at integer not null
);

-- +goose Down
drop table person_images;
alter table people drop column image_revision;
