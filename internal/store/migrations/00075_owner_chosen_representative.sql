-- +goose Up
-- The owner picked this representative; automatic bundling keeps it instead of choosing by the files. Existing
-- bundles count as picked, since most were bundled by hand.
alter table video_bundles add column representative_by_owner integer not null default 1;

-- +goose Down
alter table video_bundles drop column representative_by_owner;
