package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// WorkInfoFillTargets returns, in request order, the first library video of each user-data key among videoIDs with
// the key's work code, empty when it has none.
func (s *OverrideStore) WorkInfoFillTargets(ctx context.Context, videoIDs []int64) ([]domain.WorkInfoFillTarget, error) {
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	rows, err := s.db.sql.QueryContext(ctx, `select t.user_key, t.id, t.content_key, coalesce(w.code, '')
		from (select `+userKeyExpr("v")+` as user_key, v.id, v.content_key, j.key as at
			from json_each(?) j join videos v on v.id = j.value
			where v.content_key <> '' and `+libraryVideoCondition("v")+`) t
		left join video_work_info w on w.content_key = t.user_key
		order by t.at`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the fill targets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var targets []domain.WorkInfoFillTarget
	seen := map[string]bool{}
	for rows.Next() {
		var key string
		var target domain.WorkInfoFillTarget
		if err := rows.Scan(&key, &target.VideoID, &target.ContentKey, &target.Code); err != nil {
			return nil, fmt.Errorf("cannot read the fill targets: %w", err)
		}
		if !seen[key] {
			seen[key] = true
			targets = append(targets, target)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the fill targets: %w", err)
	}
	return targets, nil
}
