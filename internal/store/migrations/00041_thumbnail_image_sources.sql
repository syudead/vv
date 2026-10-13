-- +goose Up
-- 適用した画像を、今のサムネイルから編集を始められる解像度でも持ち、どこから来た画像かを記録する
-- （specs/046-thumbnail-image-upload/data-model.md、research.md R-3・R-5）。
-- 動画の縦横比に合わせ、長辺を最大 2048 px にした JPEG（image と同じ切り抜き）。この移行より前に
-- 書いた行では null で、今のサムネイルからの編集は image から始める。空のバイト列は書かない。
alter table thumbnail_images add column source blob;
-- 適用した画像の出どころ。generated（ComfyUI の編集結果）か uploaded（所有者が上げた画像）。
-- この移行より前の行は生成画像の適用だけなので generated にする。
alter table thumbnail_images add column origin text not null default 'generated';

-- +goose Down
alter table thumbnail_images drop column origin;
alter table thumbnail_images drop column source;
