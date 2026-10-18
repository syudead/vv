-- +goose Up
-- History persists one row per viewing of a content version, without video/progress foreign keys; titles retain their original snapshot.
create table watch_history (
    id          integer primary key autoincrement,
    content_key text    not null check (content_key <> ''),
    -- Client viewing ID; migrated rows use NULL.
    playback_id text    unique,
    -- Title shown when viewing began.
    title       text    not null,
    -- First save time in Unix milliseconds.
    played_at   integer not null
);
create index watch_history_played_idx on watch_history (played_at desc, id desc);
create index watch_history_content_idx on watch_history (content_key);

-- Backfill one entry per progress row, resolving bundles to representatives and preserving entries for currently missing videos.
insert into watch_history (content_key, playback_id, title, played_at)
select k.content_key, null,
       coalesce(
           (select o.display_name from video_overrides o where o.content_key = k.content_key),
           (select l.title from videos v join video_locations l on l.video_id = v.id
             where v.content_key = k.content_key order by l.path limit 1),
           ''),
       k.updated_at * 1000
  from (select case when p.content_key like 'bundle:%'
                    then (select b.representative_key from video_bundles b where b.user_key = p.content_key)
                    else p.content_key end as content_key,
               p.updated_at
          from playback_progress p) k
 where k.content_key is not null and k.content_key <> ''
 order by k.updated_at, k.content_key;

-- +goose Down
drop index if exists watch_history_content_idx;
drop index if exists watch_history_played_idx;
drop table if exists watch_history;
