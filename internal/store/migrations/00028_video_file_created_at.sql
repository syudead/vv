-- +goose Up
-- 一覧に出す所在のファイルの作成日時（Unix 秒）。ファイルシステムが持たなければ null
-- （specs/033-video-dates/research.md R-4）。既存の所在は null のままで、次の走査が埋める。
-- 所在の事実で、作り直せる索引の側（video_locations の一部）である。
alter table video_locations add column file_created_at integer;

-- +goose Down
alter table video_locations drop column file_created_at;
