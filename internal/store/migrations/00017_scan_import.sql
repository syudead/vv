-- +goose Up
-- 直近の取り込みの対象の動画と、それがすべて済んだ時刻
-- （specs/024-import-progress/data-model.md §1・§2）。どちらも走査と準備をやり直せば
-- 作り直せる索引の側に入る。
--
-- settled_at は対象の動画に残りの仕事が無くなった時刻（Unix 秒）。書くのは
-- internal/store の refreshScanSettled だけである。
alter table scans add column settled_at integer;
-- issues_revision は問題の記録（次の移行）を変えるたびに増やす番号。ここでは列だけを足す。
alter table scans add column issues_revision integer not null default 0;

-- 直近の走査の対象の動画の集合。仕事を積むトランザクションで加わり、新しい走査の
-- 開始で入れ替わる。動画の行が消えると連鎖して抜ける。
create table scan_videos (
    scan_id  integer not null references scans(id) on delete cascade,
    video_id integer not null references videos(id) on delete cascade,
    primary key (scan_id, video_id)
);
create index scan_videos_video_idx on scan_videos (video_id);

-- 1. 直近の走査の集合に、着手できる queued・running の仕事が残っている動画を入れる。
--    着手できるとは、登録されたメディアフォルダの中に所在があることである。実行時の
--    条件（store の registeredLocationCondition）は OS の区切り文字と大文字小文字の扱いに
--    従うが、ここでは区切り文字の '/' と '\' をどちらも認める。次に仕事の数が変わる
--    書き込みで refreshScanSettled が完了の時刻を決め直す。
insert or ignore into scan_videos (scan_id, video_id)
select (select max(id) from scans), j.video_id
  from jobs j
 where (select max(id) from scans) is not null
   and j.state in ('queued', 'running')
   and exists (
       select 1 from video_locations l join media_folders mf
        where l.video_id = j.video_id
          and (l.path = mf.path
               or instr(l.path, rtrim(mf.path, '/' || char(92)) || '/') = 1
               or instr(l.path, rtrim(mf.path, '/' || char(92)) || char(92)) = 1));

-- 2. 問題の記録は、それを足す移行が行う。

-- 3. 閉じた走査の完了の時刻は、走査を閉じた時刻とする。ただし直近の走査に 1 で入れた
--    動画があれば、まだ済んでいないので null のままにする。
update scans
   set settled_at = finished_at
 where state <> 'running'
   and not (id = (select max(id) from scans) and exists (select 1 from scan_videos where scan_id = scans.id));

-- +goose Down
drop index if exists scan_videos_video_idx;
drop table if exists scan_videos;
alter table scans drop column issues_revision;
alter table scans drop column settled_at;
