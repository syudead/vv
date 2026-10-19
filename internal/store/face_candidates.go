package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// A candidate pair is stored once, as (provisional_id, person_id); the API names it by its two people in either
// order, so every query here matches both orientations.
const openFacePairCondition = `dismissed = 0 and ((provisional_id = ?1 and person_id = ?2)
	or (provisional_id = ?2 and person_id = ?1))`

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

// ConfirmFaceCandidate merges the provisional person of the open pair of the two people into the other through the
// ordinary merge and returns the target; without an open pair it returns domain.ErrFaceCandidateNotFound.
func (s *CreditStore) ConfirmFaceCandidate(ctx context.Context, personID, otherID int64) (domain.Credit, error) {
	t, err := creditTablesFor(domain.CreditPerson)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()

	sourceID, targetID, err := openFacePairDirection(ctx, tx, personID, otherID)
	if err != nil {
		return domain.Credit{}, err
	}
	outcome, err := mergeCreditsInTx(ctx, tx, t, targetID, []int64{sourceID})
	if err != nil {
		return domain.Credit{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot merge person %d into person %d: %w", sourceID, targetID, err)
	}
	return outcome.Credit, nil
}

// openFacePairDirection returns the open pair's people as (source, target): the source is the one still without a
// canonical name, or the stored provisional side when both or neither are, since a name given later may change it.
func openFacePairDirection(ctx context.Context, tx *sql.Tx, personID, otherID int64) (int64, int64, error) {
	var provisionalID, otherSide int64
	var provisionalNamed, otherNamed bool
	err := tx.QueryRowContext(ctx, `select provisional_id, person_id,
		exists (select 1 from people_names n where n.person_id = provisional_id and n.canonical = 1),
		exists (select 1 from people_names n where n.person_id = face_candidates.person_id and n.canonical = 1)
		  from face_candidates where `+openFacePairCondition+`
		 order by similarity desc limit 1`, personID, otherID).
		Scan(&provisionalID, &otherSide, &provisionalNamed, &otherNamed)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, 0, domain.ErrFaceCandidateNotFound
	case err != nil:
		return 0, 0, fmt.Errorf("cannot read the face candidate of people %d and %d: %w", personID, otherID, err)
	case provisionalNamed && !otherNamed:
		return otherSide, provisionalID, nil
	default:
		return provisionalID, otherSide, nil
	}
}

// DismissFaceCandidate marks the open pair of the two people dismissed, so no later result offers it again; without
// an open pair it returns domain.ErrFaceCandidateNotFound.
func (s *CreditStore) DismissFaceCandidate(ctx context.Context, personID, otherID int64) error {
	res, err := s.sql.ExecContext(ctx, `update face_candidates set dismissed = 1 where `+openFacePairCondition,
		personID, otherID)
	if err != nil {
		return fmt.Errorf("cannot dismiss the face candidate of people %d and %d: %w", personID, otherID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot dismiss the face candidate of people %d and %d: %w", personID, otherID, err)
	}
	if n == 0 {
		return domain.ErrFaceCandidateNotFound
	}
	return nil
}
