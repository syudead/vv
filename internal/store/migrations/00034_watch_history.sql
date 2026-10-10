-- +goose Up
-- 視聴履歴（specs/043-watch-history/research.md R-1〜R-4、data-model.md）。再生 1 回につき 1 行の、
-- 作り直せない利用者データである。鍵は再生したバージョンの content_key（集まりの user_key ではない）で、
-- videos・playback_progress への外部キーを張らない。動画が消えても行は残り、title はそのときの題名の写しである。
create table watch_history (
    id          integer primary key autoincrement,
    content_key text    not null check (content_key <> ''),
    -- クライアントが付けた視聴 1 回の識別子（R-2）。移行で入れた行は null。
    playback_id text    unique,
    -- 視聴を始めたときに動画のページが見せていた題名。
    title       text    not null,
    -- 視聴の最初の保存の時刻（Unix ミリ秒、R-3）。
    played_at   integer not null
);
create index watch_history_played_idx on watch_history (played_at desc, id desc);
create index watch_history_content_idx on watch_history (content_key);

-- 既存の再生位置の記録 1 行につき 1 件を入れる（R-4）。集まりの鍵（'bundle:<id>'）は集まりの代表の
-- content_key に引き直し、集まりの行が無いものだけ捨てる。題名は表示名、無ければ代表の所在
-- （パスの順で最初の所在）の題名、どちらも無ければ空。移行の時点で動画が無い記録も入れる。
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
