package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// MergeByCast applies the cast rule to the video's work in one transaction, reading its performers, their face
// links and the dismissed pairs inside it; it reports whether a provisional person was merged as a face_merge. A
// video no longer in the library returns domain.ErrNotFound, and a merge refused for good wraps
// domain.ErrFaceMergeRefused.
func (s *FaceStore) MergeByCast(ctx context.Context, videoID int64) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	keys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return false, err
	}
	if len(keys) == 0 {
		return false, domain.ErrNotFound
	}
	c := &changes{}
	merged, events, err := mergeByCast(ctx, tx, c, keys[0], time.Now())
	if err != nil || !merged {
		return false, err
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return false, fmt.Errorf("cannot merge by the cast of video %d: %w", videoID, err)
	}
	return true, nil
}

// mergeByCast merges the work's only provisional performer linked by face into its only performer without a face,
// as domain.DecideCastMerge decides, and returns the events of a face the target took; see
// docs/design-docs/face-recognition.md. A merge refused for good is rolled back to before it, so tx stays usable,
// and wraps domain.ErrFaceMergeRefused.
func mergeByCast(ctx context.Context, tx *sql.Tx, c *changes, userKey string, now time.Time) (bool, []domain.Event, error) {
	performers, err := faceCastPerformers(ctx, tx, userKey)
	if err != nil {
		return false, nil, err
	}
	dismissed, err := dismissedFacePairs(ctx, tx, performers)
	if err != nil {
		return false, nil, err
	}
	pair, ok := domain.DecideCastMerge(performers, dismissed)
	if !ok {
		return false, nil, nil
	}
	events, err := mergeProvisionalOrRefuse(ctx, tx, c, pair.ProvisionalID, pair.PersonID, now)
	if err != nil {
		return false, nil, err
	}
	return true, events, nil
}

// mergeProvisionalOrRefuse runs mergeProvisional for an automatic merge. A merge refused for good is rolled back to
// before it, so tx stays usable, and wraps domain.ErrFaceMergeRefused.
func mergeProvisionalOrRefuse(ctx context.Context, tx *sql.Tx, c *changes, provisionalID, personID int64,
	now time.Time,
) ([]domain.Event, error) {
	if _, err := tx.ExecContext(ctx, `savepoint face_merge`); err != nil {
		return nil, fmt.Errorf("cannot start the automatic merge: %w", err)
	}
	_, events, _, err := mergeProvisional(ctx, tx, c, provisionalID, personID, domain.ActivityByRule, now)
	if err != nil {
		if !mergeRefused(err) {
			return nil, err
		}
		// The refusal comes before the merge touches c, so only the rows need rolling back.
		if _, rbErr := tx.ExecContext(ctx, `rollback to face_merge`); rbErr != nil {
			return nil, fmt.Errorf("cannot roll back the automatic merge: %w", rbErr)
		}
		return nil, fmt.Errorf("%w: person %d into person %d: %w", domain.ErrFaceMergeRefused, provisionalID, personID,
			err)
	}
	if _, err := tx.ExecContext(ctx, `release face_merge`); err != nil {
		return nil, fmt.Errorf("cannot finish the automatic merge: %w", err)
	}
	return events, nil
}

// mergeRefused reports a refusal of mergeProvisional that does not depend on the database being reachable.
func mergeRefused(err error) bool {
	var invalid *domain.InvalidWorkInfoError
	return errors.As(err, &invalid) || errors.Is(err, domain.ErrPersonNotProvisional) ||
		errors.Is(err, domain.ErrMergeSameCredit)
}

// faceCastPerformers reads the work's performers; a performer is linked by face when an appearance that is not
// dropped, in any video sharing the work, is theirs.
func faceCastPerformers(ctx context.Context, tx *sql.Tx, userKey string) ([]domain.FaceCastPerformer, error) {
	contentKeys, err := contentKeysForUserKeys(ctx, tx, []string{userKey})
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select w.person_id,
		not exists (select 1 from people_names n where n.person_id = w.person_id and n.canonical = 1),
		exists (select 1 from face_appearances a where a.person_id = w.person_id and a.dropped = 0
			and a.content_key in (select value from json_each(?2)))
		from video_work_people w where w.content_key = ?1 and w.role = ?3 order by w.position`,
		userKey, string(encoded), string(domain.PersonPerformer))
	if err != nil {
		return nil, fmt.Errorf("cannot read the performers of %s: %w", userKey, err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.FaceCastPerformer
	for rows.Next() {
		var p domain.FaceCastPerformer
		if err := rows.Scan(&p.PersonID, &p.Provisional, &p.FaceLinked); err != nil {
			return nil, fmt.Errorf("cannot read the performers of %s: %w", userKey, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the performers of %s: %w", userKey, err)
	}
	return out, nil
}

// dismissedFacePairs returns the pairs of the performers the owner dismissed, in both orientations, since a pair
// is dismissed whichever of its people was provisional when it was offered.
func dismissedFacePairs(ctx context.Context, tx *sql.Tx, performers []domain.FaceCastPerformer) (map[domain.FacePersonPair]bool, error) {
	ids := make([]int64, len(performers))
	for i, p := range performers {
		ids[i] = p.PersonID
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build person ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select provisional_id, person_id from face_candidates
		where dismissed = 1 and provisional_id in (select value from json_each(?1))
		  and person_id in (select value from json_each(?1))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the dismissed face candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	dismissed := map[domain.FacePersonPair]bool{}
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, fmt.Errorf("cannot read the dismissed face candidates: %w", err)
		}
		dismissed[domain.FacePersonPair{ProvisionalID: a, PersonID: b}] = true
		dismissed[domain.FacePersonPair{ProvisionalID: b, PersonID: a}] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the dismissed face candidates: %w", err)
	}
	return dismissed, nil
}
