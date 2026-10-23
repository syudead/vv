package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

const similarPeopleSamples = 4

// SimilarPeople ranks the other people by how close their faces are to the given appearances of the person, or to all
// of the person's appearances without any, so the owner can pick where faces belong. Each person is compared by their
// face image's vector and by the mean of their faces in videos, whichever is closer. A missing person returns
// domain.ErrNotFound; appearances the person lacks are ignored, and with none left the list is empty.
func (s *CreditStore) SimilarPeople(ctx context.Context, personID int64, appearanceIDs []int64, limit int) (
	[]domain.SimilarPerson, error,
) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requirePerson(ctx, tx, personID); err != nil {
		return nil, err
	}
	centroids, err := centroidsByPerson(ctx, tx)
	if err != nil {
		return nil, err
	}
	var queryVectors [][]float32
	for id, v := range centroids[personID] {
		if len(appearanceIDs) == 0 || slices.Contains(appearanceIDs, id) {
			queryVectors = append(queryVectors, v)
		}
	}
	query := domain.MeanFaceVector(queryVectors)
	if query == nil {
		return []domain.SimilarPerson{}, nil
	}
	references, err := facePeopleReferences(ctx, tx, centroids)
	if err != nil {
		return nil, err
	}
	delete(references, personID)
	ranked := domain.RankFacePeople(query, references, limit)
	out := make([]domain.SimilarPerson, 0, len(ranked))
	for _, r := range ranked {
		p, err := similarPersonDetails(ctx, tx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// centroidsByPerson reads every centroid of an appearance not dropped, by person and appearance.
func centroidsByPerson(ctx context.Context, tx *sql.Tx) (map[int64]map[int64][]float32, error) {
	rows, err := tx.QueryContext(ctx, `select id, person_id, centroid from face_appearances
		 where dropped = 0 and person_id is not null and centroid is not null`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the face centroids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]map[int64][]float32{}
	for rows.Next() {
		var id, personID int64
		var blob []byte
		if err := rows.Scan(&id, &personID, &blob); err != nil {
			return nil, fmt.Errorf("cannot read the face centroids: %w", err)
		}
		if out[personID] == nil {
			out[personID] = map[int64][]float32{}
		}
		out[personID][id] = decodeFaceVector(blob)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the face centroids: %w", err)
	}
	return out, nil
}

// facePeopleReferences gives each person their current face image's vector, of any recognizer, and the mean of their
// centroids.
func facePeopleReferences(ctx context.Context, tx *sql.Tx, centroids map[int64]map[int64][]float32) (
	map[int64][][]float32, error,
) {
	out := map[int64][][]float32{}
	for personID, byAppearance := range centroids {
		vectors := make([][]float32, 0, len(byAppearance))
		for _, v := range byAppearance {
			vectors = append(vectors, v)
		}
		if mean := domain.MeanFaceVector(vectors); mean != nil {
			out[personID] = append(out[personID], mean)
		}
	}
	rows, err := tx.QueryContext(ctx, `select v.person_id, v.vector from person_face_vectors v
		join people p on p.id = v.person_id where v.vector is not null and v.image_revision = p.image_revision`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the face vectors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var personID int64
		var blob []byte
		if err := rows.Scan(&personID, &blob); err != nil {
			return nil, fmt.Errorf("cannot read the face vectors: %w", err)
		}
		out[personID] = append(out[personID], decodeFaceVector(blob))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the face vectors: %w", err)
	}
	return out, nil
}

// similarPersonDetails reads what tells a ranked person apart: the person, their face count, a few crops and the title
// of their first appearance's video.
func similarPersonDetails(ctx context.Context, tx *sql.Tx, r domain.FaceCandidate) (domain.SimilarPerson, error) {
	person, err := creditByID(ctx, tx, creditTablesByKind[domain.CreditPerson], r.PersonID)
	if err != nil {
		return domain.SimilarPerson{}, err
	}
	out := domain.SimilarPerson{Person: person, Similarity: r.Similarity, SampleAppearanceIDs: []int64{}}
	if err := tx.QueryRowContext(ctx, `select count(*) from face_appearances where person_id = ? and dropped = 0`,
		r.PersonID).Scan(&out.FaceCount); err != nil {
		return domain.SimilarPerson{}, fmt.Errorf("cannot count the faces of person %d: %w", r.PersonID, err)
	}
	// Newest video first, as the faces page orders them; a face whose video is gone comes last.
	rows, err := tx.QueryContext(ctx, `select a.id from face_appearances a left join videos v on v.content_key = a.content_key
		 where a.person_id = ? and a.dropped = 0 and a.crop is not null
		 order by `+appearanceAddedAt+` desc, a.id desc limit ?`, r.PersonID, similarPeopleSamples)
	if err != nil {
		return domain.SimilarPerson{}, fmt.Errorf("cannot read the crops of person %d: %w", r.PersonID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return domain.SimilarPerson{}, fmt.Errorf("cannot read the crops of person %d: %w", r.PersonID, err)
		}
		out.SampleAppearanceIDs = append(out.SampleAppearanceIDs, id)
	}
	if err := rows.Err(); err != nil {
		return domain.SimilarPerson{}, fmt.Errorf("cannot read the crops of person %d: %w", r.PersonID, err)
	}
	err = tx.QueryRowContext(ctx, `select `+contentLabelExpr("a.content_key")+` from face_appearances a
		 where a.person_id = ? and a.dropped = 0 order by a.id limit 1`, r.PersonID).Scan(&out.FirstSeenTitle)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.SimilarPerson{}, fmt.Errorf("cannot read the first video of person %d: %w", r.PersonID, err)
	}
	return out, nil
}
