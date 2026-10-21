package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

const personAppearanceCursorSort = "personAppearance"

// An appearance without a video sorts after every video, so the cursor value stays an integer.
const appearanceAddedAt = `coalesce(v.added_at, -1)`

// PersonAppearances lists the person's appearances that are not dropped, newest video first, then by descending id;
// a missing person returns domain.ErrNotFound and an unreadable cursor domain.ErrInvalidCursor. A nonpositive
// limit returns every remaining appearance without a cursor.
func (s *CreditStore) PersonAppearances(ctx context.Context, personID int64, cursor string, limit int) (domain.PersonAppearancePage, error) {
	where, args := `a.person_id = ? and a.dropped = 0`, []any{personID}
	if cursor != "" {
		addedAt, id, err := decodePersonAppearanceCursor(cursor)
		if err != nil {
			return domain.PersonAppearancePage{}, err
		}
		where += ` and (` + appearanceAddedAt + `, a.id) < (?, ?)`
		args = append(args, addedAt, id)
	}
	query := `select a.id, coalesce(v.id, 0), ` + contentLabelExpr("a.content_key") + `, a.tracks, a.crop is not null,
		` + appearanceAddedAt + `
		  from face_appearances a left join videos v on v.content_key = a.content_key
		 where ` + where + ` order by ` + appearanceAddedAt + ` desc, a.id desc`
	if limit > 0 {
		query += ` limit ?`
		args = append(args, limit+1)
	}

	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.PersonAppearancePage{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requirePerson(ctx, tx, personID); err != nil {
		return domain.PersonAppearancePage{}, err
	}
	var total int
	if err := tx.QueryRowContext(ctx, `select count(*) from face_appearances where person_id = ? and dropped = 0`,
		personID).Scan(&total); err != nil {
		return domain.PersonAppearancePage{}, fmt.Errorf("cannot count the appearances of person %d: %w", personID, err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.PersonAppearancePage{}, fmt.Errorf("cannot read the appearances of person %d: %w", personID, err)
	}
	defer func() { _ = rows.Close() }()
	page := domain.PersonAppearancePage{Items: []domain.PersonAppearance{}, Total: total}
	var addedAts []int64
	for rows.Next() {
		var item domain.PersonAppearance
		var tracks string
		var addedAt int64
		if err := rows.Scan(&item.ID, &item.VideoID, &item.Title, &tracks, &item.HasCrop, &addedAt); err != nil {
			return domain.PersonAppearancePage{}, fmt.Errorf("cannot read the appearances of person %d: %w", personID, err)
		}
		spans, err := decodeFaceTracks(tracks)
		if err != nil {
			return domain.PersonAppearancePage{}, fmt.Errorf("cannot read the tracks of face appearance %d: %w", item.ID, err)
		}
		item.Spans = domain.FaceTimeSpans(spans)
		page.Items = append(page.Items, item)
		addedAts = append(addedAts, addedAt)
	}
	if err := rows.Err(); err != nil {
		return domain.PersonAppearancePage{}, fmt.Errorf("cannot read the appearances of person %d: %w", personID, err)
	}
	if limit > 0 && len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeCursorFields(cursorFields{
			sort: personAppearanceCursorSort, id: page.Items[limit-1].ID, value: strconv.FormatInt(addedAts[limit-1], 10),
		})
	}
	return page, nil
}

func decodePersonAppearanceCursor(cursor string) (addedAt, id int64, err error) {
	c, err := decodeCursor(cursor)
	if err != nil {
		return 0, 0, err
	}
	if c.sort != personAppearanceCursorSort || c.seed != "" || c.isNull {
		return 0, 0, fmt.Errorf("%w: cursor is not for person appearances", domain.ErrInvalidCursor)
	}
	addedAt, err = strconv.ParseInt(c.value, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: cannot parse the added time", domain.ErrInvalidCursor)
	}
	return addedAt, c.id, nil
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
