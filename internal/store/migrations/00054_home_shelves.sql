-- +goose Up
-- ホームの棚（specs/057-home/data-model.md「Migration」、research.md R-7）。棚はホームの位置を持つ
-- スマートフォルダで、位置の小さい順、同じなら id の順に並ぶ。どの行にも位置を入れないので、ホームは
-- フォルダの棚を持たずに始まる。
alter table smart_folders add column home_position integer;
create index smart_folders_home_idx on smart_folders (home_position) where home_position is not null;

-- +goose Down
drop index if exists smart_folders_home_idx;
alter table smart_folders drop column home_position;
