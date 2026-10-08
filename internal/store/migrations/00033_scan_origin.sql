-- +goose Up
-- scans.origin は走査を始めた主体である。manual は利用者（と起動時の再開・外部クライアント）、
-- watch はフォルダの監視。既存の行はすべて manual である
-- （specs/042-folder-watch-import/data-model.md）。
alter table scans add column origin text not null default 'manual'
    check (origin in ('manual', 'watch'));

-- +goose Down
alter table scans drop column origin;
