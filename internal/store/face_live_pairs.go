package store

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// liveFacePairs ranks the people whose faces reach the recognizer's Candidate threshold against the person's, measured
// as the merge dialog measures them: the mean of the person's faces in videos, or their face image without any,
// against each other person's face image and the mean of their faces. Only pairs with a provisional side that the
// owner has not dismissed are kept. Stored pairs are written when an analysis or a face image is new, so a person
// whose faces grew through merges or moves, or whose face image is a poor match for their videos, would otherwise
// never be offered. See docs/design-docs/face-recognition.md.
func liveFacePairs(ctx context.Context, tx *sql.Tx, personID int64) ([]domain.FaceCandidate, error) {
	centroids, err := centroidsByPerson(ctx, tx, nil)
	if err != nil {
		return nil, err
	}
	references, err := facePeopleReferences(ctx, tx, centroids)
	if err != nil {
		return nil, err
	}
	query := meanCentroid(centroids[personID])
	if query == nil && len(references[personID]) > 0 {
		query = references[personID][0]
	}
	threshold, ok := domain.FaceCandidateThreshold(len(query))
	if !ok {
		return nil, nil
	}
	delete(references, personID)
	provisional, err := provisionalPeopleIDs(ctx, tx)
	if err != nil {
		return nil, err
	}
	dismissed, err := dismissedFacePartners(ctx, tx, personID)
	if err != nil {
		return nil, err
	}
	var out []domain.FaceCandidate
	for _, r := range domain.RankFacePeople(query, references, 0) {
		if r.Similarity < threshold {
			break
		}
		if !dismissed[r.PersonID] && (provisional[personID] || provisional[r.PersonID]) {
			out = append(out, r)
		}
	}
	return out, nil
}

func provisionalPeopleIDs(ctx context.Context, tx *sql.Tx) (map[int64]bool, error) {
	ids, err := queryInt64s(ctx, tx, `select p.id from people p
		where not exists (select 1 from people_names n where n.person_id = p.id and n.position = 0)`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the provisional people: %w", err)
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func dismissedFacePartners(ctx context.Context, tx *sql.Tx, personID int64) (map[int64]bool, error) {
	ids, err := queryInt64s(ctx, tx, `select case when provisional_id = ?1 then person_id else provisional_id end
		from face_candidates where dismissed = 1 and (provisional_id = ?1 or person_id = ?1)`, personID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the dismissed face candidates of person %d: %w", personID, err)
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// ensureLiveFacePair stores the pair of the two people when it has no row yet but the live ranking offers it, so
// confirming or dismissing a pair the faces page showed from that ranking works as for a stored one.
func ensureLiveFacePair(ctx context.Context, tx *sql.Tx, personID, otherID int64, now time.Time) error {
	var stored bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from face_candidates where `+facePairCondition+`)`,
		personID, otherID).Scan(&stored); err != nil {
		return fmt.Errorf("cannot read the face candidate of people %d and %d: %w", personID, otherID, err)
	}
	if stored {
		return nil
	}
	live, err := liveFacePairs(ctx, tx, personID)
	if err != nil {
		return err
	}
	for _, r := range live {
		if r.PersonID != otherID {
			continue
		}
		provisional, err := provisionalPeopleIDs(ctx, tx)
		if err != nil {
			return err
		}
		if provisional[personID] {
			return offerFacePair(ctx, tx, personID, otherID, r.Similarity, now)
		}
		return offerFacePair(ctx, tx, otherID, personID, r.Similarity, now)
	}
	return nil
}

// withLiveFacePairs adds to the stored pairs those the live ranking offers and no row holds, highest similarity first.
func withLiveFacePairs(ctx context.Context, tx *sql.Tx, personID int64, stored []domain.FacePair) ([]domain.FacePair, error) {
	live, err := liveFacePairs(ctx, tx, personID)
	if err != nil {
		return nil, err
	}
	rowed, err := queryInt64s(ctx, tx, `select case when provisional_id = ?1 then person_id else provisional_id end
		from face_candidates where provisional_id = ?1 or person_id = ?1`, personID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the face candidates of person %d: %w", personID, err)
	}
	has := make(map[int64]bool, len(rowed))
	for _, id := range rowed {
		has[id] = true
	}
	out := stored
	for _, r := range live {
		if has[r.PersonID] {
			continue
		}
		p := domain.FacePair{Similarity: r.Similarity}
		if err := tx.QueryRowContext(ctx, `select o.id, coalesce(n.name, ''), n.person_id is null, `+
			personImageRevision("o.id")+` from people o
			left join people_names n on n.person_id = o.id and n.position = 0 where o.id = ?1`, r.PersonID).
			Scan(&p.Person.ID, &p.Person.Name, &p.Person.Provisional, &p.ImageRevision); err != nil {
			return nil, fmt.Errorf("cannot read person %d: %w", r.PersonID, err)
		}
		out = append(out, p)
	}
	sortFacePairs(out)
	return out, nil
}

func sortFacePairs(pairs []domain.FacePair) {
	slices.SortStableFunc(pairs, func(a, b domain.FacePair) int {
		if c := cmp.Compare(b.Similarity, a.Similarity); c != 0 {
			return c
		}
		return cmp.Compare(a.Person.ID, b.Person.ID)
	})
}
