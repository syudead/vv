package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Count only queued/running import stages with registered locations, matching ClaimJob eligibility.
func remainingJobCondition(alias string) string {
	return alias + `.state in ('queued', 'running') and ` + alias + `.kind in (` + importJobKinds + `) and exists (
		select 1 from video_locations l where l.video_id = ` + alias + `.video_id and ` +
		registeredLocationCondition("l") + `)`
}

// Add scan targets through this helper inside the transaction making their jobs available.
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

// Carry unfinished videos into the latest scan when registration changes or a new scan makes their work relevant.
func addScanVideosWithRemainingJobs(ctx context.Context, q queryExecer) error {
	if _, err := q.ExecContext(ctx, `insert into scan_videos (scan_id, video_id)
		select distinct s.id, j.video_id from (select max(id) as id from scans) s, jobs j
		where s.id is not null and `+remainingJobCondition("j")+`
		on conflict (scan_id, video_id) do nothing`); err != nil {
		return fmt.Errorf("cannot carry unfinished videos into the current import: %w", err)
	}
	return nil
}

// Only this helper writes settled_at: closed scans without remaining work settle once; any new remaining work clears it before commit.
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

func settle(ctx context.Context, q queryExecer, c *changes) error {
	if c == nil || !c.settle {
		return nil
	}
	return refreshScanSettled(ctx, q, time.Now().Unix())
}
