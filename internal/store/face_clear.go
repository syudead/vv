package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// ClearPeople deletes every person with every face appearance, and leaves every analysed video to be analysed
// again: the done and failed face results go, and while the feature is on every video is queued. face.generation
// increases, so a running analysis or face embedding started before the clear writes nothing. It returns the
// number of people deleted and of videos queued. See docs/design-docs/face-recognition.md.
func (s *FaceStore) ClearPeople(ctx context.Context) (domain.PeopleCleared, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.PeopleCleared{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var out domain.PeopleCleared
	if err := tx.QueryRowContext(ctx, `select count(*) from people`).Scan(&out.People); err != nil {
		return domain.PeopleCleared{}, fmt.Errorf("cannot count the people: %w", err)
	}
	generation, err := faceGeneration(ctx, tx)
	if err != nil {
		return domain.PeopleCleared{}, err
	}
	if err := writeSetting(ctx, tx, faceGenerationKey, strconv.FormatInt(generation+1, 10)); err != nil {
		return domain.PeopleCleared{}, err
	}
	// Appearances are deleted, not left with a null person: the next analysis would carry such a row over and never
	// link its face again.
	for _, stmt := range []string{
		`delete from face_appearances`,
		`delete from people`,
		`delete from face_results`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return domain.PeopleCleared{}, fmt.Errorf("cannot clear the people (%s): %w", stmt, err)
		}
	}
	c := &changes{}
	c.matchesChanged()
	enabled, err := readBoolSetting(ctx, tx, faceEnabledKey)
	if err != nil {
		return domain.PeopleCleared{}, err
	}
	if enabled {
		out.Queued, err = queueContentTx(ctx, tx, c, faceQueue, `select v.content_key from videos v
			where v.content_key <> '' and `+registeredVideoCondition("v"), nil, `''`, domain.ActivityByOwner, nil)
		if err != nil {
			return domain.PeopleCleared{}, err
		}
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return domain.PeopleCleared{}, fmt.Errorf("cannot clear the people: %w", err)
	}
	return out, nil
}
