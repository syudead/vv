package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// ProvisionalMatches lists the provisional performers of the listed registered videos and how they relate to the
// people being added, for the bulk editor's merge suggestions; see docs/design-docs/bulk-editing.md.
func (s *VideoEditStore) ProvisionalMatches(ctx context.Context, videoIDs, personIDs []int64) ([]domain.ProvisionalMatch, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the provisional performers: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return nil, err
	}
	byVideo, err := provisionalPerformersByKey(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	matches := domain.ProvisionalPerformers(byVideo)
	if len(matches) == 0 || len(personIDs) == 0 {
		return matches, nil
	}
	if err := addFaceSimilarity(ctx, tx, matches, personIDs); err != nil {
		return nil, err
	}
	return matches, nil
}

// provisionalPerformersByKey reads, for each user key with any, its provisional performers.
func provisionalPerformersByKey(ctx context.Context, tx *sql.Tx, keys []string) ([][]int64, error) {
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select w.content_key, w.person_id from video_work_people w
		 where w.role = ? and w.content_key in (select value from json_each(?))
		   and not exists (select 1 from people_names n where n.person_id = w.person_id and n.position = 0)
		 order by w.content_key, w.person_id`, string(domain.PersonPerformer), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the provisional performers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out [][]int64
	last := ""
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("cannot read the provisional performers: %w", err)
		}
		if len(out) == 0 || key != last {
			out = append(out, nil)
			last = key
		}
		out[len(out)-1] = append(out[len(out)-1], id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the provisional performers: %w", err)
	}
	return out, nil
}

// addFaceSimilarity fills SimilarTo: the mean of each provisional person's faces against each added person's face
// and the mean of their faces, at the recognizer's merge threshold, leaving out pairs the owner dismissed.
func addFaceSimilarity(ctx context.Context, tx *sql.Tx, matches []domain.ProvisionalMatch, personIDs []int64) error {
	ids := slices.Clone(personIDs)
	for _, m := range matches {
		ids = append(ids, m.PersonID)
	}
	centroids, err := centroidsByPerson(ctx, tx, ids)
	if err != nil {
		return err
	}
	references, err := facePeopleReferences(ctx, tx, centroids)
	if err != nil {
		return err
	}
	added := make(map[int64][][]float32, len(personIDs))
	for _, id := range personIDs {
		if refs, ok := references[id]; ok {
			added[id] = refs
		}
	}
	people := make([]domain.FaceCastPerformer, len(ids))
	for i, id := range ids {
		people[i] = domain.FaceCastPerformer{PersonID: id}
	}
	dismissed, err := dismissedFacePairs(ctx, tx, people)
	if err != nil {
		return err
	}
	for i := range matches {
		query := meanCentroid(centroids[matches[i].PersonID])
		threshold, ok := domain.FaceMergeThreshold(len(query))
		if !ok {
			continue
		}
		for _, r := range domain.RankFacePeople(query, added, 0) {
			if r.Similarity >= threshold && !dismissed[domain.FacePersonPair{ProvisionalID: matches[i].PersonID, PersonID: r.PersonID}] {
				matches[i].SimilarTo = append(matches[i].SimilarTo, r.PersonID)
			}
		}
	}
	return nil
}

func meanCentroid(byAppearance map[int64][]float32) []float32 {
	vectors := make([][]float32, 0, len(byAppearance))
	for _, v := range byAppearance {
		vectors = append(vectors, v)
	}
	return domain.MeanFaceVector(vectors)
}
