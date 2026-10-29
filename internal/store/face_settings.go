package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// One of domain's recognizer ids; absent means domain.DefaultFaceRecognizer.
const faceRecognizerKey = "face.recognizer"

// FaceSettings reads face.enabled and face.recognizer; an unknown stored recognizer reads as the default.
func (s *SettingsStore) FaceSettings(ctx context.Context) (domain.FaceSettings, error) {
	settings := domain.DefaultFaceSettings()
	var err error
	if settings.Enabled, err = readBoolSetting(ctx, s.db.sql, faceEnabledKey); err != nil {
		return domain.FaceSettings{}, err
	}
	if settings.Recognizer, err = faceRecognizer(ctx, s.db.sql); err != nil {
		return domain.FaceSettings{}, err
	}
	return settings, nil
}

// faceRecognizer reads face.recognizer; absent or unknown reads as the default.
func faceRecognizer(ctx context.Context, q rowQueryer) (domain.FaceRecognizer, error) {
	value, found, err := readSetting(ctx, q, faceRecognizerKey)
	if err != nil {
		return "", err
	}
	if recognizer := domain.FaceRecognizer(value); found && recognizer.Valid() {
		return recognizer, nil
	}
	return domain.DefaultFaceRecognizer, nil
}

// SaveFaceSettings expects settings validated by domain.ValidateFaceSettings. A changed recognizer, in the same
// transaction, increases face.generation so a result computed for the old recognizer is refused, clears the old
// recognizer's vectors, queues again every video that has a face analysis result and queues the embedding of every
// face.
func (s *SettingsStore) SaveFaceSettings(ctx context.Context, settings domain.FaceSettings) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot save the face recognition settings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := faceRecognizer(ctx, tx)
	if err != nil {
		return err
	}
	c := &changes{}
	if current != settings.Recognizer {
		if err := switchFaceRecognizer(ctx, tx, c, settings.Recognizer); err != nil {
			return err
		}
	}
	if err := writeSetting(ctx, tx, faceEnabledKey, strconv.FormatBool(settings.Enabled)); err != nil {
		return err
	}
	if err := writeSetting(ctx, tx, faceRecognizerKey, string(settings.Recognizer)); err != nil {
		return err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot save the face recognition settings: %w", err)
	}
	return nil
}

// switchFaceRecognizer fences results of the old recognizer, removes its vectors, which the new one's cannot be
// compared with, and queues the analysed videos and a queue_face_images job, which queues every face again. Appearances keep their tracks, so the next analysis carries
// them over with their people.
func switchFaceRecognizer(ctx context.Context, tx *sql.Tx, c *changes, recognizer domain.FaceRecognizer) error {
	generation, err := faceGeneration(ctx, tx)
	if err != nil {
		return err
	}
	if err := writeSetting(ctx, tx, faceGenerationKey, strconv.FormatInt(generation+1, 10)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from person_face_vectors where recognizer <> ?`, string(recognizer)); err != nil {
		return fmt.Errorf("cannot remove the face image vectors of the old recognizer: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `update face_appearances set centroid = null
		where recognizer <> ? and centroid is not null`, string(recognizer)); err != nil {
		return fmt.Errorf("cannot remove the face vectors of the old recognizer: %w", err)
	}
	_, err = queueContentTx(ctx, tx, c, faceQueue, `select v.content_key from videos v
		join face_results r on r.content_key = v.content_key
		where v.content_key <> '' and `+registeredVideoCondition("v"), nil, `'done', 'failed'`,
		domain.ActivityByOwner, nil)
	if err != nil {
		return err
	}
	queued, err := queueSubjectJob(ctx, tx, subjectJob{kind: domain.JobQueueFaceImages, priority: domain.JobPriorityImport})
	if queued {
		c.jobsQueued(domain.JobQueueFaceImages)
	}
	return err
}
