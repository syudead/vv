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

// 保存は保存済みのキーに要求を当て、規則に合わなければ保存しない。確かめはアダプタに任せる。
func TestComfyUIConnectionSaveAndCheck(t *testing.T) {
	store := &fakeComfyUIStore{settings: domain.ComfyUISettings{Provider: domain.ComfyUICloud, APIKey: "k"}}
	probe := &fakeProbe{err: errors.New("cannot reach ComfyUI")}
	conn := NewComfyUIConnection(store, probe)
	ctx := context.Background()

	if _, err := conn.SaveSettings(ctx, domain.ComfyUISettingsUpdate{Provider: domain.ComfyUISelfHosted, URL: "nope"}); !errors.Is(err, domain.ErrComfyUIURLInvalid) {
		t.Fatalf("err = %v", err)
	}
	if store.saves != 0 {
		t.Fatalf("規則に合わない値を保存した: %+v", store.settings)
	}
	want := domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://h:8188", APIKey: "k"}
	saved, err := conn.SaveSettings(ctx, domain.ComfyUISettingsUpdate{Provider: domain.ComfyUISelfHosted, URL: "http://h:8188/"})
	if err != nil || saved != want || store.settings != want {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if got, err := conn.Settings(ctx); err != nil || got != want {
		t.Fatalf("Settings = %+v, %v", got, err)
	}
	if err := conn.Check(ctx); err == nil || probe.checks != 1 {
		t.Fatalf("Check = %v, checks = %d", err, probe.checks)
	}
}
