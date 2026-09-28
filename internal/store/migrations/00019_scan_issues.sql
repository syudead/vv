-- +goose Up
-- 直近の取り込みで起きた、利用者に知らせる出来事（specs/024-import-progress/data-model.md §3）。
-- 走査と準備をやり直せば作り直せる索引の側に入る。
--
-- 1出来事1行で保存し、読み出しで動画（未登録ならファイルのパス）ごとに1件へまとめる。
-- 行を入れる・消す取引は、同じ中で直近の走査の scans.issues_revision を増やす。
create table scan_issues (
    id         integer primary key,
    scan_id    integer not null references scans(id) on delete cascade,
    video_id   integer references videos(id) on delete cascade,
    path       text not null,
    kind       text not null,
    created_at integer not null
);
create unique index scan_issues_video_kind_idx on scan_issues (scan_id, video_id, kind)
    where video_id is not null;
create unique index scan_issues_path_kind_idx on scan_issues (scan_id, path, kind)
    where video_id is null;
create index scan_issues_video_idx on scan_issues (video_id);

-- data-model.md §1 の手順 2: 今 failed の準備がある動画を、直近の走査の問題として入れる
-- （research.md R-11）。走査中のファイルの失敗は移行前の行に残っていないので復元しない。
-- path は所在のうちパスの最小のもの（動画の代表の選び方と同じ順）である。
insert or ignore into scan_issues (scan_id, video_id, path, kind, created_at)
select (select max(id) from scans), v.id,
       (select min(l.path) from video_locations l where l.video_id = v.id),
       k.kind, cast(strftime('%s', 'now') as integer)
  from videos v
  join (select 'probe_failed' as kind union all select 'thumbnail_failed'
        union all select 'seek_thumbnail_failed' union all select 'preview_failed') k
    on (k.kind = 'probe_failed' and v.probe_state = 'failed')
    or (k.kind = 'thumbnail_failed' and v.thumbnail_state = 'failed')
    or (k.kind = 'seek_thumbnail_failed' and v.seek_thumbnail_state = 'failed')
    or (k.kind = 'preview_failed' and v.preview_state = 'failed')
 where (select max(id) from scans) is not null
   and exists (select 1 from video_locations l where l.video_id = v.id);

update scans
   set issues_revision = issues_revision + 1
 where id = (select max(id) from scans)
   and exists (select 1 from scan_issues where scan_id = scans.id);

-- +goose Down
drop index if exists scan_issues_video_idx;
drop index if exists scan_issues_path_kind_idx;
drop index if exists scan_issues_video_kind_idx;
drop table if exists scan_issues;
