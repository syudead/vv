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
	moved, err := s.relinkFaceAppearances(ctx, personID, appearanceIDs, 0, nil)
	return moved.Person, err
}

// MoveFaceAppearances moves appearances of the person, with their videos' performer credits, to another existing
// person the owner names, in one transaction, and returns that person with the credits only the reverse move can put
// back. restore is what the move being undone returned: in those works the target's credit stays and the person's
// comes back at its old position. A missing person is domain.ErrNotFound, the same person
// domain.ErrMergeSameCredit, and any appearance the person lacks domain.ErrFaceAppearanceNotFound.
func (s *CreditStore) MoveFaceAppearances(
	ctx context.Context, personID int64, appearanceIDs []int64, targetID int64, restore []domain.FaceMoveCredit,
) (domain.FaceMove, error) {
	if targetID == personID {
		return domain.FaceMove{}, domain.ErrMergeSameCredit
	}
	return s.relinkFaceAppearances(ctx, personID, appearanceIDs, targetID, restore)
}

// relinkFaceAppearances moves the appearances to targetID, or to a new provisional person when targetID is 0.
func (s *CreditStore) relinkFaceAppearances(
	ctx context.Context, personID int64, appearanceIDs []int64, targetID int64, restore []domain.FaceMoveCredit,
) (domain.FaceMove, error) {
	if len(appearanceIDs) == 0 {
		return domain.FaceMove{}, domain.ErrFaceAppearanceNotFound
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FaceMove{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range append([]int64{personID}, targetID) {
		if id == 0 {
			continue
		}
		if err := requirePerson(ctx, tx, id); err != nil {
			return domain.FaceMove{}, err
		}
	}
	contentKeys, crop, err := readSplitAppearances(ctx, tx, personID, appearanceIDs, targetID == 0)
	if err != nil {
		return domain.FaceMove{}, err
	}
	now := time.Now()
	created := targetID == 0
	if created {
		if targetID, err = createFacePerson(ctx, tx, crop, now); err != nil {
			return domain.FaceMove{}, err
		}
	}
	for _, id := range appearanceIDs {
		if _, err := tx.ExecContext(ctx, `update face_appearances set person_id = ? where id = ?`, targetID, id); err != nil {
			return domain.FaceMove{}, fmt.Errorf("cannot move face appearance %d: %w", id, err)
		}
	}
	if created {
		if err := dismissFacePair(ctx, tx, targetID, personID, now); err != nil {
			return domain.FaceMove{}, err
		}
	}
	events, taken, err := splitCreditEvents(ctx, tx, contentKeys, personID, targetID, restore, now)
	if err != nil {
		return domain.FaceMove{}, err
	}
	target, err := creditByID(ctx, tx, creditTablesByKind[domain.CreditPerson], targetID)
	if err != nil {
		return domain.FaceMove{}, err
	}
	if created {
		// The new face is published even when no credit changed, such as for a video gone from the library.
		events = append(events, personFaceEvents(domain.Credit{}, target)...)
	}
	c := &changes{}
	c.matchesChanged()
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.FaceMove{}, fmt.Errorf("cannot move face appearances %v from person %d: %w", appearanceIDs, personID, err)
	}
	return domain.FaceMove{Person: target, Restore: taken}, nil
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

// splitCreditEvents moves the performer credits of the videos, or puts back those restore names, and returns the
// events of the works they changed with the credits taken from oldID that only a reverse move can put back.
func splitCreditEvents(
	ctx context.Context, tx *sql.Tx, contentKeys []string, oldID, newID int64, restore []domain.FaceMoveCredit, now time.Time,
) ([]domain.Event, []domain.FaceMoveCredit, error) {
	var edited []string
	var taken []domain.FaceMoveCredit
	for _, contentKey := range contentKeys {
		userKey, took, err := relinkFacePerformer(ctx, tx, contentKey, oldID, newID, restore)
		if err != nil {
			return nil, nil, err
		}
		if took != nil {
			taken = append(taken, *took)
		}
		if userKey != "" && !slices.Contains(edited, userKey) {
			edited = append(edited, userKey)
		}
	}
	if len(edited) == 0 {
		return nil, taken, nil
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return nil, nil, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	return events, taken, err
}

// relinkFacePerformer moves the video's performer credit, unless restore names its work and newID is not credited
// there while oldID is: then newID's credit comes back at its old position and oldID keeps its own.
func relinkFacePerformer(
	ctx context.Context, tx *sql.Tx, contentKey string, oldID, newID int64, restore []domain.FaceMoveCredit,
) (string, *domain.FaceMoveCredit, error) {
	userKey, err := currentUserKey(ctx, tx, contentKey)
	if err != nil || userKey == "" {
		return "", nil, err
	}
	for _, credit := range restore {
		if credit.UserKey != userKey {
			continue
		}
		restored, err := restoreFacePerformer(ctx, tx, userKey, oldID, newID, credit.Position)
		if err != nil || restored {
			return userKey, nil, err
		}
	}
	return moveFacePerformer(ctx, tx, userKey, oldID, newID)
}

// restoreFacePerformer inserts newID's performer credit at position, shifting the later ones, when newID is not
// credited in the work and oldID is; it reports whether it did.
func restoreFacePerformer(ctx context.Context, tx *sql.Tx, userKey string, oldID, newID int64, position int) (bool, error) {
	var credited, holds, count int
	if err := tx.QueryRowContext(ctx, `select coalesce(sum(person_id = ?2), 0), coalesce(sum(person_id = ?3), 0), count(*)
		  from video_work_people where content_key = ?1 and role = 'performer'`, userKey, newID, oldID).
		Scan(&credited, &holds, &count); err != nil {
		return false, fmt.Errorf("cannot read the performers of %s: %w", userKey, err)
	}
	if credited > 0 || holds == 0 {
		return false, nil
	}
	// The list may have shortened since, and positions stay compact.
	position = min(position, count)
	// Positions are part of the key, so the later rows step aside through negative values.
	for _, query := range []string{
		`update video_work_people set position = -position - 2 where content_key = ?1 and role = 'performer' and position >= ?2`,
		`update video_work_people set position = -position - 1 where content_key = ?1 and role = 'performer' and position < 0`,
		`insert into video_work_people (content_key, role, position, person_id) values (?1, 'performer', ?2, ?3)`,
	} {
		if _, err := tx.ExecContext(ctx, query, userKey, position, newID); err != nil {
			return false, fmt.Errorf("cannot put back the performer credit of person %d: %w", newID, err)
		}
	}
	return true, nil
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

// moveFacePerformer gives the work's performer credit of the old person to the new one, in the same position,
// unless another appearance of the old person in the same work still links it; then the new person is appended. It
// returns the user key when its credits changed, and the old person's credit when it was removed because the new
// person already held one.
func moveFacePerformer(ctx context.Context, tx *sql.Tx, userKey string, oldID, newID int64) (
	string, *domain.FaceMoveCredit, error,
) {
	linked, err := personLinkedInWork(ctx, tx, userKey, oldID)
	if err != nil {
		return "", nil, err
	}
	if linked {
		added, err := addFacePerformer(ctx, tx, userKey, newID)
		if err != nil || !added {
			return "", nil, err
		}
		return userKey, nil, nil
	}
	var position int
	err = tx.QueryRowContext(ctx, `select position from video_work_people
		where content_key = ? and role = 'performer' and person_id = ?`, userKey, oldID).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("cannot read the performer credit of person %d: %w", oldID, err)
	}
	// A person the owner moves faces to may already be credited; then the old person's row goes instead.
	res, err := tx.ExecContext(ctx, `update or ignore video_work_people set person_id = ?3
		where content_key = ?1 and role = 'performer' and person_id = ?2`, userKey, oldID, newID)
	if err != nil {
		return "", nil, fmt.Errorf("cannot move the performer credit of person %d: %w", oldID, err)
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return userKey, nil, err
	}
	if _, err := removeFacePerformer(ctx, tx, userKey, oldID); err != nil {
		return "", nil, err
	}
	return userKey, &domain.FaceMoveCredit{UserKey: userKey, Position: position}, nil
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
