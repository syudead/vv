-- +goose Up
-- Rename index maintenance time to distinguish it from owner edit time; preserve stored values.
alter table videos rename column updated_at to indexed_at;

-- +goose Down
alter table videos rename column indexed_at to updated_at;
