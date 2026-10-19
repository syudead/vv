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

// A face is one JPEG row in person_images; people.image_revision changes with every change of the face.

// personImageRevision reads a person's revision, or 0 when the person has no face; idExpr names the person's id column.
func personImageRevision(idExpr string) string {
	return `coalesce((select p.image_revision from people p join person_images i on i.person_id = p.id
		 where p.id = ` + idExpr + `), 0)`
}

// PersonImage returns domain.ErrNotFound for a missing person and domain.ErrPersonImageNotFound for one without a face.
func (s *CreditStore) PersonImage(ctx context.Context, id int64) (domain.PersonImage, error) {
	var revision int64
	var image []byte
	var width, height sql.NullInt64
	err := s.read.QueryRowContext(ctx, `select p.image_revision, i.image, i.width, i.height
		 from people p left join person_images i on i.person_id = p.id where p.id = ?`, id).
		Scan(&revision, &image, &width, &height)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.PersonImage{}, domain.ErrNotFound
	case err != nil:
		return domain.PersonImage{}, fmt.Errorf("cannot read the image of person %d: %w", id, err)
	case image == nil:
		return domain.PersonImage{}, domain.ErrPersonImageNotFound
	}
	return domain.PersonImage{JPEG: image, Width: int(width.Int64), Height: int(height.Int64), Revision: revision}, nil
}

// SetPersonImage stores a fitted face, replacing any other, and returns the person with its new revision.
func (s *CreditStore) SetPersonImage(ctx context.Context, id int64, image domain.PersonImage) (domain.Credit, error) {
	return s.changePersonImage(ctx, id, func(tx *sql.Tx) (bool, error) {
		_, err := tx.ExecContext(ctx, `insert into person_images (person_id, image, width, height, updated_at)
			 values (?, ?, ?, ?, ?) on conflict (person_id) do update set
			 image = excluded.image, width = excluded.width, height = excluded.height, updated_at = excluded.updated_at`,
			id, image.JPEG, image.Width, image.Height, time.Now().Unix())
		return true, err
	})
}

// AddPersonImage stores a fitted face only when the person has none, so it never replaces one set since the caller
// looked; it returns the new revision, or 0 when the person kept a face.
func (s *CreditStore) AddPersonImage(ctx context.Context, id int64, image domain.PersonImage) (int64, error) {
	added := false
	person, err := s.changePersonImage(ctx, id, func(tx *sql.Tx) (bool, error) {
		res, err := tx.ExecContext(ctx, `insert into person_images (person_id, image, width, height, updated_at)
			 values (?, ?, ?, ?, ?) on conflict (person_id) do nothing`,
			id, image.JPEG, image.Width, image.Height, time.Now().Unix())
		if err != nil {
			return false, err
		}
		n, err := res.RowsAffected()
		added = n > 0
		return added, err
	})
	if err != nil || !added {
		return 0, err
	}
	return person.ImageRevision, nil
}

// DeletePersonImage removes the face; a person without one is returned unchanged.
func (s *CreditStore) DeletePersonImage(ctx context.Context, id int64) (domain.Credit, error) {
	return s.changePersonImage(ctx, id, func(tx *sql.Tx) (bool, error) {
		res, err := tx.ExecContext(ctx, `delete from person_images where person_id = ?`, id)
		if err != nil {
			return false, err
		}
		n, err := res.RowsAffected()
		return n > 0, err
	})
}

// changePersonImage runs change for an existing person and bumps the revision when it reports a change.
// Faces feed no smart rule or collection value, so the commit skips the match-change refresh.
func (s *CreditStore) changePersonImage(ctx context.Context, id int64, change func(*sql.Tx) (bool, error)) (domain.Credit, error) {
	t := creditTablesByKind[domain.CreditPerson]
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, err := existingCreditIDs(ctx, tx, t, []int64{id})
	if err != nil {
		return domain.Credit{}, err
	}
	if !existing[id] {
		return domain.Credit{}, domain.ErrNotFound
	}
	changed, err := change(tx)
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot change the image of person %d: %w", id, err)
	}
	if changed {
		if err := bumpPersonImageRevision(ctx, tx, id); err != nil {
			return domain.Credit{}, err
		}
	}
	credit, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	var events []domain.Event
	if changed {
		events = personFaceEvents(domain.Credit{}, credit)
	}
	if err := s.db.commitEvents(ctx, tx, events...); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot change the image of person %d: %w", id, err)
	}
	return credit, nil
}

// personFaceEvents reports a person whose face the write set, replaced or brought, or a provisional person with a
// face that it named, so the face worker embeds the face and applies the face-image rule.
func personFaceEvents(before, after domain.Credit) []domain.Event {
	if after.ImageRevision == 0 {
		return nil
	}
	if after.ImageRevision != before.ImageRevision || before.Provisional && !after.Provisional {
		return []domain.Event{domain.PersonImageChanged{PersonID: after.ID}}
	}
	return nil
}

func bumpPersonImageRevision(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `update people set image_revision = image_revision + 1 where id = ?`, id); err != nil {
		return fmt.Errorf("cannot change the image revision of person %d: %w", id, err)
	}
	return nil
}

// fillMergedPersonImage gives a target without a face the first face in sourceIDs order and returns that source, or
// 0 when the target kept its face or no source had one; it must run before the sources are deleted.
func fillMergedPersonImage(ctx context.Context, tx *sql.Tx, targetID int64, sourceIDs []int64) (int64, error) {
	withFace, err := peopleWithImages(ctx, tx, append([]int64{targetID}, sourceIDs...))
	if err != nil {
		return 0, err
	}
	if withFace[targetID] {
		return 0, nil
	}
	for _, sourceID := range sourceIDs {
		if !withFace[sourceID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `insert into person_images (person_id, image, width, height, updated_at)
			 select ?, image, width, height, ? from person_images where person_id = ?`,
			targetID, time.Now().Unix(), sourceID); err != nil {
			return 0, fmt.Errorf("cannot move the image to person %d: %w", targetID, err)
		}
		return sourceID, bumpPersonImageRevision(ctx, tx, targetID)
	}
	return 0, nil
}

func peopleWithImages(ctx context.Context, tx *sql.Tx, ids []int64) (map[int64]bool, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build person ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select person_id from person_images where person_id in (select value from json_each(?))`,
		string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the images of people: %w", err)
	}
	defer func() { _ = rows.Close() }()
	withFace := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the images of people: %w", err)
		}
		withFace[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the images of people: %w", err)
	}
	return withFace, nil
}
