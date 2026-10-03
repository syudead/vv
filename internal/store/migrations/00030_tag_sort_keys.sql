-- +goose Up
-- 名前の自然順の鍵（specs/036-tag-admin-scale/research.md R-10、data-model.md §0）。
-- タグ管理画面の一覧をサーバーのページで読むため、名前の順を SQL の order by と keyset の
-- カーソルで引けるようにする。sort_key は domain.NaturalSortKey(name) を Go が書く
-- （video_locations.title_key と同じ鍵）。作り直せる派生の値で、表の意味は変えない。
alter table tag_names add column sort_key text not null default '';

-- 却下した名前にも同じ鍵と、鍵を作った規則の版を持たせる。search_version は
-- tag_names.search_version と同じ意味（domain.SearchKeyVersion）。既定の 0 で、既存の行は
-- 起動時の TagStore.RefreshSearchKeys の埋め直しに入る。
alter table rejected_tag_names add column sort_key text not null default '';
alter table rejected_tag_names add column search_version integer not null default 0;

-- 既存のタグ名の行を埋め直しの対象に戻す。起動時に search_key と一緒に sort_key が埋まる。
update tag_names set search_version = 0;

-- +goose Down
alter table rejected_tag_names drop column search_version;
alter table rejected_tag_names drop column sort_key;
alter table tag_names drop column sort_key;
