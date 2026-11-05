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

// An image is one JPEG row in person_images or organization_images; the entity's image_revision changes with every
// change of the image.

// personImageRevision reads a person's revision, or 0 when the person has no face; idExpr names the person's id column.
func personImageRevision(idExpr string) string {
	return creditTablesByKind[domain.CreditPerson].imageRevision(idExpr)
}

// imageRevision reads an entity's revision, or 0 when it has no image; idExpr names the entity's id column.
func (t creditTables) imageRevision(idExpr string) string {
	return t.q(`coalesce((select ir.image_revision from {entity} ir join {images} ii on ii.{id} = ir.id
		 where ir.id = ` + idExpr + `), 0)`)
}

// PersonImage returns domain.ErrNotFound for a missing person and domain.ErrPersonImageNotFound for one without a face.
func (s *CreditStore) PersonImage(ctx context.Context, id int64) (domain.PersonImage, error) {
	return s.CreditImage(ctx, domain.CreditPerson, id)
}

// CreditImage returns domain.ErrNotFound for a missing entity and domain.ErrPersonImageNotFound for one without an
// image.
func (s *CreditStore) CreditImage(ctx context.Context, kind domain.CreditKind, id int64) (domain.PersonImage, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.PersonImage{}, err
	}
	var revision int64
	var image []byte
	var width, height sql.NullInt64
	err = s.read.QueryRowContext(ctx, t.q(`select e.image_revision, i.image, i.width, i.height
		 from {entity} e left join {images} i on i.{id} = e.id where e.id = ?`), id).
		Scan(&revision, &image, &width, &height)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.PersonImage{}, domain.ErrNotFound
	case err != nil:
		return domain.PersonImage{}, fmt.Errorf("cannot read the image of %s %d: %w", kind, id, err)
	case image == nil:
		return domain.PersonImage{}, domain.ErrPersonImageNotFound
	}
	return domain.PersonImage{JPEG: image, Width: int(width.Int64), Height: int(height.Int64), Revision: revision}, nil
}

// SetPersonImage stores a fitted face, replacing any other, and returns the person with its new revision.
func (s *CreditStore) SetPersonImage(ctx context.Context, id int64, image domain.PersonImage) (domain.Credit, error) {
	return s.SetCreditImage(ctx, domain.CreditPerson, id, image)
}

// SetCreditImage stores a fitted image, replacing any other, and returns the entity with its new revision.
func (s *CreditStore) SetCreditImage(ctx context.Context, kind domain.CreditKind, id int64, image domain.PersonImage) (domain.Credit, error) {
	return s.changeCreditImage(ctx, kind, id, func(tx *sql.Tx, t creditTables) (bool, error) {
		_, err := tx.ExecContext(ctx, t.q(`insert into {images} ({id}, image, width, height, updated_at)
			 values (?, ?, ?, ?, ?) on conflict ({id}) do update set
			 image = excluded.image, width = excluded.width, height = excluded.height, updated_at = excluded.updated_at`),
			id, image.JPEG, image.Width, image.Height, time.Now().Unix())
		return true, err
	})
}

// AddPersonImage stores a fitted face only when the person has none, so it never replaces one set since the caller
// looked; it returns the new revision, or 0 when the person kept a face.
func (s *CreditStore) AddPersonImage(ctx context.Context, id int64, image domain.PersonImage) (int64, error) {
	added := false
	person, err := s.changeCreditImage(ctx, domain.CreditPerson, id, func(tx *sql.Tx, _ creditTables) (bool, error) {
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
	return s.DeleteCreditImage(ctx, domain.CreditPerson, id)
}

// DeleteCreditImage removes the image; an entity without one is returned unchanged.
func (s *CreditStore) DeleteCreditImage(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error) {
	return s.changeCreditImage(ctx, kind, id, func(tx *sql.Tx, t creditTables) (bool, error) {
		res, err := tx.ExecContext(ctx, t.q(`delete from {images} where {id} = ?`), id)
		if err != nil {
			return false, err
		}
		n, err := res.RowsAffected()
		return n > 0, err
	})
}

// changeCreditImage runs change for an existing entity and bumps the revision when it reports a change.
// Images feed no smart rule or collection value, so the commit skips the match-change refresh.
func (s *CreditStore) changeCreditImage(ctx context.Context, kind domain.CreditKind, id int64,
	change func(*sql.Tx, creditTables) (bool, error),
) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
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
	changed, err := change(tx, t)
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot change the image of %s %d: %w", kind, id, err)
	}
	if changed {
		if err := bumpImageRevision(ctx, tx, t, id); err != nil {
			return domain.Credit{}, err
		}
	}
	credit, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	var events []domain.Event
	if changed && kind == domain.CreditPerson {
		if events, err = queuePersonFace(ctx, tx, domain.Credit{}, credit); err != nil {
			return domain.Credit{}, err
		}
	}
	if err := s.db.commitEvents(ctx, tx, events...); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot change the image of %s %d: %w", kind, id, err)
	}
	return credit, nil
}

// queuePersonFace queues a face_image job, in the write's transaction, for a person whose face the write set, replaced
// or brought, or a provisional person with a face that it named, so the face is embedded and the face-image rule
// applied. It returns the events to publish after commit.
func queuePersonFace(ctx context.Context, tx *sql.Tx, before, after domain.Credit) ([]domain.Event, error) {
	if after.ImageRevision == 0 {
		return nil, nil
	}
	if after.ImageRevision != before.ImageRevision || before.Provisional && !after.Provisional {
		return queueFaceImageJob(ctx, tx, after.ID)
	}
	return nil, nil
}

func bumpPersonImageRevision(ctx context.Context, tx *sql.Tx, id int64) error {
	return bumpImageRevision(ctx, tx, creditTablesByKind[domain.CreditPerson], id)
}

func bumpImageRevision(ctx context.Context, tx *sql.Tx, t creditTables, id int64) error {
	if _, err := tx.ExecContext(ctx, t.q(`update {entity} set image_revision = image_revision + 1 where id = ?`), id); err != nil {
		return fmt.Errorf("cannot change the image revision of %s %d: %w", t.kind, id, err)
	}
	return nil
}

// fillMergedImage gives a target without an image the first image in sourceIDs order and returns that source, or
// 0 when the target kept its image or no source had one; it must run before the sources are deleted.
func fillMergedImage(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64, sourceIDs []int64) (int64, error) {
	withImage, err := creditsWithImages(ctx, tx, t, append([]int64{targetID}, sourceIDs...))
	if err != nil {
		return 0, err
	}
	if withImage[targetID] {
		return 0, nil
	}
	for _, sourceID := range sourceIDs {
		if !withImage[sourceID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, t.q(`insert into {images} ({id}, image, width, height, updated_at)
			 select ?, image, width, height, ? from {images} where {id} = ?`),
			targetID, time.Now().Unix(), sourceID); err != nil {
			return 0, fmt.Errorf("cannot move the image to %s %d: %w", t.kind, targetID, err)
		}
		return sourceID, bumpImageRevision(ctx, tx, t, targetID)
	}
	return 0, nil
}

func creditsWithImages(ctx context.Context, tx *sql.Tx, t creditTables, ids []int64) (map[int64]bool, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	rows, err := tx.QueryContext(ctx, t.q(`select {id} from {images} where {id} in (select value from json_each(?))`),
		string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the images of %s entities: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	withImage := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the images of %s entities: %w", t.kind, err)
		}
		withImage[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the images of %s entities: %w", t.kind, err)
	}
	return withImage, nil
}
