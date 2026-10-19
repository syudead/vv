package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A person's face image is embedded by the face worker and applied by the face-image rule; see
// docs/design-docs/face-recognition.md.

// FaceImage returns the person's face image with its stored vector, or domain.ErrNotFound when the person is gone or
// has no face.
func (s *FaceStore) FaceImage(ctx context.Context, personID int64) (domain.FaceImageJob, error) {
	job, err := s.readFaceImageJob(ctx, `p.id = ?2`, personID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FaceImageJob{}, domain.ErrNotFound
	}
	return job, err
}

// StaleFaceImage returns a face image without a vector of its revision and the current recognizer, or
// domain.ErrNoJob when every face has one.
func (s *FaceStore) StaleFaceImage(ctx context.Context) (domain.FaceImageJob, error) {
	job, err := s.readFaceImageJob(ctx,
		`v.person_id is null or v.image_revision <> p.image_revision or v.recognizer <> ?1`)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FaceImageJob{}, domain.ErrNoJob
	}
	return job, err
}

// readFaceImageJob reads the recognizer and generation with the first face matching condition, in which ?1 is the
// recognizer and args follow from ?2; it returns sql.ErrNoRows when none matches.
func (s *FaceStore) readFaceImageJob(ctx context.Context, condition string, args ...any) (domain.FaceImageJob, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.FaceImageJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var job domain.FaceImageJob
	if job.Recognizer, err = faceRecognizer(ctx, tx); err != nil {
		return domain.FaceImageJob{}, err
	}
	if job.Generation, err = faceGeneration(ctx, tx); err != nil {
		return domain.FaceImageJob{}, err
	}
	var vector []byte
	err = tx.QueryRowContext(ctx, `select p.id, p.image_revision, i.image,
			coalesce(v.image_revision = p.image_revision and v.recognizer = ?1, 0), v.vector
		 from people p join person_images i on i.person_id = p.id
		 left join person_face_vectors v on v.person_id = p.id
		 where `+condition+` order by p.id limit 1`, append([]any{string(job.Recognizer)}, args...)...).
		Scan(&job.PersonID, &job.Revision, &job.JPEG, &job.Embedded, &vector)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.FaceImageJob{}, err
		}
		return domain.FaceImageJob{}, fmt.Errorf("cannot read a face image: %w", err)
	}
	if job.Embedded && vector != nil {
		job.Vector = decodeFaceVector(vector)
	}
	return job, nil
}

// StoreFaceImageVector stores the vector of the job's face, nil when it had no single good face, and applies the
// face-image rule in the same transaction: the only provisional person at Merge is merged into a named person as a
// face_merge, and the others from Candidate up are offered. A merge refused for good is left out: the rest is stored
// and the error wraps domain.ErrFaceMergeRefused. A job of an older face.generation returns
// domain.ErrFaceGenerationChanged; one whose face changed or went since writes nothing.
func (s *FaceStore) StoreFaceImageVector(ctx context.Context, job domain.FaceImageJob, vector []float32) (
	domain.FaceImageDecision, error,
) {
	spec, ok := domain.LookupFaceRecognizer(job.Recognizer)
	if !ok {
		return domain.FaceImageDecision{}, fmt.Errorf("unknown face recognizer %q", job.Recognizer)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FaceImageDecision{}, err
	}
	defer func() { _ = tx.Rollback() }()
	generation, err := faceGeneration(ctx, tx)
	if err != nil {
		return domain.FaceImageDecision{}, err
	}
	if generation != job.Generation {
		return domain.FaceImageDecision{}, domain.ErrFaceGenerationChanged
	}
	var revision int64
	err = tx.QueryRowContext(ctx, `select p.image_revision from people p join person_images i on i.person_id = p.id
		where p.id = ?`, job.PersonID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) || err == nil && revision != job.Revision {
		// The next job reads the face as it is now.
		return domain.FaceImageDecision{}, nil
	}
	if err != nil {
		return domain.FaceImageDecision{}, fmt.Errorf("cannot read the face of person %d: %w", job.PersonID, err)
	}
	if !job.Embedded {
		if _, err := tx.ExecContext(ctx, `insert into person_face_vectors (person_id, image_revision, recognizer, vector)
			values (?, ?, ?, ?) on conflict (person_id) do update set image_revision = excluded.image_revision,
			recognizer = excluded.recognizer, vector = excluded.vector`,
			job.PersonID, job.Revision, string(job.Recognizer), nullBytes(encodeFaceVector(vector))); err != nil {
			return domain.FaceImageDecision{}, fmt.Errorf("cannot store the face vector of person %d: %w", job.PersonID, err)
		}
	}
	c := &changes{}
	decision, events, err := applyFaceImage(ctx, tx, c, spec, job.PersonID, vector)
	refused := errors.Is(err, domain.ErrFaceMergeRefused)
	if err != nil && !refused {
		return domain.FaceImageDecision{}, err
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.FaceImageDecision{}, fmt.Errorf("cannot store the face vector of person %d: %w", job.PersonID, err)
	}
	if refused {
		// Without the vector the face would stay stale and be embedded again, and the merge refused again.
		decision.MergeID = 0
		return decision, err
	}
	return decision, nil
}

// applyFaceImage decides what the person's face does to provisional people and writes it.
func applyFaceImage(ctx context.Context, tx *sql.Tx, c *changes, spec domain.FaceRecognizerSpec, personID int64,
	vector []float32,
) (domain.FaceImageDecision, []domain.Event, error) {
	if vector == nil {
		return domain.FaceImageDecision{}, nil, nil
	}
	person, err := creditByID(ctx, tx, creditTablesByKind[domain.CreditPerson], personID)
	if err != nil {
		return domain.FaceImageDecision{}, nil, err
	}
	provisional, err := provisionalFaceVectors(ctx, tx, spec.Recognizer, personID)
	if err != nil {
		return domain.FaceImageDecision{}, nil, err
	}
	decision := domain.DecideFaceImageMerge(spec.Thresholds, personID, person.Provisional, vector, provisional)
	now := time.Now()
	for _, candidate := range decision.Candidates {
		if err := offerFacePair(ctx, tx, candidate.PersonID, personID, candidate.Similarity, now); err != nil {
			return domain.FaceImageDecision{}, nil, err
		}
	}
	if decision.MergeID == 0 {
		return decision, nil, nil
	}
	events, err := mergeProvisionalOrRefuse(ctx, tx, c, decision.MergeID, personID, now)
	return decision, events, err
}

// provisionalFaceVectors reads the vectors of provisional people other than the person, leaving out those whose pair
// with the person the owner dismissed.
func provisionalFaceVectors(ctx context.Context, tx *sql.Tx, recognizer domain.FaceRecognizer, personID int64) (
	[]domain.FacePersonVector, error,
) {
	ids, err := queryInt64s(ctx, tx, `select p.id from people p where p.id <> ?1
		and not exists (select 1 from people_names n where n.person_id = p.id and n.canonical = 1)
		and not exists (select 1 from face_candidates c where c.dismissed = 1
			and (c.provisional_id = p.id and c.person_id = ?1 or c.provisional_id = ?1 and c.person_id = p.id))`,
		personID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the provisional people: %w", err)
	}
	open := make(map[int64]bool, len(ids))
	for _, id := range ids {
		open[id] = true
	}
	vectors, err := facePersonVectors(ctx, tx, recognizer)
	if err != nil {
		return nil, err
	}
	var out []domain.FacePersonVector
	for _, v := range vectors {
		if open[v.PersonID] {
			out = append(out, v)
		}
	}
	return out, nil
}

// offerFacePair offers the provisional person with the person, keeping one row per pair in either orientation; a
// pair the owner dismissed stays dismissed.
func offerFacePair(ctx context.Context, tx *sql.Tx, provisionalID, personID int64, similarity float64, now time.Time) error {
	res, err := tx.ExecContext(ctx, `update face_candidates set similarity = ?3
		where provisional_id = ?2 and person_id = ?1`, provisionalID, personID, similarity)
	if err != nil {
		return fmt.Errorf("cannot store a face candidate of person %d: %w", provisionalID, err)
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert into face_candidates (provisional_id, person_id, similarity, dismissed,
		created_at) values (?, ?, ?, 0, ?) on conflict (provisional_id, person_id) do update set
		similarity = excluded.similarity`, provisionalID, personID, similarity, now.UnixMilli()); err != nil {
		return fmt.Errorf("cannot store a face candidate of person %d: %w", provisionalID, err)
	}
	return nil
}
