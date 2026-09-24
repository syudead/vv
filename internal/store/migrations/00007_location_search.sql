-- +goose Up
-- 所在ごとの照合用の鍵（specs/013-library-search/data-model.md §1・§2）。
-- 鍵の値は SQL では作れない（NFKC などの正規化が要る）ので、ここでは列・索引・
-- トリガを作るまでとし、値は起動時の埋め直し（store.RefreshSearchKeys）が埋める。
alter table video_locations add column search_key text not null default '';
alter table video_locations add column title_key text not null default '';
alter table video_locations add column search_version integer not null default 0;

drop trigger if exists video_locations_au;
drop trigger if exists video_locations_ad;
drop trigger if exists video_locations_ai;
drop table if exists videos_fts;

create virtual table location_search_fts using fts5(
    search_key, content='video_locations', content_rowid='id', tokenize='trigram'
);
-- 索引へ写すのは search_key だけである。題名やパスの変更は、Go が同じ書き込みの
-- 中で search_key を作り直すことで索引に届く。
-- +goose StatementBegin
create trigger location_search_fts_ai after insert on video_locations begin
    insert into location_search_fts(rowid, search_key) values (new.id, new.search_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger location_search_fts_ad after delete on video_locations begin
    insert into location_search_fts(location_search_fts, rowid, search_key)
    values ('delete', old.id, old.search_key);
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger location_search_fts_au after update of search_key on video_locations begin
    insert into location_search_fts(location_search_fts, rowid, search_key)
    values ('delete', old.id, old.search_key);
    insert into location_search_fts(rowid, search_key) values (new.id, new.search_key);
end;
-- +goose StatementEnd
-- 既存の所在も（空の鍵のまま）索引に載せておく。以後は更新トリガが、埋め直しで
-- 書かれた鍵を写す。
insert into location_search_fts(location_search_fts) values ('rebuild');

-- +goose Down
drop trigger if exists location_search_fts_au;
drop trigger if exists location_search_fts_ad;
drop trigger if exists location_search_fts_ai;
drop table if exists location_search_fts;

create virtual table videos_fts using fts5(
    title, path, content='video_locations', content_rowid='id', tokenize='trigram'
);
-- +goose StatementBegin
create trigger video_locations_ai after insert on video_locations begin
    insert into videos_fts(rowid, title, path) values (new.id, new.title, new.path);
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_locations_ad after delete on video_locations begin
    insert into videos_fts(videos_fts, rowid, title, path) values ('delete', old.id, old.title, old.path);
end;
-- +goose StatementEnd
-- +goose StatementBegin
create trigger video_locations_au after update on video_locations begin
    insert into videos_fts(videos_fts, rowid, title, path) values ('delete', old.id, old.title, old.path);
    insert into videos_fts(rowid, title, path) values (new.id, new.title, new.path);
end;
-- +goose StatementEnd
insert into videos_fts(videos_fts) values ('rebuild');

alter table video_locations drop column search_version;
alter table video_locations drop column title_key;
alter table video_locations drop column search_key;
