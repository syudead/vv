-- +goose Up
-- フォルダの配下の所在を、パスの接頭辞の範囲で引くための索引（issue 674）。Windows では
-- パスを lower(path) で大文字小文字を区別せずに比べる（folderPathExpr）ので、path の
-- 一意の索引は使えない。同じ式の索引を持たせ、フォルダの画面とフォルダ検索が
-- video_locations を全件走査しないようにする。ほかの OS では path の索引を使う。
create index video_locations_lower_path_idx on video_locations (lower(path));

-- +goose Down
drop index if exists video_locations_lower_path_idx;
