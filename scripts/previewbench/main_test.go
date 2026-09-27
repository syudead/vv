package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	cfg, err := parseArgs([]string{"a.mp4", "b.mp4"}, io.Discard)
	if err != nil {
		t.Fatalf("既定の引数を解釈できない: %v", err)
	}
	if cfg.runs != defaultRuns || len(cfg.videos) != 2 || cfg.videos[0] != "a.mp4" || cfg.videos[1] != "b.mp4" {
		t.Errorf("解釈結果が違う: %+v", cfg)
	}

	cfg, err = parseArgs([]string{"-runs", "5", "long.mp4"}, io.Discard)
	if err != nil || cfg.runs != 5 || len(cfg.videos) != 1 {
		t.Errorf("-runs を解釈できない: %+v, %v", cfg, err)
	}

	for _, args := range [][]string{
		{},
		{"-runs", "3"},
		{"-runs", "0", "a.mp4"},
		{"-runs", "x", "a.mp4"},
		{"-unknown", "a.mp4"},
	} {
		if _, err := parseArgs(args, io.Discard); !errors.Is(err, errUsage) {
			t.Errorf("%q を使い方の誤りとして扱わない: %v", args, err)
		}
	}
}

func TestFormatResult(t *testing.T) {
	r := result{
		video:      "long.mp4",
		durationMs: 7_200_000,
		runs:       []time.Duration{1500 * time.Millisecond, 2250 * time.Millisecond},
		peakBytes:  300 * 1024 * 1024,
		peakOK:     true,
	}
	out := formatResult(r)
	for _, want := range []string{"long.mp4（長さ 7200.000 秒）", "1 回目: 1.500 秒", "2 回目: 2.250 秒", "ピークメモリ（全回の最大）: 300.0 MiB"} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q が無い:\n%s", want, out)
		}
	}

	r.peakOK = false
	out = formatResult(r)
	if !strings.Contains(out, "この OS では測りません") || strings.Contains(out, "MiB") {
		t.Errorf("ピークメモリを測らない OS の表示が違う:\n%s", out)
	}
}

func writeVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.mp4")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeDeps(generateErr error, calls *int) deps {
	return deps{
		probeDuration: func(context.Context, string) (int64, error) { return 120_000, nil },
		generate: func(_ context.Context, _, output string, durationMs int64) error {
			*calls++
			if durationMs != 120_000 {
				return errors.New("長さが渡っていない")
			}
			if generateErr != nil {
				return generateErr
			}
			return os.WriteFile(output, []byte("mp4"), 0o600)
		},
		peakRSS: func() (int64, bool) { return 64 * 1024 * 1024, true },
	}
}

func TestRunPrintsEachRun(t *testing.T) {
	video := writeVideo(t)
	calls := 0
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-runs", "3", video}, fakeDeps(nil, &calls), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("終了コード %d: %s", code, stderr.String())
	}
	if calls != 3 {
		t.Errorf("生成を %d 回走らせた（3 回のはず）", calls)
	}
	for _, want := range []string{video, "1 回目", "3 回目", "64.0 MiB"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("出力に %q が無い:\n%s", want, stdout.String())
		}
	}
}

func TestRunFailsWithReason(t *testing.T) {
	calls := 0
	var stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing.mp4")
	if code := run(context.Background(), []string{missing}, fakeDeps(nil, &calls), io.Discard, &stderr); code == 0 {
		t.Error("存在しない入力で終了コード 0 を返した")
	}
	if !strings.Contains(stderr.String(), missing) || calls != 0 {
		t.Errorf("存在しない入力の理由が出ていない（生成 %d 回）: %s", calls, stderr.String())
	}

	stderr.Reset()
	video := writeVideo(t)
	if code := run(context.Background(), []string{video}, fakeDeps(errors.New("ffmpeg が落ちた"), &calls), io.Discard, &stderr); code == 0 {
		t.Error("ffmpeg の失敗で終了コード 0 を返した")
	}
	if !strings.Contains(stderr.String(), "ffmpeg が落ちた") {
		t.Errorf("ffmpeg の失敗の理由が出ていない: %s", stderr.String())
	}

	stderr.Reset()
	if code := run(context.Background(), nil, fakeDeps(nil, &calls), io.Discard, &stderr); code != 2 {
		t.Errorf("引数が無いときの終了コードが %d（2 のはず）", code)
	}
}
