-- +goose Up
-- シーク用サムネイルを個別 JPEG からスプライトシートに作り直す
-- （specs/021-seek-thumbnail-sprite/research.md R-6）。schema は変えない。
-- done の動画だけを作り直しの対象にする。failed は読み取りのやり直しで戻し、
-- 代表サムネイルと動くプレビューの状態とジョブには触れない。
-- 旧形式の個別 JPEG は、スプライトの公開時に internal/artifacts が消す。
insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
select 'seek_thumbnail', id, 'queued', 0, unixepoch(), unixepoch()
from videos
where seek_thumbnail_state = 'done'
  and probe_state <> 'pending'
  and exists (select 1 from video_locations where video_id = videos.id)
  and not exists (
      select 1 from jobs
      where jobs.kind = 'seek_thumbnail' and jobs.video_id = videos.id
        and jobs.state in ('queued', 'running'));

update videos set seek_thumbnail_state = 'pending' where seek_thumbnail_state = 'done';

-- +goose Down
-- 旧形式の個別 JPEG は公開時に消えるので、done に戻すと置き場の無い動画を
-- 完成扱いにする。戻さず、積んだジョブもそのまま残す（schema は変えていない）。
select 1;
