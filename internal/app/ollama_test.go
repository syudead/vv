package app

import (
	"context"
	"errors"
	"slices"
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
	caps   domain.ModelCapabilities
	// capsErr fails Capabilities; shows records each call as "endpoint model".
	capsErr error
	shows   []string
}

func (p *fakeProbe) Check(context.Context) error {
	p.checks++
	return p.err
}

func (p *fakeProbe) Models(context.Context) ([]domain.OllamaModel, error) {
	return nil, p.err
}

func (p *fakeProbe) Capabilities(_ context.Context, endpoint, model string) (domain.ModelCapabilities, error) {
	p.shows = append(p.shows, endpoint+" "+model)
	return p.caps, p.capsErr
}

func TestAssistantReadinessFollowsSavedConnection(t *testing.T) {
	store := &fakeOllamaStore{settings: domain.DefaultOllamaSettings()}
	conn := NewOllamaConnection(store, &fakeProbe{caps: domain.ModelCapabilities{Tools: true}})
	ctx := context.Background()
	readiness := func() domain.AssistantNotReady {
		t.Helper()
		got, err := conn.AssistantReadiness(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got.NotReady
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

func TestAssistantReadinessReadsCapabilitiesOncePerModel(t *testing.T) {
	ctx := context.Background()
	gemma := domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef", GenerationModel: "gemma3:27b"}
	store := &fakeOllamaStore{settings: gemma, saved: true}
	probe := &fakeProbe{caps: domain.ModelCapabilities{Vision: true}}
	conn := NewOllamaConnection(store, probe)

	for range 2 {
		got, err := conn.AssistantReadiness(ctx)
		want := domain.AssistantStatus{NotReady: domain.AssistantGenerationModelWithoutTools, Vision: true}
		if err != nil || got != want {
			t.Fatalf("readiness = %+v, %v; want %+v", got, err, want)
		}
	}
	if !slices.Equal(probe.shows, []string{"http://h:1 gemma3:27b"}) {
		t.Fatalf("shows = %v, want one read for the pair", probe.shows)
	}

	// Saving rereads the pair, so a model updated in Ollama is seen without a restart.
	probe.caps = domain.ModelCapabilities{Tools: true, Vision: true}
	if _, err := conn.SaveSettings(ctx, gemma); err != nil {
		t.Fatal(err)
	}
	if got, err := conn.AssistantReadiness(ctx); err != nil || got != (domain.AssistantStatus{Vision: true}) {
		t.Fatalf("after save = %+v, %v", got, err)
	}
	if len(probe.shows) != 2 {
		t.Errorf("shows = %v, want the save's read only", probe.shows)
	}
}

func TestAssistantReadinessWhenOllamaCannotAnswer(t *testing.T) {
	ctx := context.Background()
	store := &fakeOllamaStore{
		settings: domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef", GenerationModel: "qwen3:8b"}, saved: true,
	}
	probe := &fakeProbe{capsErr: domain.ErrOllamaUnreachable}
	conn := NewOllamaConnection(store, probe)
	if got, err := conn.AssistantReadiness(ctx); err != nil || got != (domain.AssistantStatus{}) {
		t.Fatalf("unreachable = %+v, %v; want ready, the turn reports the failure", got, err)
	}
	// Nothing is kept, so the next read asks again once Ollama is up.
	probe.capsErr = nil
	if got, _ := conn.AssistantReadiness(ctx); got.NotReady != domain.AssistantGenerationModelWithoutTools || len(probe.shows) != 2 {
		t.Fatalf("second read = %+v, shows = %v", got, probe.shows)
	}
}

func TestOllamaConnectionCheckNamesModelWithoutTools(t *testing.T) {
	ctx := context.Background()
	store := &fakeOllamaStore{settings: domain.OllamaSettings{Endpoint: "http://h:1", Model: "clef", GenerationModel: "qwen2:0.5b"}}
	probe := &fakeProbe{}
	conn := NewOllamaConnection(store, probe)
	if err := conn.Check(ctx); !errors.Is(err, domain.ErrGenerationModelWithoutTools) {
		t.Fatalf("Check = %v", err)
	}
	probe.caps = domain.ModelCapabilities{Tools: true}
	if err := conn.Check(ctx); err != nil {
		t.Fatalf("with tools: Check = %v", err)
	}
	store.settings.GenerationModel = ""
	if err := conn.Check(ctx); err != nil || len(probe.shows) != 2 {
		t.Fatalf("no generation model: Check = %v, shows = %v", err, probe.shows)
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
