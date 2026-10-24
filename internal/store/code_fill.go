package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// CodedVideos returns, in request order, the first library video of each user-data key among videoIDs that has a
// work code, with the code, and the number of user-data keys the videos have, coded or not.
func (s *OverrideStore) CodedVideos(ctx context.Context, videoIDs []int64) ([]domain.CodedVideo, int, error) {
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot build video ids: %w", err)
	}
	rows, err := s.db.sql.QueryContext(ctx, `select t.user_key, t.id, t.content_key, coalesce(w.code, '')
		from (select `+userKeyExpr("v")+` as user_key, v.id, v.content_key, j.key as at
			from json_each(?) j join videos v on v.id = j.value
			where v.content_key <> '' and `+libraryVideoCondition("v")+`) t
		left join video_work_info w on w.content_key = t.user_key
		order by t.at`, string(encoded))
	if err != nil {
		return nil, 0, fmt.Errorf("cannot read the videos' codes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var coded []domain.CodedVideo
	seen := map[string]bool{}
	for rows.Next() {
		var key string
		var video domain.CodedVideo
		if err := rows.Scan(&key, &video.VideoID, &video.ContentKey, &video.Code); err != nil {
			return nil, 0, fmt.Errorf("cannot read the videos' codes: %w", err)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if video.Code != "" {
			coded = append(coded, video)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("cannot read the videos' codes: %w", err)
	}
	return coded, len(seen), nil
}
