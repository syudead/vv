package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeComfyUIStore struct {
	settings domain.ComfyUISettings
	saves    int
}

func (s *fakeComfyUIStore) ComfyUISettings(context.Context) (domain.ComfyUISettings, error) {
	return s.settings, nil
}

func (s *fakeComfyUIStore) SaveComfyUISettings(_ context.Context, settings domain.ComfyUISettings) error {
	s.settings = settings
	s.saves++
	return nil
}

func TestComfyUIConnectionSaveAndCheck(t *testing.T) {
	store := &fakeComfyUIStore{settings: domain.ComfyUISettings{Provider: domain.ComfyUICloud, APIKey: "k"}}
	probe := &fakeProbe{err: errors.New("cannot reach ComfyUI")}
	conn := NewComfyUIConnection(store, probe, []domain.ComfyUIModel{"flux2_klein_4b", "qwen_image_edit_2511"})
	ctx := context.Background()

	if _, err := conn.SaveSettings(ctx, domain.ComfyUISettingsUpdate{Provider: domain.ComfyUISelfHosted, URL: "nope"}); !errors.Is(err, domain.ErrComfyUIURLInvalid) {
		t.Fatalf("err = %v", err)
	}
	if store.saves != 0 {
		t.Fatalf("規則に合わない値を保存した: %+v", store.settings)
	}
	// The store keeps the model absent, and the connection names the adapter's default.
	stored := domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://h:8188", APIKey: "k"}
	want := stored
	want.Model = "flux2_klein_4b"
	saved, err := conn.SaveSettings(ctx, domain.ComfyUISettingsUpdate{Provider: domain.ComfyUISelfHosted, URL: "http://h:8188/"})
	if err != nil || saved != want || store.settings != stored {
		t.Fatalf("saved = %+v, %v; stored = %+v", saved, err, store.settings)
	}
	if got, err := conn.Settings(ctx); err != nil || got != want {
		t.Fatalf("Settings = %+v, %v", got, err)
	}
	if err := conn.Check(ctx); err == nil || probe.checks != 1 {
		t.Fatalf("Check = %v, checks = %d", err, probe.checks)
	}
}
