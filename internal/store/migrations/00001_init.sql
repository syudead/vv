-- +goose Up
-- Phase 0 の最小スキーマ。利用者データは持たず、全体が「消してもスキャンで
-- 再構築できる索引」である（specs/001-initial-setup/data-model.md）。

create table videos (
    -- FTS5 の rowid と対応させるため integer primary key にする。
    id         integer primary key,
    -- ファイルの絶対パス。保存前に Unicode NFC へ正規化する。
    path       text    not null unique,
    -- 表示名。Phase 0 では拡張子を除いたファイル名。
    title      text    not null,
    size_bytes integer not null,
    -- ファイルの更新時刻（Unix 秒）。
    mtime      integer not null,
    -- 登録時刻（Unix 秒）。
    added_at   integer not null default (unixepoch())
);

-- 外部コンテンツ方式にして本文の二重保持を避ける。trigram は3文字単位で索引を
-- 作るため、2文字以下の検索語は MATCH に一致しない。1〜2文字はこの表への
-- LIKE '%…%'（trigram 索引で処理される）に振り分ける（research.md R-001）。
create virtual table videos_fts using fts5(
    title,
    path,
    content='videos',
    content_rowid='id',
    tokenize='trigram'
);

-- videos への変更を videos_fts へ同期するトリガ。外部コンテンツ方式では
-- 削除・更新の前に 'delete' 行を書き込んでから新しい値を入れる。
-- +goose StatementBegin
create trigger videos_ai after insert on videos begin
    insert into videos_fts(rowid, title, path) values (new.id, new.title, new.path);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_ad after delete on videos begin
    insert into videos_fts(videos_fts, rowid, title, path) values ('delete', old.id, old.title, old.path);
end;
-- +goose StatementEnd

-- +goose StatementBegin
create trigger videos_au after update on videos begin
    insert into videos_fts(videos_fts, rowid, title, path) values ('delete', old.id, old.title, old.path);
    insert into videos_fts(rowid, title, path) values (new.id, new.title, new.path);
end;
-- +goose StatementEnd

-- +goose Down
drop trigger if exists videos_au;
drop trigger if exists videos_ad;
drop trigger if exists videos_ai;
drop table if exists videos_fts;
drop table if exists videos;
