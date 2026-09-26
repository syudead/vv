// previewbench は動くプレビューの生成にかかる壁時計時間と ffmpeg のピークメモリを測る。
// 生成方式を変える前後で、同じ入力と環境の数字を比べるために使う。手順は
// docs/how-to/preview-benchmark.md にある。
//
// 測るのは本番の生成コード media.GeneratePreview そのものである。ffmpeg の引数を
// ここへ写すと本番とずれるので写さない。本番コードに計測のための口も足さない
// （specs/019-preview-input-seek/research.md R-4）。
//
// ピークメモリは終了した子プロセス全体の最大（RUSAGE_CHILDREN の maxrss）なので、
// 複数の入力を1回の実行に渡すと前の入力の値を引き継ぐ。比べるときは入力ごとに
// 実行を分ける。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/syudead/vv/internal/media"
)

const defaultRuns = 2

// config はコマンドラインの解釈結果である。
type config struct {
	runs   int
	videos []string
}

// errUsage は使い方の誤りを表す。終了コードを分けるために使う。
var errUsage = errors.New("使い方の誤り")

func parseArgs(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("previewbench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "使い方: go run ./scripts/previewbench [-runs N] <video>...")
		flags.PrintDefaults()
	}
	runs := flags.Int("runs", defaultRuns, "入力ごとに生成を走らせる回数")
	if err := flags.Parse(args); err != nil {
		return config{}, fmt.Errorf("%w: %w", errUsage, err)
	}
	if *runs < 1 {
		return config{}, fmt.Errorf("%w: -runs は 1 以上にしてください（%d）", errUsage, *runs)
	}
	if flags.NArg() == 0 {
		return config{}, fmt.Errorf("%w: 測る動画を1つ以上渡してください", errUsage)
	}
	return config{runs: *runs, videos: flags.Args()}, nil
}

// deps は外部プロセスに触れる処理である。テストから差し替える。
type deps struct {
	probeDuration func(ctx context.Context, path string) (int64, error)
	generate      func(ctx context.Context, videoPath, output string, durationMs int64) error
	// peakRSS は終了した子プロセス全体のピークメモリ（バイト）を返す。
	// 取れない OS では ok が false になる。
	peakRSS func() (bytes int64, ok bool)
}

func productionDeps() deps {
	return deps{
		probeDuration: func(ctx context.Context, path string) (int64, error) {
			probe, err := media.Probe(ctx, path)
			if err != nil {
				return 0, err
			}
			return probe.DurationMs, nil
		},
		generate: media.GeneratePreview,
		peakRSS:  childrenPeakRSS,
	}
}

// result は1つの入力の計測結果である。
type result struct {
	video      string
	durationMs int64
	runs       []time.Duration
	peakBytes  int64
	peakOK     bool
}

func measure(ctx context.Context, cfg config, d deps, workDir string, progress func(result)) ([]result, error) {
	results := make([]result, 0, len(cfg.videos))
	for _, video := range cfg.videos {
		info, err := os.Stat(video)
		if err != nil {
			return results, fmt.Errorf("入力を開けません (%s): %w", video, err)
		}
		if info.IsDir() {
			return results, fmt.Errorf("入力がファイルではありません (%s)", video)
		}
		durationMs, err := d.probeDuration(ctx, video)
		if err != nil {
			return results, fmt.Errorf("入力の長さを取れません (%s): %w", video, err)
		}
		if durationMs <= 0 {
			return results, fmt.Errorf("入力の長さが分かりません (%s)", video)
		}
		r := result{video: video, durationMs: durationMs}
		output := filepath.Join(workDir, "preview.mp4")
		for range cfg.runs {
			started := time.Now()
			if err := d.generate(ctx, video, output, durationMs); err != nil {
				return results, fmt.Errorf("プレビューを生成できません (%s): %w", video, err)
			}
			r.runs = append(r.runs, time.Since(started))
			if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
				return results, fmt.Errorf("生成した出力を消せません (%s): %w", output, err)
			}
		}
		r.peakBytes, r.peakOK = d.peakRSS()
		results = append(results, r)
		if progress != nil {
			progress(r)
		}
	}
	return results, nil
}

func formatResult(r result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s（長さ %.3f 秒）\n", r.video, float64(r.durationMs)/1000)
	for i, elapsed := range r.runs {
		fmt.Fprintf(&b, "  %d 回目: %.3f 秒\n", i+1, elapsed.Seconds())
	}
	if r.peakOK {
		fmt.Fprintf(&b, "  ピークメモリ（全回の最大）: %.1f MiB\n", float64(r.peakBytes)/(1024*1024))
	} else {
		b.WriteString("  ピークメモリ: この OS では測りません\n")
	}
	return b.String()
}

func run(ctx context.Context, args []string, d deps, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	workDir, err := os.MkdirTemp("", "previewbench-")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "一時ディレクトリを作れません: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	_, err = measure(ctx, cfg, d, workDir, func(r result) { _, _ = fmt.Fprint(stdout, formatResult(r)) })
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], productionDeps(), os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
