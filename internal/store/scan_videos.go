package store

// 直近の取り込みの対象の動画（scan_videos）と、それがすべて済んだ時刻
// （scans.settled_at）を保つ操作（specs/024-import-progress/data-model.md §1・§2）。

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// remainingJobCondition は、別名 alias の jobs の行が残りの仕事に数えられる条件である。
// queued・running で、登録されたメディアフォルダの中に所在がある（ワーカーが
// 取り出せる）ものに限る。ClaimJob と同じ範囲である。
func remainingJobCondition(alias string) string {
	return alias + `.state in ('queued', 'running') and exists (
		select 1 from video_locations l where l.video_id = ` + alias + `.video_id and ` +
		registeredLocationCondition("l") + `)`
}

// addScanVideos は動画を直近の走査の対象に加える。走査が1つも無ければ加えない。
// 仕事を積む（または着手できるようにする）トランザクションの中で呼ぶ。
// 対象の集合へ加える経路はすべてここを通す（data-model.md §2「加わる時点」）。
func addScanVideos(ctx context.Context, q queryExecer, videoIDs ...int64) error {
	if len(videoIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(videoIDs)), ",")
	args := make([]any, 0, len(videoIDs))
	for _, id := range videoIDs {
		args = append(args, id)
	}
	if _, err := q.ExecContext(ctx, `insert into scan_videos (scan_id, video_id)
		select s.id, v.id from (select max(id) as id from scans) s, videos v
		where s.id is not null and v.id in (`+placeholders+`)
		on conflict (scan_id, video_id) do nothing`, args...); err != nil {
		return fmt.Errorf("cannot add videos to the current import: %w", err)
	}
	return nil
}

// addScanVideosWithRemainingJobs は、残りの仕事がある動画をすべて直近の走査の対象に
// 加える。メディアフォルダの追加・付け替えで仕事が着手できるようになったときと、
// 新しい走査へ前の走査の未完了の動画を持ち越すときに使う。
func addScanVideosWithRemainingJobs(ctx context.Context, q queryExecer) error {
	if _, err := q.ExecContext(ctx, `insert into scan_videos (scan_id, video_id)
		select distinct s.id, j.video_id from (select max(id) as id from scans) s, jobs j
		where s.id is not null and `+remainingJobCondition("j")+`
		on conflict (scan_id, video_id) do nothing`); err != nil {
		return fmt.Errorf("cannot carry unfinished videos into the current import: %w", err)
	}
	return nil
}

// refreshScanSettled は直近の走査の完了の時刻を決め直す。settled_at を書くのは
// ここだけである（data-model.md §1）。
//
// 直近の走査が閉じていて、対象の動画に残りの仕事が無ければ、値が無いときだけ now を
// 入れる。そうでなければ null に戻す。残りの仕事の数が変わりうるトランザクションで、
// コミットの前に呼ぶ。呼び忘れを防ぐため、changes に印を付けた取引では commit が呼ぶ。
func refreshScanSettled(ctx context.Context, q queryExecer, now int64) error {
	if _, err := q.ExecContext(ctx, `update scans
		set settled_at = case
			when state <> 'running' and not exists (
				select 1 from scan_videos sv join jobs j on j.video_id = sv.video_id
				where sv.scan_id = scans.id and `+remainingJobCondition("j")+`)
			then coalesce(settled_at, ?)
			else null end
		where id = (select max(id) from scans)`, now); err != nil {
		return fmt.Errorf("cannot update when the current import settled: %w", err)
	}
	return nil
}

// settle は、changes に残りの仕事が変わりうる印があれば、取引の中で完了の時刻を
// 決め直す。
func settle(ctx context.Context, q queryExecer, c *changes) error {
	if c == nil || !c.settle {
		return nil
	}
	return refreshScanSettled(ctx, q, time.Now().Unix())
}
