package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	if cfg.kind != kindPreview || cfg.runs != defaultRuns || len(cfg.videos) != 2 || cfg.videos[0] != "a.mp4" || cfg.videos[1] != "b.mp4" {
		t.Errorf("解釈結果が違う: %+v", cfg)
	}

	cfg, err = parseArgs([]string{"-runs", "5", "long.mp4"}, io.Discard)
	if err != nil || cfg.runs != 5 || len(cfg.videos) != 1 {
		t.Errorf("-runs を解釈できない: %+v, %v", cfg, err)
	}

	cfg, err = parseArgs([]string{"-kind", "seek", "long.mp4"}, io.Discard)
	if err != nil || cfg.kind != kindSeek || cfg.runs != defaultRuns || len(cfg.videos) != 1 {
		t.Errorf("-kind seek を解釈できない: %+v, %v", cfg, err)
	}
	cfg, err = parseArgs([]string{"-kind", "preview", "long.mp4"}, io.Discard)
	if err != nil || cfg.kind != kindPreview {
		t.Errorf("-kind preview を解釈できない: %+v, %v", cfg, err)
	}

	for _, args := range [][]string{
		{},
		{"-kind", "thumbnail", "a.mp4"},
		{"-kind", "", "a.mp4"},
		{"-kind", "seek"},
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
	for _, want := range []string{"long.mp4（長さ 7200.000 秒、動くプレビュー）", "1 回目: 1.500 秒", "2 回目: 2.250 秒", "ピークメモリ（全回の最大）: 300.0 MiB"} {
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

func TestFormatResultSeek(t *testing.T) {
	r := result{
		kind:       kindSeek,
		video:      "long.mp4",
		durationMs: 7_200_000,
		runs:       []time.Duration{1500 * time.Millisecond, 2250 * time.Millisecond},
		peakBytes:  300 * 1024 * 1024,
		peakOK:     true,
		outputs:    []outputStats{{files: 1440, bytes: 3 * 1024 * 1024}, {files: 1440, bytes: 3 * 1024 * 1024}},
	}
	out := formatResult(r)
	for _, want := range []string{
		"long.mp4（長さ 7200.000 秒、シーク用サムネイル）",
		"1 回目: 1.500 秒（ファイル数 1440、合計 3.0 MiB = 3145728 バイト）",
		"2 回目: 2.250 秒（ファイル数 1440、合計 3.0 MiB = 3145728 バイト）",
		"ピークメモリ（全回の最大）: 300.0 MiB",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("出力に %q が無い:\n%s", want, out)
		}
	}
}

func TestCountOutputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "000000.jpg"), make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000001.jpg"), make([]byte, 23), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.json"), make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}
	stats, err := countOutputs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if stats.files != 3 || stats.bytes != 130 {
		t.Errorf("集計が違う: %+v（3 ファイル・130 バイトのはず）", stats)
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
		generateSeek: func(context.Context, string, string, int64) error {
			return errors.New("シーク用を呼んだ")
		},
		peakRSS: func() (int64, bool) { return 64 * 1024 * 1024, true },
	}
}

func TestRunSeekCountsOutputsAndRemovesThem(t *testing.T) {
	video := writeVideo(t)
	var dirs []string
	d := deps{
		probeDuration: func(context.Context, string) (int64, error) { return 120_000, nil },
		generate: func(context.Context, string, string, int64) error {
			return errors.New("動くプレビューを呼んだ")
		},
		generateSeek: func(_ context.Context, videoPath, outputDir string, durationMs int64) error {
			if videoPath != video || durationMs != 120_000 {
				return errors.New("入力か長さが渡っていない")
			}
			dirs = append(dirs, outputDir)
			for i, size := range []int{1000, 2000, 500} {
				name := filepath.Join(outputDir, fmt.Sprintf("%06d.jpg", i))
				if err := os.WriteFile(name, make([]byte, size), 0o600); err != nil {
					return err
				}
			}
			return nil
		},
		peakRSS: func() (int64, bool) { return 0, false },
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-kind", "seek", video}, d, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("終了コード %d: %s", code, stderr.String())
	}
	if len(dirs) != defaultRuns {
		t.Fatalf("シーク用の生成を %d 回走らせた（%d 回のはず）", len(dirs), defaultRuns)
	}
	for _, want := range []string{"シーク用サムネイル", "1 回目", "2 回目", "ファイル数 3", "3500 バイト", "この OS では測りません"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("出力に %q が無い:\n%s", want, stdout.String())
		}
	}
	// 一時出力は生成のたびに消え、終了後は作業ディレクトリごと残らない。
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Dir(dir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("一時出力が残っている (%s): %v", filepath.Dir(dir), err)
		}
	}
}

func TestRunSeekFailsWithReason(t *testing.T) {
	video := writeVideo(t)
	calls := 0
	d := fakeDeps(nil, &calls)
	var workDir string
	d.generateSeek = func(_ context.Context, _, outputDir string, _ int64) error {
		workDir = filepath.Dir(outputDir)
		return errors.New("ffmpeg が落ちた")
	}
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"-kind", "seek", video}, d, io.Discard, &stderr); code != 1 {
		t.Errorf("シーク用の生成の失敗で終了コード %d を返した（1 のはず）", code)
	}
	if !strings.Contains(stderr.String(), "ffmpeg が落ちた") || calls != 0 {
		t.Errorf("失敗の理由が出ていない（動くプレビュー %d 回）: %s", calls, stderr.String())
	}
	if _, err := os.Stat(workDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("失敗の後に一時出力が残っている (%s): %v", workDir, err)
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
