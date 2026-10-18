-- +goose Up
-- This initial schema contains only indexes rebuildable by scanning.

create table videos (
    -- Use an integer primary key to share FTS5 rowids.
    id         integer primary key,
    -- Absolute file path, normalized to Unicode NFC before storage.
    path       text    not null unique,
    title      text    not null,
    size_bytes integer not null,
    -- Unix seconds.
    mtime      integer not null,
    -- Unix seconds.
    added_at   integer not null default (unixepoch())
);

-- External content avoids duplicating text; trigram MATCH requires at least three characters.
create virtual table videos_fts using fts5(
    title,
    path,
    content='videos',
    content_rowid='id',
    tokenize='trigram'
);

-- External-content FTS requires deleting old index entries before inserting updated values.
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
