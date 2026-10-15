package main_test

import (
	"os"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/wasmplugin"
	"github.com/syudead/vv/internal/wasmplugin/plugintest"
)

// module はプラグインを wasip1 向けにビルドし、manifest.json を足したモジュールを返す。
func module(t *testing.T) []byte {
	t.Helper()
	manifest, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	return plugintest.WithManifest(plugintest.Build(t, "."), string(manifest))
}

func runtime(t *testing.T) *wasmplugin.Runtime {
	t.Helper()
	rt, err := wasmplugin.New(t.Context(), wasmplugin.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(t.Context()) })
	return rt
}

// TestInspect はビルドしたプラグインが VVMDM の検査を通ることを確かめる。
func TestInspect(t *testing.T) {
	manifest, problem, err := runtime(t).Inspect(t.Context(), module(t))
	if err != nil || problem != "" {
		t.Fatalf("Inspect: problem %q, error %v", problem, err)
	}
	if manifest.Name != "wikipedia" {
		t.Errorf("manifest name = %q", manifest.Name)
	}
}

// TestFetchRejectsOtherSites はネットワークに出ない経路で、プラグインが失敗を error の応答で返すことを確かめる。
func TestFetchRejectsOtherSites(t *testing.T) {
	_, err := runtime(t).Run(t.Context(), "test", module(t), domain.WorkInfoRunRequest{
		Operation: domain.WorkInfoFetch, URL: "https://example.com/works/1",
	})
	if err == nil || !strings.Contains(err.Error(), "not a Wikipedia article") {
		t.Fatalf("Run error = %v; want the plugin's message about a non-Wikipedia URL", err)
	}
}
