-- +goose Up
-- watch_history.title_key は題名の写しの照合形（FoldForMatch の結果で、改行は空白）で、動画の無い件の
-- 題名の検索に使う（specs/043-watch-history/data-model.md「Migration」、research.md R-10）。SQL では
-- 照合形を作れないので、既存の行は null のまま残し、起動時に
-- PlaybackStore.RefreshWatchHistoryTitleKeys が埋める。新しい件は書いた時点で持つ。
alter table watch_history add column title_key text;

-- +goose Down
alter table watch_history drop column title_key;
