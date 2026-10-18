-- +goose Up
-- Display width/height includes rotation and pixel aspect ratio; unknown values remain NULL.
alter table videos add column display_aspect_ratio real;

-- +goose Down
alter table videos drop column display_aspect_ratio;
