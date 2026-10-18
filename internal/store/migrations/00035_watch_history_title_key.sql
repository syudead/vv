-- +goose Up
-- Go backfills normalized title keys at startup; SQL leaves legacy rows NULL because it cannot perform Unicode folding.
alter table watch_history add column title_key text;

-- +goose Down
alter table watch_history drop column title_key;
