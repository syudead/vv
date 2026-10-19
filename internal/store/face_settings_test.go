package store

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A switch of recognizer fences results of the old one; saving the same recognizer again must not.
func TestSaveFaceSettingsIncreasesTheGenerationOnlyOnARecognizerSwitch(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	saves := []struct {
		settings   domain.FaceSettings
		generation int64
	}{
		{domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8}, 0},
		{domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50}, 1},
		{domain.FaceSettings{Enabled: false, Recognizer: domain.FaceRecognizerArcFaceR50}, 1},
	}
	for _, save := range saves {
		if err := db.Settings().SaveFaceSettings(ctx, save.settings); err != nil {
			t.Fatal(err)
		}
		got, err := db.Settings().FaceSettings(ctx)
		if err != nil || got != save.settings {
			t.Fatalf("FaceSettings = %+v, %v; want %+v", got, err, save.settings)
		}
		if generation, err := faceGeneration(ctx, db.sql); err != nil || generation != save.generation {
			t.Fatalf("after saving %+v: generation = %d, %v; want %d", save.settings, generation, err, save.generation)
		}
	}
}
