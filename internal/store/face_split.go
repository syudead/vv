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
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// SplitFaceAppearances turns appearances the analysis linked to the person into one new provisional person, in one
// transaction: the new person's face is the first appearance's crop, the appearances and their videos' performer
// credits move to it, and the pair of the two people is dismissed so neither automatic rule nor candidates join them
// again. See docs/design-docs/face-recognition.md. It returns the new person; a missing person is domain.ErrNotFound,
// and any appearance that is dropped or linked to another person domain.ErrFaceAppearanceNotFound, splitting none.
func (s *CreditStore) SplitFaceAppearances(ctx context.Context, personID int64, appearanceIDs []int64) (domain.Credit, error) {
	return s.relinkFaceAppearances(ctx, personID, appearanceIDs, 0)
}

// MoveFaceAppearances moves appearances of the person, with their videos' performer credits, to another existing
// person the owner names, in one transaction, and returns that person. A missing person is domain.ErrNotFound, the
// same person domain.ErrMergeSameCredit, and any appearance the person lacks domain.ErrFaceAppearanceNotFound.
func (s *CreditStore) MoveFaceAppearances(ctx context.Context, personID int64, appearanceIDs []int64, targetID int64) (
	domain.Credit, error,
) {
	if targetID == personID {
		return domain.Credit{}, domain.ErrMergeSameCredit
	}
	return s.relinkFaceAppearances(ctx, personID, appearanceIDs, targetID)
}

// relinkFaceAppearances moves the appearances to targetID, or to a new provisional person when targetID is 0.
func (s *CreditStore) relinkFaceAppearances(ctx context.Context, personID int64, appearanceIDs []int64, targetID int64) (
	domain.Credit, error,
) {
	if len(appearanceIDs) == 0 {
		return domain.Credit{}, domain.ErrFaceAppearanceNotFound
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range append([]int64{personID}, targetID) {
		if id == 0 {
			continue
		}
		if err := requirePerson(ctx, tx, id); err != nil {
			return domain.Credit{}, err
		}
	}
	contentKeys, crop, err := readSplitAppearances(ctx, tx, personID, appearanceIDs, targetID == 0)
	if err != nil {
		return domain.Credit{}, err
	}
	now := time.Now()
	created := targetID == 0
	if created {
		if targetID, err = createFacePerson(ctx, tx, crop, now); err != nil {
			return domain.Credit{}, err
		}
	}
	for _, id := range appearanceIDs {
		if _, err := tx.ExecContext(ctx, `update face_appearances set person_id = ? where id = ?`, targetID, id); err != nil {
			return domain.Credit{}, fmt.Errorf("cannot move face appearance %d: %w", id, err)
		}
	}
	if created {
		if err := dismissFacePair(ctx, tx, targetID, personID, now); err != nil {
			return domain.Credit{}, err
		}
	}
	events, err := splitCreditEvents(ctx, tx, contentKeys, personID, targetID, now)
	if err != nil {
		return domain.Credit{}, err
	}
	target, err := creditByID(ctx, tx, creditTablesByKind[domain.CreditPerson], targetID)
	if err != nil {
		return domain.Credit{}, err
	}
	if created {
		// The new face is published even when no credit changed, such as for a video gone from the library.
		events = append(events, personFaceEvents(domain.Credit{}, target)...)
	}
	c := &changes{}
	c.matchesChanged()
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot move face appearances %v from person %d: %w", appearanceIDs, personID, err)
	}
	return target, nil
}

// readSplitAppearances returns the videos of the appearances, all still linked to the person, and with withCrop the
// first one's crop.
func readSplitAppearances(ctx context.Context, tx *sql.Tx, personID int64, appearanceIDs []int64, withCrop bool) (
	[]string, domain.PersonImage, error,
) {
	contentKeys := make([]string, 0, len(appearanceIDs))
	var crop domain.PersonImage
	for i, id := range appearanceIDs {
		contentKey, appearanceCrop, err := readSplitAppearance(ctx, tx, personID, id, withCrop && i == 0)
		if err != nil {
			return nil, domain.PersonImage{}, err
		}
		if i == 0 {
			crop = appearanceCrop
		}
		contentKeys = append(contentKeys, contentKey)
	}
	return contentKeys, crop, nil
}

// splitCreditEvents moves the performer credits of the videos and returns the events of the works they changed.
func splitCreditEvents(ctx context.Context, tx *sql.Tx, contentKeys []string, oldID, newID int64, now time.Time) ([]domain.Event, error) {
	var edited []string
	for _, contentKey := range contentKeys {
		userKey, err := moveFacePerformer(ctx, tx, contentKey, oldID, newID)
		if err != nil {
			return nil, err
		}
		if userKey != "" && !slices.Contains(edited, userKey) {
			edited = append(edited, userKey)
		}
	}
	if len(edited) == 0 {
		return nil, nil
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return nil, err
	}
	return workInfoChangedEvents(ctx, tx, edited)
}

// readSplitAppearance returns the video of an appearance still linked to the person, and with withCrop its decoded crop.
func readSplitAppearance(ctx context.Context, tx *sql.Tx, personID, appearanceID int64, withCrop bool) (
	string, domain.PersonImage, error,
) {
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
	if !withCrop || len(jpeg) == 0 {
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
	// A person the owner moves faces to may already be credited; then the old person's row goes instead.
	changed := int64(0)
	for _, query := range []string{
		`update or ignore video_work_people set person_id = ?3 where content_key = ?1 and role = 'performer' and person_id = ?2`,
		`delete from video_work_people where content_key = ?1 and role = 'performer' and person_id = ?2`,
	} {
		res, err := tx.ExecContext(ctx, query, userKey, oldID, newID)
		if err != nil {
			return "", fmt.Errorf("cannot move the performer credit of person %d: %w", oldID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return "", fmt.Errorf("cannot move the performer credit of person %d: %w", oldID, err)
		}
		changed += n
	}
	if changed == 0 {
		return "", nil
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
