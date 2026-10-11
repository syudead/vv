package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeOllamaStore struct{ settings domain.OllamaSettings }

func (s *fakeOllamaStore) OllamaSettings(context.Context) (domain.OllamaSettings, error) {
	return s.settings, nil
}

func (s *fakeOllamaStore) SaveOllamaSettings(_ context.Context, settings domain.OllamaSettings) error {
	s.settings = settings
	return nil
}

type fakeProbe struct {
	err    error
	checks int
}

func (p *fakeProbe) Check(context.Context) error {
	p.checks++
	return p.err
}

// 接続先は整えて保存し、規則に合わなければ保存しない。確かめは判定モデルのアダプタに任せる。
func TestOllamaConnectionSaveAndCheck(t *testing.T) {
	store := &fakeOllamaStore{settings: domain.DefaultOllamaSettings()}
	probe := &fakeProbe{err: errors.New("cannot reach Ollama")}
	conn := NewOllamaConnection(store, probe)
	ctx := context.Background()

	if _, err := conn.SaveSettings(ctx, domain.OllamaSettings{Endpoint: "nope", Model: "clef"}); !errors.Is(err, domain.ErrInvalidOllamaSettings) {
		t.Fatalf("err = %v", err)
	}
	if store.settings != domain.DefaultOllamaSettings() {
		t.Fatalf("規則に合わない値を保存した: %+v", store.settings)
	}
	want := domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef"}
	saved, err := conn.SaveSettings(ctx, domain.OllamaSettings{Endpoint: " http://h:1/ ", Model: "clef"})
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
