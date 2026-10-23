package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A candidate pair is stored once, as (provisional_id, person_id); the API names it by its two people in either
// order, so every query here matches both orientations.
const facePairCondition = `((provisional_id = ?1 and person_id = ?2) or (provisional_id = ?2 and person_id = ?1))`

const openFacePairCondition = `dismissed = 0 and ` + facePairCondition

// FaceCandidates lists the open pairs the person takes part in, highest similarity first, as the other person of
// each; a missing person returns domain.ErrNotFound.
func (s *CreditStore) FaceCandidates(ctx context.Context, personID int64) ([]domain.FacePair, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requirePerson(ctx, tx, personID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `select o.id, coalesce(n.name, ''), n.person_id is null,
		`+personImageRevision("o.id")+`, c.similarity
		  from face_candidates c
		  join people o on o.id = case when c.provisional_id = ?1 then c.person_id else c.provisional_id end
		  left join people_names n on n.person_id = o.id and n.canonical = 1
		 where c.dismissed = 0 and (c.provisional_id = ?1 or c.person_id = ?1)
		 order by c.similarity desc, o.id`, personID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the face candidates of person %d: %w", personID, err)
	}
	defer func() { _ = rows.Close() }()
	out := []domain.FacePair{}
	for rows.Next() {
		var p domain.FacePair
		if err := rows.Scan(&p.Person.ID, &p.Person.Name, &p.Person.Provisional, &p.ImageRevision, &p.Similarity); err != nil {
			return nil, fmt.Errorf("cannot read the face candidates of person %d: %w", personID, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the face candidates of person %d: %w", personID, err)
	}
	return out, nil
}

// requirePerson returns domain.ErrNotFound when no person has the id.
func requirePerson(ctx context.Context, q rowQueryer, personID int64) error {
	var exists int
	if err := q.QueryRowContext(ctx, `select 1 from people where id = ?`, personID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("cannot read person %d: %w", personID, err)
	}
	return nil
}

// ConfirmFaceCandidate merges the other person of the open pair into personID through the ordinary merge, or personID
// into the other when only the other has a name, and returns the target; without an open pair it returns
// domain.ErrFaceCandidateNotFound. A merged person without a name is recorded as a face_merge decided by the owner,
// whose id the result carries so the faces page can undo it.
func (s *CreditStore) ConfirmFaceCandidate(ctx context.Context, personID, otherID int64) (domain.FaceCandidateConfirmation, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FaceCandidateConfirmation{}, err
	}
	defer func() { _ = tx.Rollback() }()

	sourceID, targetID, err := openFacePairDirection(ctx, tx, personID, otherID)
	if err != nil {
		return domain.FaceCandidateConfirmation{}, err
	}
	source, err := creditByID(ctx, tx, creditTablesByKind[domain.CreditPerson], sourceID)
	if err != nil {
		return domain.FaceCandidateConfirmation{}, err
	}
	c := &changes{}
	var result domain.FaceCandidateConfirmation
	var events []domain.Event
	// The undo snapshot keeps no external IDs, so a person holding some is merged like a named one, without undo.
	if source.Provisional && len(source.ExternalIDs) == 0 {
		result.Person, events, result.UndoActivityID, err = mergeProvisional(ctx, tx, c, sourceID, targetID,
			domain.ActivityByOwner, time.Now())
	} else {
		var outcome domain.CreditMergeOutcome
		outcome, events, err = mergeCreditsInTx(ctx, tx, creditTablesByKind[domain.CreditPerson], targetID, []int64{sourceID})
		result.Person = outcome.Credit
		c.matchesChanged()
	}
	if err != nil {
		return domain.FaceCandidateConfirmation{}, err
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.FaceCandidateConfirmation{}, fmt.Errorf("cannot merge person %d into person %d: %w", sourceID, targetID, err)
	}
	return result, nil
}

// openFacePairDirection returns the open pair's people as (source, target): the person the owner confirms from stays,
// unless only the other one has a canonical name, so a named person is never merged into one without a name.
func openFacePairDirection(ctx context.Context, tx *sql.Tx, personID, otherID int64) (int64, int64, error) {
	var personNamed, otherNamed bool
	err := tx.QueryRowContext(ctx, `select
		exists (select 1 from people_names n where n.person_id = ?1 and n.canonical = 1),
		exists (select 1 from people_names n where n.person_id = ?2 and n.canonical = 1)
		  from face_candidates where `+openFacePairCondition+`
		 limit 1`, personID, otherID).
		Scan(&personNamed, &otherNamed)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, 0, domain.ErrFaceCandidateNotFound
	case err != nil:
		return 0, 0, fmt.Errorf("cannot read the face candidate of people %d and %d: %w", personID, otherID, err)
	case otherNamed && !personNamed:
		return personID, otherID, nil
	default:
		return otherID, personID, nil
	}
}

// DismissFaceCandidate marks the open pair of the two people dismissed, so no later result offers it again; without
// an open pair it returns domain.ErrFaceCandidateNotFound.
func (s *CreditStore) DismissFaceCandidate(ctx context.Context, personID, otherID int64) error {
	return s.setFaceCandidateDismissed(ctx, personID, otherID, true)
}

// ReopenFaceCandidate takes back a dismissal of the pair of the two people, so it is offered again; without a
// dismissed pair it returns domain.ErrFaceCandidateNotFound.
func (s *CreditStore) ReopenFaceCandidate(ctx context.Context, personID, otherID int64) error {
	return s.setFaceCandidateDismissed(ctx, personID, otherID, false)
}

func (s *CreditStore) setFaceCandidateDismissed(ctx context.Context, personID, otherID int64, dismissed bool) error {
	action := "dismiss"
	if !dismissed {
		action = "reopen"
	}
	res, err := s.sql.ExecContext(ctx, `update face_candidates set dismissed = ?3
		 where dismissed = 1 - ?3 and `+facePairCondition, personID, otherID, dismissed)
	if err != nil {
		return fmt.Errorf("cannot %s the face candidate of people %d and %d: %w", action, personID, otherID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot %s the face candidate of people %d and %d: %w", action, personID, otherID, err)
	}
	if n == 0 {
		return domain.ErrFaceCandidateNotFound
	}
	return nil
}
