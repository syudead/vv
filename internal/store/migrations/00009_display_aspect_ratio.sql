-- +goose Up
-- 表示される縦横比（回転と画素の縦横比を反映した横÷縦）。プレイヤーの枠を再生前から
-- 動画の形に合わせるために使う。解析前や取得できない動画は null のままにする。
alter table videos add column display_aspect_ratio real;

-- +goose Down
alter table videos drop column display_aspect_ratio;
