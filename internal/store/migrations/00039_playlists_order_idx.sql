-- +goose Up
-- プレイリストの一覧の並びとカーソル（specs/044-playlists/data-model.md「Migration」、
-- contracts/screen-api.md「GET /api/playlists」）。ListPlaylists は (updated_at desc, id desc) の順に
-- カーソルの次から 1 ページずつ読むので、この索引が無いとページごとに playlists を全部読んで
-- 並べ替える。
create index playlists_order_idx on playlists (updated_at desc, id desc);

-- +goose Down
drop index if exists playlists_order_idx;
