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

	"github.com/syudead/vv/internal/store"
)

func TestParseArgs(t *testing.T) {
	cfg, err := parseArgs([]string{"-scale", "1000", "-skip-build", "-chromium", "/opt/chrome"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := config{scale: 1000, skipBuild: true, chromium: "/opt/chrome"}
	if cfg != want {
		t.Fatalf("parseArgs = %+v, want %+v", cfg, want)
	}
	for _, args := range [][]string{{}, {"-scale", "0"}, {"-scale", "10", "extra"}} {
		if _, err := parseArgs(args, io.Discard); !errors.Is(err, errUsage) {
			t.Errorf("parseArgs(%q) = %v, want errUsage", args, err)
		}
	}
}

func TestPlanTagsMatchesTheParentIssueShape(t *testing.T) {
	for _, scale := range []int{1000, 3000} {
		plans := planTags(scale)
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
				if v < 0 || v >= scale*videosPerTag || seen[v] {
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
	if !reflect.DeepEqual(planTags(100), planTags(100)) {
		t.Error("planTags is not deterministic")
	}
}

func TestSeedWritesTheTagsThroughTheStore(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	mediaDir := filepath.Join(t.TempDir(), "media")
	const scale = 33
	summary, err := seed(ctx, dataDir, mediaDir, scale, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := seedSummary{tags: scale, unused: 3, tentative: 16, videos: scale * videosPerTag}
	if summary != want {
		t.Fatalf("seed = %+v, want %+v", summary, want)
	}

	db, err := store.OpenContext(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tags, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int, len(tags))
	for _, tag := range tags {
		counts[tag.Name] = tag.VideoCount
	}
	for _, plan := range planTags(scale) {
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
