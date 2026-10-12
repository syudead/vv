package store

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 保存前はゼロ値を返し、保存した値を読み戻せる。キーを消した保存は空で読める。
func TestComfyUISettingsRoundTrip(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	got, err := db.Settings().ComfyUISettings(ctx)
	if err != nil || got != (domain.ComfyUISettings{}) || got.Saved() {
		t.Fatalf("保存前 = %+v, %v", got, err)
	}
	want := domain.ComfyUISettings{Provider: domain.ComfyUICloud, URL: "http://gpu:8188", APIKey: "secret"}
	if err := db.Settings().SaveComfyUISettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, err = db.Settings().ComfyUISettings(ctx); err != nil || got != want {
		t.Fatalf("読み戻し = %+v, %v", got, err)
	}
	want = domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://gpu:8188"}
	if err := db.Settings().SaveComfyUISettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, err = db.Settings().ComfyUISettings(ctx); err != nil || got != want {
		t.Fatalf("キーを消したあと = %+v, %v", got, err)
	}
	if ollama, err := db.Settings().OllamaSettings(ctx); err != nil || ollama != domain.DefaultOllamaSettings() {
		t.Fatalf("ComfyUI の接続先が Ollama の接続先を変えた: %+v, %v", ollama, err)
	}
}
