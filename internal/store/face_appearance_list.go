package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// An appearance without a video sorts after every video, so the cursor value stays an integer.
const appearanceAddedAt = `coalesce(v.added_at, -1)`

// PersonAppearances lists the person's appearances that are not dropped in the query's order, each scored against
// the person's other faces; a missing person returns domain.ErrNotFound and an unreadable cursor
// domain.ErrInvalidCursor. A nonpositive limit returns every remaining appearance without a cursor.
func (s *CreditStore) PersonAppearances(ctx context.Context, personID int64, q domain.PersonAppearanceQuery) (
	domain.PersonAppearancePage, error,
) {
	compare := domain.ComparePersonAppearances(q.Sort)
	var after *domain.PersonAppearance
	if q.Cursor != "" {
		c, err := decodePersonAppearanceCursor(q.Cursor, q.Sort)
		if err != nil {
			return domain.PersonAppearancePage{}, err
		}
		after = &c
	}
	all, err := s.allPersonAppearances(ctx, personID)
	if err != nil {
		return domain.PersonAppearancePage{}, err
	}
	slices.SortFunc(all, compare)
	page := domain.PersonAppearancePage{Items: []domain.PersonAppearance{}, Total: len(all)}
	start := 0
	if after != nil {
		start = len(all)
		for i, item := range all {
			if compare(item, *after) > 0 {
				start = i
				break
			}
		}
	}
	rest := all[start:]
	if q.Limit > 0 && len(rest) > q.Limit {
		rest = rest[:q.Limit]
		page.NextCursor = encodePersonAppearanceCursor(q.Sort, rest[len(rest)-1])
	}
	page.Items = append(page.Items, rest...)
	return page, nil
}

// allPersonAppearances reads every appearance of the person that is not dropped, scored against the others.
func (s *CreditStore) allPersonAppearances(ctx context.Context, personID int64) ([]domain.PersonAppearance, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requirePerson(ctx, tx, personID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `select a.id, coalesce(v.id, 0), `+contentLabelExpr("a.content_key")+`, a.tracks,
		a.crop is not null, `+appearanceAddedAt+`, a.centroid
		  from face_appearances a left join videos v on v.content_key = a.content_key
		 where a.person_id = ? and a.dropped = 0`, personID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the appearances of person %d: %w", personID, err)
	}
	defer func() { _ = rows.Close() }()
	var items []domain.PersonAppearance
	vectors := map[int64][]float32{}
	for rows.Next() {
		var item domain.PersonAppearance
		var tracks string
		var centroid []byte
		if err := rows.Scan(&item.ID, &item.VideoID, &item.Title, &tracks, &item.HasCrop, &item.AddedAt, &centroid); err != nil {
			return nil, fmt.Errorf("cannot read the appearances of person %d: %w", personID, err)
		}
		spans, err := decodeFaceTracks(tracks)
		if err != nil {
			return nil, fmt.Errorf("cannot read the tracks of face appearance %d: %w", item.ID, err)
		}
		item.Spans = domain.FaceTimeSpans(spans)
		item.Focus = domain.FaceFocusOf(spans)
		if len(centroid) > 0 {
			vectors[item.ID] = decodeFaceVector(centroid)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the appearances of person %d: %w", personID, err)
	}
	scores := domain.FaceAppearanceSimilarities(vectors)
	for i := range items {
		if score, ok := scores[items[i].ID]; ok {
			items[i].Similarity = &score
			threshold, known := domain.FaceCandidateThreshold(len(vectors[items[i].ID]))
			items[i].Unlike = known && score < threshold
		}
	}
	return items, nil
}

// A cursor names the last appearance of a page by what its order compares: the score while it has one, the added
// time and the id.
func encodePersonAppearanceCursor(sort domain.PersonAppearanceSort, last domain.PersonAppearance) string {
	value := strconv.FormatInt(last.AddedAt, 10)
	if last.Similarity != nil {
		value += ":" + strconv.FormatFloat(*last.Similarity, 'g', -1, 64)
	}
	return encodeCursorFields(cursorFields{sort: personAppearanceCursorSort(sort), id: last.ID, value: value})
}

func decodePersonAppearanceCursor(cursor string, sort domain.PersonAppearanceSort) (domain.PersonAppearance, error) {
	c, err := decodeCursor(cursor)
	if err != nil {
		return domain.PersonAppearance{}, err
	}
	if c.sort != personAppearanceCursorSort(sort) || c.seed != "" || c.isNull {
		return domain.PersonAppearance{}, fmt.Errorf("%w: cursor is not for this order of person appearances",
			domain.ErrInvalidCursor)
	}
	added, score, scored := strings.Cut(c.value, ":")
	item := domain.PersonAppearance{ID: c.id}
	if item.AddedAt, err = strconv.ParseInt(added, 10, 64); err != nil {
		return domain.PersonAppearance{}, fmt.Errorf("%w: cannot parse the added time", domain.ErrInvalidCursor)
	}
	if scored {
		value, err := strconv.ParseFloat(score, 64)
		if err != nil {
			return domain.PersonAppearance{}, fmt.Errorf("%w: cannot parse the similarity", domain.ErrInvalidCursor)
		}
		item.Similarity = &value
	}
	return item, nil
}

func personAppearanceCursorSort(sort domain.PersonAppearanceSort) string {
	switch sort {
	case domain.PersonAppearancesLeastSimilar:
		return "personAppearanceSimilar"
	case domain.PersonAppearancesMostSimilar:
		return "personAppearanceMostSimilar"
	}
	return "personAppearance"
}

// FaceAppearanceCrop returns the appearance's crop; a missing appearance, one whose person was deleted, or one
// without a crop returns domain.ErrNotFound.
func (s *CreditStore) FaceAppearanceCrop(ctx context.Context, id int64) ([]byte, error) {
	var crop []byte
	err := s.read.QueryRowContext(ctx, `select crop from face_appearances
		 where id = ? and person_id is not null and crop is not null`, id).Scan(&crop)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the crop of face appearance %d: %w", id, err)
	}
	return crop, nil
}
