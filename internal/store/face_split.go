package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Read the size of a stored crop.
	"time"

	"github.com/syudead/vv/internal/domain"
)

// SplitFaceAppearance turns one appearance the analysis linked to the person into its own provisional person, in
// one transaction: the new person's face is the appearance's crop, the appearance and the video's performer credit
// move to it, and the pair of the two people is dismissed so neither automatic rule nor candidates join them
// again. See docs/design-docs/face-recognition.md. It returns the new person; a missing person is
// domain.ErrNotFound, and an appearance that is dropped or linked to another person
// domain.ErrFaceAppearanceNotFound.
func (s *CreditStore) SplitFaceAppearance(ctx context.Context, personID, appearanceID int64) (domain.Credit, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requirePerson(ctx, tx, personID); err != nil {
		return domain.Credit{}, err
	}
	contentKey, crop, err := readSplitAppearance(ctx, tx, personID, appearanceID)
	if err != nil {
		return domain.Credit{}, err
	}
	now := time.Now()
	newID, err := createFacePerson(ctx, tx, crop, now)
	if err != nil {
		return domain.Credit{}, err
	}
	if _, err := tx.ExecContext(ctx, `update face_appearances set person_id = ? where id = ?`, newID, appearanceID); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot move face appearance %d: %w", appearanceID, err)
	}
	if err := dismissFacePair(ctx, tx, newID, personID, now); err != nil {
		return domain.Credit{}, err
	}
	events, err := splitCreditEvents(ctx, tx, contentKey, personID, newID, now)
	if err != nil {
		return domain.Credit{}, err
	}
	created, err := creditByID(ctx, tx, creditTablesByKind[domain.CreditPerson], newID)
	if err != nil {
		return domain.Credit{}, err
	}
	// The new face is published even when no credit changed, such as for a video gone from the library.
	events = append(events, personFaceEvents(domain.Credit{}, created)...)
	c := &changes{}
	c.matchesChanged()
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot split face appearance %d from person %d: %w", appearanceID, personID, err)
	}
	return created, nil
}

// splitCreditEvents moves the performer credit and returns the events of the work it changed, if any.
func splitCreditEvents(ctx context.Context, tx *sql.Tx, contentKey string, oldID, newID int64, now time.Time) ([]domain.Event, error) {
	edited, err := moveFacePerformer(ctx, tx, contentKey, oldID, newID)
	if err != nil || edited == "" {
		return nil, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, []string{edited}, now); err != nil {
		return nil, err
	}
	return workInfoChangedEvents(ctx, tx, []string{edited})
}

// readSplitAppearance returns the video and the decoded crop of an appearance still linked to the person.
func readSplitAppearance(ctx context.Context, tx *sql.Tx, personID, appearanceID int64) (string, domain.PersonImage, error) {
	var contentKey string
	var jpeg []byte
	err := tx.QueryRowContext(ctx, `select content_key, crop from face_appearances
		where id = ? and person_id = ? and dropped = 0`, appearanceID, personID).Scan(&contentKey, &jpeg)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.PersonImage{}, domain.ErrFaceAppearanceNotFound
	}
	if err != nil {
		return "", domain.PersonImage{}, fmt.Errorf("cannot read face appearance %d: %w", appearanceID, err)
	}
	if len(jpeg) == 0 {
		return contentKey, domain.PersonImage{}, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(jpeg))
	if err != nil {
		return "", domain.PersonImage{}, fmt.Errorf("cannot read the crop of face appearance %d: %w", appearanceID, err)
	}
	return contentKey, domain.PersonImage{JPEG: jpeg, Width: config.Width, Height: config.Height}, nil
}

// moveFacePerformer gives the video's performer credit of the old person to the new one, in the same position,
// unless another appearance of the old person in the same work still links it; then the new person is appended. It
// returns the user key whose credits changed, or "" when none did or the video left the library.
func moveFacePerformer(ctx context.Context, tx *sql.Tx, contentKey string, oldID, newID int64) (string, error) {
	userKey, err := currentUserKey(ctx, tx, contentKey)
	if err != nil || userKey == "" {
		return "", err
	}
	linked, err := personLinkedInWork(ctx, tx, userKey, oldID)
	if err != nil {
		return "", err
	}
	if linked {
		added, err := addFacePerformer(ctx, tx, userKey, newID)
		if err != nil || !added {
			return "", err
		}
		return userKey, nil
	}
	res, err := tx.ExecContext(ctx, `update video_work_people set person_id = ?
		where content_key = ? and role = 'performer' and person_id = ?`, newID, userKey, oldID)
	if err != nil {
		return "", fmt.Errorf("cannot move the performer credit of person %d: %w", oldID, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return "", err
	}
	return userKey, nil
}

// personLinkedInWork reports whether an appearance not dropped, in any video of the work, still links the person.
func personLinkedInWork(ctx context.Context, tx *sql.Tx, userKey string, personID int64) (bool, error) {
	contentKeys, err := contentKeysForUserKeys(ctx, tx, []string{userKey})
	if err != nil {
		return false, err
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return false, fmt.Errorf("cannot build content keys: %w", err)
	}
	var linked bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from face_appearances where person_id = ?
		and dropped = 0 and content_key in (select value from json_each(?)))`, personID, string(encoded)).Scan(&linked); err != nil {
		return false, fmt.Errorf("cannot read the face appearances of person %d: %w", personID, err)
	}
	return linked, nil
}
