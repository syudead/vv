package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// keySubjectJobKinds are the kinds whose subject_key, and subject_key_b, are content keys of videos.
const keySubjectJobKinds = `'auto_tag', 'face', 'version_judge'`

// BackgroundRemaining counts the unfinished jobs, queued or running, of every job group: videos for the import group
// and jobs for the others. A job waiting out a retry delay or parked under an unreadable folder is counted. A job that
// names a video the audience does not list is not; a job that names none is (docs/design-docs/background-pause.md).
func (s *IngestStore) BackgroundRemaining(ctx context.Context, audience domain.Audience) (map[domain.BackgroundGroup]int, error) {
	unfinished := `jobs.state in ('queued', 'running') and not ` + namesUnlistedVideo(audience)
	remaining := map[domain.BackgroundGroup]int{}
	var videos int
	if err := s.db.sql.QueryRowContext(ctx, `select count(distinct jobs.video_id) from jobs where `+unfinished+
		` and jobs.kind in (`+importJobKinds+`)`).Scan(&videos); err != nil {
		return nil, fmt.Errorf("cannot count the remaining import work: %w", err)
	}
	remaining[domain.BackgroundImport] = videos
	rows, err := s.db.sql.QueryContext(ctx, `select jobs.kind, count(*) from jobs where `+unfinished+
		` and jobs.kind not in (`+importJobKinds+`) group by jobs.kind`)
	if err != nil {
		return nil, fmt.Errorf("cannot count the remaining background jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, fmt.Errorf("cannot count the remaining background jobs: %w", err)
		}
		if group, ok := domain.JobGroup(domain.JobKind(kind)); ok {
			remaining[group] += count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot count the remaining background jobs: %w", err)
	}
	return remaining, nil
}

// namesUnlistedVideo is true for a job whose video, named by id or by content key, is not listed for the audience.
func namesUnlistedVideo(audience domain.Audience) string {
	unlisted := `not ` + listedVideoCondition("nv", audience)
	return `(exists (select 1 from videos nv where nv.id = jobs.video_id and ` + unlisted + `) or ` +
		`(jobs.kind in (` + keySubjectJobKinds + `) and exists (select 1 from videos nv where nv.content_key <> '' ` +
		`and nv.content_key in (jobs.subject_key, jobs.subject_key_b) and ` + unlisted + `)))`
}

// VideoIDsByContentKey returns the id of the video holding each content key that has one.
func (s *IngestStore) VideoIDsByContentKey(ctx context.Context, keys []string) (map[string]int64, error) {
	ids := map[string]int64{}
	if len(keys) == 0 {
		return ids, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.sql.QueryContext(ctx, `select content_key, id from videos
		where content_key <> '' and content_key in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot look up the videos by content key: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("cannot look up the videos by content key: %w", err)
		}
		ids[key] = id
	}
	return ids, rows.Err()
}
