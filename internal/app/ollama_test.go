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

// 保存に成功したときだけ、登録した関数を呼ぶ（重複の判定の再開、docs/design-docs/duplicate-judging.md）。
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
