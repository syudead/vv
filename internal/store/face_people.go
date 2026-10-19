package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Writes to people and credits move or drop face rows in their own transactions through these helpers.

// deletePersonFaceData drops the person's appearances and clears their centroids and crops, so a later analysis
// neither links nor recreates the face; the face image, its vector and the candidates go by cascade with the person.
func deletePersonFaceData(ctx context.Context, tx *sql.Tx, personID int64) error {
	if _, err := tx.ExecContext(ctx, `update face_appearances set dropped = 1, centroid = null, crop = null
		where person_id = ?`, personID); err != nil {
		return fmt.Errorf("cannot delete the face data of person %d: %w", personID, err)
	}
	return nil
}

// mergePersonFaceData moves the sources' appearances and candidate pairs to the target and deletes the pairs that
// would join the target to itself; it must run before the sources are deleted. encodedSources is a JSON id array.
func mergePersonFaceData(ctx context.Context, tx *sql.Tx, targetID int64, encodedSources string) error {
	statements := []struct{ what, statement string }{
		{"move the face appearances", `update face_appearances set person_id = ?1
			where person_id in (select value from json_each(?2))`},
		// A pair the target already has keeps the target's row; the source's goes with the source.
		{"move the face candidates", `update or ignore face_candidates set provisional_id = ?1
			where provisional_id in (select value from json_each(?2))`},
		{"move the face candidates", `update or ignore face_candidates set person_id = ?1
			where person_id in (select value from json_each(?2))`},
		{"drop the face candidates", `delete from face_candidates where provisional_id = person_id`},
	}
	for _, s := range statements {
		if _, err := tx.ExecContext(ctx, s.statement, targetID, encodedSources); err != nil {
			return fmt.Errorf("cannot %s to person %d: %w", s.what, targetID, err)
		}
	}
	return nil
}

// moveFaceVector gives the target the vector of the source face it just took, when that vector is current, so the
// face is not embedded again. It must run after the face moved and before the source is deleted.
func moveFaceVector(ctx context.Context, tx *sql.Tx, targetID, sourceID int64) error {
	if _, err := tx.ExecContext(ctx, `insert into person_face_vectors (person_id, image_revision, recognizer, vector)
		select ?1, (select image_revision from people where id = ?1), v.recognizer, v.vector
		  from person_face_vectors v join people p on p.id = v.person_id
		 where v.person_id = ?2 and v.image_revision = p.image_revision
		on conflict (person_id) do update set image_revision = excluded.image_revision,
			recognizer = excluded.recognizer, vector = excluded.vector`, targetID, sourceID); err != nil {
		return fmt.Errorf("cannot move the face vector to person %d: %w", targetID, err)
	}
	return nil
}

// dropRemovedPerformerFaces drops the video's appearances of every performer a work-information write removes, so
// a face link the owner removed is never made again.
func dropRemovedPerformerFaces(ctx context.Context, tx *sql.Tx, userKey string, before, after domain.WorkInfo) error {
	kept := map[int64]bool{}
	for _, p := range after.People {
		if p.Role == domain.PersonPerformer {
			kept[p.Person.ID] = true
		}
	}
	for _, p := range before.People {
		if p.Role != domain.PersonPerformer || kept[p.Person.ID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `update face_appearances set dropped = 1
			where person_id = ?1 and dropped = 0 and (content_key = ?2 or content_key in (
				select m.content_key from video_bundles b join video_bundle_members m on m.bundle_id = b.id
				 where b.user_key = ?2))`, p.Person.ID, userKey); err != nil {
			return fmt.Errorf("cannot drop the face appearances of person %d: %w", p.Person.ID, err)
		}
	}
	return nil
}
