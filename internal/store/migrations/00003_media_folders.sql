-- +goose Up
create table media_folders (
    id integer primary key autoincrement,
    path text not null unique,
    version integer not null default 1 check (version >= 1),
    created_at integer not null,
    updated_at integer not null
);

create temporary table legacy_video_locations as
select id as video_id, path, title, size_bytes, mtime, added_at, updated_at from videos;

drop trigger if exists videos_au;
drop trigger if exists videos_ad;
drop trigger if exists videos_ai;
drop table if exists videos_fts;

create table videos_new (
    id integer primary key,
    added_at integer not null default (unixepoch()),
    content_key text not null default '',
    duration_ms integer, width integer, height integer, container text,
    video_codec text, audio_codec text,
    playable integer not null default 0, unplayable_reason text,
    probe_state text not null default 'pending', probe_error text,
    thumbnail_state text not null default 'pending', updated_at integer not null default 0
);
insert into videos_new
select id, added_at, content_key, duration_ms, width, height, container, video_codec,
       audio_codec, playable, unplayable_reason, probe_state, probe_error,
       thumbnail_state, updated_at from videos;

create table jobs_new (
    id integer primary key,
    kind text not null check (kind in ('probe', 'thumbnail')),
    video_id integer not null references videos_new (id) on delete cascade,
    state text not null check (state in ('queued', 'running', 'done', 'failed')),
    attempts integer not null default 0, last_error text,
    created_at integer not null, updated_at integer not null,
    location_id integer, location_version integer, location_path text
);
insert into jobs_new (id, kind, video_id, state, attempts, last_error, created_at, updated_at)
select id, kind, video_id, case when state = 'running' then 'queued' else state end,
       attempts, last_error, created_at, updated_at from jobs;

drop table jobs;
drop table videos;
alter table videos_new rename to videos;
alter table jobs_new rename to jobs;

create unique index videos_content_key_idx on videos (content_key) where content_key <> '';
create index videos_added_at_desc_idx on videos (added_at desc, id desc);
create index jobs_state_id_idx on jobs (state, id);
create unique index jobs_pending_kind_video_idx on jobs (kind, video_id)
where state in ('queued', 'running');

create table video_locations (
    id integer primary key autoincrement,
    video_id integer not null references videos (id) on delete cascade,
    path text not null unique,
    version integer not null default 1 check (version >= 1),
    title text not null, size_bytes integer not null, mtime integer not null,
    created_at integer not null, updated_at integer not null
);
insert into video_locations (video_id, path, version, title, size_bytes, mtime, created_at, updated_at)
select video_id, path, 1, title, size_bytes, mtime, added_at, updated_at from legacy_video_locations;
drop table legacy_video_locations;

create index video_locations_video_path_idx on video_locations (video_id, path);
create index video_locations_title_idx on video_locations (title, video_id);
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

-- +goose Down
drop trigger if exists video_locations_au;
drop trigger if exists video_locations_ad;
drop trigger if exists video_locations_ai;
drop table if exists videos_fts;

create table videos_old (
    id integer primary key,
    path text not null unique, title text not null, size_bytes integer not null, mtime integer not null,
    added_at integer not null default (unixepoch()), content_key text not null default '',
    duration_ms integer, width integer, height integer, container text,
    video_codec text, audio_codec text, playable integer not null default 0,
    unplayable_reason text, probe_state text not null default 'pending', probe_error text,
    thumbnail_state text not null default 'pending', updated_at integer not null default 0
);
insert into videos_old
select v.id, l.path, l.title, l.size_bytes, l.mtime, v.added_at, v.content_key,
       v.duration_ms, v.width, v.height, v.container, v.video_codec, v.audio_codec,
       v.playable, v.unplayable_reason, v.probe_state, v.probe_error,
       v.thumbnail_state, v.updated_at
from videos v join video_locations l on l.id = (
    select id from video_locations where video_id = v.id order by path limit 1
);

create table jobs_old (
    id integer primary key, kind text not null check (kind in ('probe', 'thumbnail')),
    video_id integer not null references videos_old (id) on delete cascade,
    state text not null check (state in ('queued', 'running', 'done', 'failed')),
    attempts integer not null default 0, last_error text,
    created_at integer not null, updated_at integer not null
);
insert into jobs_old select id, kind, video_id, state, attempts, last_error, created_at, updated_at from jobs;

drop table jobs;
drop table video_locations;
drop table videos;
alter table videos_old rename to videos;
alter table jobs_old rename to jobs;
drop table media_folders;

create unique index videos_content_key_idx on videos (content_key) where content_key <> '';
create index videos_added_at_desc_idx on videos (added_at desc, id desc);
create index videos_title_asc_idx on videos (title asc, id asc);
create index jobs_state_id_idx on jobs (state, id);
create unique index jobs_pending_kind_video_idx on jobs (kind, video_id) where state in ('queued', 'running');
create virtual table videos_fts using fts5(title, path, content='videos', content_rowid='id', tokenize='trigram');
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
insert into videos_fts(videos_fts) values ('rebuild');
