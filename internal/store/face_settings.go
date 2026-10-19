package store

import (
	"context"
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
	value, found, err := readSetting(ctx, s.db.sql, faceRecognizerKey)
	if err != nil {
		return domain.FaceSettings{}, err
	}
	if recognizer := domain.FaceRecognizer(value); found && recognizer.Valid() {
		settings.Recognizer = recognizer
	}
	return settings, nil
}

// SaveFaceSettings expects settings validated by domain.ValidateFaceSettings. A changed recognizer increases
// face.generation in the same transaction, so a result computed for the old recognizer is refused.
func (s *SettingsStore) SaveFaceSettings(ctx context.Context, settings domain.FaceSettings) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot save the face recognition settings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := readSetting(ctx, tx, faceRecognizerKey)
	if err != nil {
		return err
	}
	if !found {
		current = string(domain.DefaultFaceRecognizer)
	}
	if current != string(settings.Recognizer) {
		generation, err := faceGeneration(ctx, tx)
		if err != nil {
			return err
		}
		if err := writeSetting(ctx, tx, faceGenerationKey, strconv.FormatInt(generation+1, 10)); err != nil {
			return err
		}
	}
	if err := writeSetting(ctx, tx, faceEnabledKey, strconv.FormatBool(settings.Enabled)); err != nil {
		return err
	}
	if err := writeSetting(ctx, tx, faceRecognizerKey, string(settings.Recognizer)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot save the face recognition settings: %w", err)
	}
	return nil
}
