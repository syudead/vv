package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/store"
)

func TestParseArgs(t *testing.T) {
	cfg, err := parseArgs([]string{"-scale", "1000", "-skip-build", "-chromium", "/opt/chrome"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := config{scale: 1000, videos: 10000, skipBuild: true, chromium: "/opt/chrome"}
	if cfg != want {
		t.Fatalf("parseArgs = %+v, want %+v", cfg, want)
	}
	if name := cfg.dataName(); name != "1000" {
		t.Errorf("dataName = %q, want 1000", name)
	}
	cfg, err = parseArgs([]string{"-scale", "30000", "-videos", "30000"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if want := (config{scale: 30000, videos: 30000}); cfg != want {
		t.Fatalf("parseArgs = %+v, want %+v", cfg, want)
	}
	if name := cfg.dataName(); name != "30000-videos-30000" {
		t.Errorf("dataName = %q, want 30000-videos-30000", name)
	}
	for _, args := range [][]string{{}, {"-scale", "0"}, {"-scale", "10", "extra"}, {"-scale", "10", "-videos", "-1"}, {"-scale", "10", "-videos", "0"}} {
		if _, err := parseArgs(args, io.Discard); !errors.Is(err, errUsage) {
			t.Errorf("parseArgs(%q) = %v, want errUsage", args, err)
		}
	}
}

func TestPlanTagsMatchesTheParentIssueShape(t *testing.T) {
	for _, shape := range []struct{ scale, videos int }{{1000, 10000}, {3000, 30000}, {30000, 30000}} {
		scale := shape.scale
		plans := planTags(scale, shape.videos)
		if len(plans) != scale {
			t.Fatalf("scale %d: %d tags", scale, len(plans))
		}
		names := make(map[string]bool, scale)
		unused, tentative := 0, 0
		for _, plan := range plans {
			if names[plan.name] {
				t.Fatalf("scale %d: duplicate name %q", scale, plan.name)
			}
			names[plan.name] = true
			if len(plan.videos) == 0 {
				unused++
			}
			if plan.tentative {
				tentative++
			}
			seen := make(map[int]bool, len(plan.videos))
			for _, v := range plan.videos {
				if v < 0 || v >= shape.videos || seen[v] {
					t.Fatalf("scale %d: tag %q has a bad video %d", scale, plan.name, v)
				}
				seen[v] = true
			}
		}
		// 親 Issue の「1,000 個のうち約 90 個」が本数 0、半数が仮。
		if unused < scale*8/100 || unused > scale*10/100 {
			t.Errorf("scale %d: %d unused tags, want about 9%%", scale, unused)
		}
		if tentative != scale/2 {
			t.Errorf("scale %d: %d tentative tags, want %d", scale, tentative, scale/2)
		}
	}
	// 動画が 10 本未満でも、本数 0 は unusedEvery 個に 1 個だけで、本数の多いタグは空にならない。
	for videos := 1; videos < 10; videos++ {
		for i, plan := range planTags(100, videos) {
			if wantUnused := i%unusedEvery == unusedEvery-1; (len(plan.videos) == 0) != wantUnused {
				t.Errorf("videos %d: tag %d has %d videos, want unused=%v", videos, i, len(plan.videos), wantUnused)
			}
		}
	}
	if !reflect.DeepEqual(planTags(100, 1000), planTags(100, 1000)) {
		t.Error("planTags is not deterministic")
	}
}

func TestSeedWritesTheTagsThroughTheStore(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	mediaDir := filepath.Join(t.TempDir(), "media")
	const scale = 33
	const videos = 50
	summary, err := seed(ctx, dataDir, mediaDir, scale, videos, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := seedSummary{tags: scale, unused: 3, tentative: 16, videos: videos}
	if summary != want {
		t.Fatalf("seed = %+v, want %+v", summary, want)
	}

	db, err := store.OpenContext(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tagsPage, err := db.Tags().ListTags(ctx, domain.TagListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	tags := tagsPage.Items
	counts := make(map[string]int, len(tags))
	for _, tag := range tags {
		counts[tag.Name] = tag.VideoCount
	}
	for _, plan := range planTags(scale, videos) {
		if got := counts[plan.name]; got != len(plan.videos) {
			t.Errorf("tag %q has %d videos, want %d", plan.name, got, len(plan.videos))
		}
	}
}

func TestCopyDirReplacesTheTarget(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "run")
	writeFile(t, filepath.Join(src, "a", "b.txt"), "seed")
	writeFile(t, filepath.Join(dst, "stale.txt"), "old")
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dst, "a", "b.txt")); got != "seed" {
		t.Errorf("copied = %q", got)
	}
	if exists(filepath.Join(dst, "stale.txt")) {
		t.Error("the stale file was kept")
	}
}

func TestRunRejectsBadUsage(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"-scale", "-1"}, io.Discard, &stderr); code != 2 {
		t.Fatalf("run = %d, want 2 (%s)", code, stderr.String())
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
