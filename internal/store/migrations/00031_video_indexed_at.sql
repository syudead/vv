-- +goose Up
-- videos.updated_at は索引の行を書き換えた時刻で、取り込み・解析・サムネイル作成などの
-- 裏方の処理でも進む。API の updatedAt（利用者の編集による更新日時、video_edits.edited_at）
-- と取り違えないよう、役割の分かる名前に変える（#650）。値は変えない。
alter table videos rename column updated_at to indexed_at;

-- +goose Down
alter table videos rename column indexed_at to updated_at;
