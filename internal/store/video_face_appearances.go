package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// VideoFaceAppearances lists the not-dropped appearances of the video's content key with their people, once its
// face analysis is done; a missing or hidden video returns domain.ErrNotFound.
func (s *CreditStore) VideoFaceAppearances(ctx context.Context, videoID int64) (domain.VideoFaceAppearances, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoFaceAppearances{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var key string
	out := domain.VideoFaceAppearances{Items: []domain.VideoFaceAppearance{}}
	err = tx.QueryRowContext(ctx, `select v.content_key, v.content_key <> '' and `+faceQueue.stateOf("v.content_key")+
		` = 'done' from videos v where v.id = ? and `+visibleVideoCondition("v", domain.AudienceOwner), videoID).
		Scan(&key, &out.Analysed)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VideoFaceAppearances{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.VideoFaceAppearances{}, fmt.Errorf("cannot read video %d: %w", videoID, err)
	}
	if !out.Analysed {
		return out, nil
	}
	rows, err := tx.QueryContext(ctx, `select a.id, a.person_id, coalesce(n.name, ''), n.person_id is null,
		`+personImageRevision("a.person_id")+`, a.crop is not null, a.tracks
		  from face_appearances a
		  left join people_names n on n.person_id = a.person_id and n.canonical = 1
		 where a.content_key = ? and a.dropped = 0 and a.person_id is not null`, key)
	if err != nil {
		return domain.VideoFaceAppearances{}, fmt.Errorf("cannot read the face appearances of video %d: %w", videoID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item domain.VideoFaceAppearance
		var tracks string
		if err := rows.Scan(&item.ID, &item.Person.ID, &item.Person.Name, &item.Person.Provisional,
			&item.ImageRevision, &item.HasCrop, &tracks); err != nil {
			return domain.VideoFaceAppearances{}, fmt.Errorf("cannot read the face appearances of video %d: %w", videoID, err)
		}
		spans, err := decodeFaceTracks(tracks)
		if err != nil {
			return domain.VideoFaceAppearances{}, fmt.Errorf("cannot read the tracks of face appearance %d: %w", item.ID, err)
		}
		item.Spans = domain.FaceTimeSpans(spans)
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.VideoFaceAppearances{}, fmt.Errorf("cannot read the face appearances of video %d: %w", videoID, err)
	}
	slices.SortFunc(out.Items, domain.CompareVideoFaceAppearances)
	return out, nil
}
