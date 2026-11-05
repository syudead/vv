-- +goose Up
-- Organizations get the person's profile, image and favorite shapes; domain.NormalizeOrganizationProfile checks values.
alter table organizations add column founded_date text;
alter table organizations add column dissolved_date text;
alter table organizations add column location text;
alter table organizations add column bio text;
-- image_revision lives on organizations, not on the image row, so a logo uploaded after a removal never reuses a revision.
alter table organizations add column image_revision integer not null default 0;
create table organization_links (
    organization_id integer not null references organizations (id) on delete cascade,
    position        integer not null,
    url             text    not null,
    primary key (organization_id, position),
    unique (organization_id, url)
) without rowid;
create table organization_images (
    organization_id integer primary key references organizations (id) on delete cascade,
    image           blob    not null,
    width           integer not null,
    height          integer not null,
    updated_at      integer not null
);
-- Favorited time is Unix milliseconds, shared with the other favorites; no smart rule reads organization favorites.
create table organization_favorites (
    organization_id integer primary key references organizations (id) on delete cascade,
    favorited_at    integer not null
);


-- +goose Down
drop table organization_favorites;
drop table organization_images;
drop table organization_links;
alter table organizations drop column image_revision;
alter table organizations drop column bio;
alter table organizations drop column location;
alter table organizations drop column dissolved_date;
alter table organizations drop column founded_date;
