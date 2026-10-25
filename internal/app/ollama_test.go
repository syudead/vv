package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeOllamaStore struct {
	settings domain.OllamaSettings
	saved    bool
}

func (s *fakeOllamaStore) SavedOllamaSettings(context.Context) (domain.OllamaSettings, bool, error) {
	return s.settings, s.saved, nil
}

func (s *fakeOllamaStore) SaveOllamaSettings(_ context.Context, settings domain.OllamaSettings) error {
	s.settings = settings
	s.saved = true
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

func (p *fakeProbe) Models(context.Context) ([]string, error) {
	return nil, p.err
}

func TestAssistantReadinessFollowsSavedConnection(t *testing.T) {
	store := &fakeOllamaStore{settings: domain.DefaultOllamaSettings()}
	conn := NewOllamaConnection(store, &fakeProbe{})
	ctx := context.Background()
	readiness := func() domain.AssistantNotReady {
		t.Helper()
		got, err := conn.AssistantReadiness(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := readiness(); got != domain.AssistantOllamaNotSaved {
		t.Fatalf("未保存 = %q", got)
	}
	if _, err := conn.SaveSettings(ctx, domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef", GenerationModel: "qwen3:8b"}); err != nil {
		t.Fatal(err)
	}
	if got := readiness(); got != domain.AssistantReady {
		t.Fatalf("生成用のモデルあり = %q", got)
	}
	if _, err := conn.SaveSettings(ctx, domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef", GenerationModel: ""}); err != nil {
		t.Fatal(err)
	}
	if _, saved, _ := conn.Settings(ctx); !saved {
		t.Fatal("空の生成用のモデルで接続が未保存に戻った")
	}
	if got := readiness(); got != domain.AssistantGenerationModelUnset {
		t.Fatalf("空の生成用のモデル = %q", got)
	}
}

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
	if got, saved, err := conn.Settings(ctx); err != nil || saved || got != domain.DefaultOllamaSettings() {
		t.Fatalf("保存の前の Settings = %+v, %v, %v", got, saved, err)
	}
	want := domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef"}
	saved, err := conn.SaveSettings(ctx, domain.OllamaSettings{Endpoint: " http://h:1/ ", Model: "clef"})
	if err != nil || saved != want || store.settings != want {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if got, saved, err := conn.Settings(ctx); err != nil || got != want || !saved {
		t.Fatalf("保存のあとの Settings = %+v, %v, %v", got, saved, err)
	}
	if err := conn.Check(ctx); err == nil || probe.checks != 1 {
		t.Fatalf("Check = %v, checks = %d", err, probe.checks)
	}
}

func TestOllamaConnectionOnSaved(t *testing.T) {
	conn := NewOllamaConnection(&fakeOllamaStore{}, &fakeProbe{})
	calls := 0
	conn.OnSaved(func(context.Context) { calls++ })
	ctx := context.Background()
	if _, err := conn.SaveSettings(ctx, domain.OllamaSettings{Endpoint: "nope", Model: "clef"}); err == nil {
		t.Fatal("規則に合わない値を保存した")
	}
	if calls != 0 {
		t.Fatalf("保存しないのに呼んだ: %d", calls)
	}
	if _, err := conn.SaveSettings(ctx, domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("保存のあとの呼び出し = %d, want 1", calls)
	}
}
