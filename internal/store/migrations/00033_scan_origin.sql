-- +goose Up
-- Existing scans are manual; watch identifies folder-watcher scans.
alter table scans add column origin text not null default 'manual'
    check (origin in ('manual', 'watch'));

-- +goose Down
alter table scans drop column origin;
