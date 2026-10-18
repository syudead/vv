-- +goose Up
-- Support direct keyset seeks by descending edit time and ID instead of sorting all playlists per page.
create index playlists_order_idx on playlists (updated_at desc, id desc);

-- +goose Down
drop index if exists playlists_order_idx;
