-- +goose Up
-- Store the same crop at up to 2048 pixels for future editing; legacy NULL sources fall back to the published image.
alter table thumbnail_images add column source blob;
-- Legacy rows are generated images; newer uploads preserve their origin.
alter table thumbnail_images add column origin text not null default 'generated';

-- +goose Down
alter table thumbnail_images drop column origin;
alter table thumbnail_images drop column source;
