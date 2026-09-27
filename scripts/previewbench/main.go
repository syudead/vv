// previewbench は動くプレビューとシーク用サムネイルの生成にかかる壁時計時間と
// ffmpeg のピークメモリを測る。シーク用では出力のファイル数と合計バイト数も出す。
// 生成方式を変える前後で、同じ入力と環境の数字を比べるために使う。手順は
// docs/how-to/preview-benchmark.md にある。
//
// 測るのは本番の生成コードそのもの（動くプレビューは media.GeneratePreview、
// シーク用は media.GenerateSeekThumbnailSet）である。ffmpeg の引数をここへ写すと
// 本番とずれるので写さない（specs/019-preview-input-seek/research.md R-4）。
// シーク用の生成方式を変えるときは GenerateSeekThumbnailSet の中身だけを変え、
// ここには触れない（specs/021-seek-thumbnail-sprite/plan.md）。
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
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/syudead/vv/internal/media"
)

const defaultRuns = 2

// kind は測る生成の種類である。
type kind string

const (
	kindPreview kind = "preview" // 動くプレビュー
	kindSeek    kind = "seek"    // シーク用サムネイル
)

// config はコマンドラインの解釈結果である。
type config struct {
	kind   kind
	runs   int
	videos []string
}

// errUsage は使い方の誤りを表す。終了コードを分けるために使う。
var errUsage = errors.New("使い方の誤り")

func parseArgs(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("previewbench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "使い方: go run ./scripts/previewbench [-kind preview|seek] [-runs N] <video>...")
		flags.PrintDefaults()
	}
	kindName := flags.String("kind", string(kindPreview), "測る生成の種類（preview: 動くプレビュー、seek: シーク用サムネイル）")
	runs := flags.Int("runs", defaultRuns, "入力ごとに生成を走らせる回数")
	if err := flags.Parse(args); err != nil {
		return config{}, fmt.Errorf("%w: %w", errUsage, err)
	}
	k := kind(*kindName)
	if k != kindPreview && k != kindSeek {
		return config{}, fmt.Errorf("%w: -kind は preview か seek にしてください（%s）", errUsage, *kindName)
	}
	if *runs < 1 {
		return config{}, fmt.Errorf("%w: -runs は 1 以上にしてください（%d）", errUsage, *runs)
	}
	if flags.NArg() == 0 {
		return config{}, fmt.Errorf("%w: 測る動画を1つ以上渡してください", errUsage)
	}
	return config{kind: k, runs: *runs, videos: flags.Args()}, nil
}

// deps は外部プロセスに触れる処理である。テストから差し替える。
type deps struct {
	probeDuration func(ctx context.Context, path string) (int64, error)
	generate      func(ctx context.Context, videoPath, output string, durationMs int64) error
	// generateSeek はシーク用サムネイルの一式を既にある outputDir へ書く。
	generateSeek func(ctx context.Context, videoPath, outputDir string, durationMs int64) error
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
		generate:     media.GeneratePreview,
		generateSeek: media.GenerateSeekThumbnailSet,
		peakRSS:      childrenPeakRSS,
	}
}

// result は1つの入力の計測結果である。
type result struct {
	kind       kind
	video      string
	durationMs int64
	runs       []time.Duration
	peakBytes  int64
	peakOK     bool
	// outputs は回ごとの出力の集計である。シーク用だけが持つ。
	outputs []outputStats
}

// outputStats は1回の生成が書いた出力のファイル数と合計バイト数である。
type outputStats struct {
	files int
	bytes int64
}

// countOutputs は dir の下の通常ファイルを全部数え、合計バイト数を足す。
func countOutputs(dir string) (outputStats, error) {
	var stats outputStats
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		stats.files++
		stats.bytes += info.Size()
		return nil
	})
	return stats, err
}

// runOnce は1回の生成を走らせて壁時計時間を返し、出力を消す。
func runOnce(ctx context.Context, cfg config, d deps, workDir, video string, durationMs int64) (time.Duration, *outputStats, error) {
	if cfg.kind == kindSeek {
		outputDir := filepath.Join(workDir, "seek")
		if err := os.Mkdir(outputDir, 0o700); err != nil {
			return 0, nil, fmt.Errorf("出力ディレクトリを作れません (%s): %w", outputDir, err)
		}
		defer func() { _ = os.RemoveAll(outputDir) }()
		started := time.Now()
		if err := d.generateSeek(ctx, video, outputDir, durationMs); err != nil {
			return 0, nil, fmt.Errorf("シーク用サムネイルを生成できません (%s): %w", video, err)
		}
		elapsed := time.Since(started)
		stats, err := countOutputs(outputDir)
		if err != nil {
			return 0, nil, fmt.Errorf("生成した出力を数えられません (%s): %w", outputDir, err)
		}
		if err := os.RemoveAll(outputDir); err != nil {
			return 0, nil, fmt.Errorf("生成した出力を消せません (%s): %w", outputDir, err)
		}
		return elapsed, &stats, nil
	}
	output := filepath.Join(workDir, "preview.mp4")
	started := time.Now()
	if err := d.generate(ctx, video, output, durationMs); err != nil {
		return 0, nil, fmt.Errorf("プレビューを生成できません (%s): %w", video, err)
	}
	elapsed := time.Since(started)
	if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, nil, fmt.Errorf("生成した出力を消せません (%s): %w", output, err)
	}
	return elapsed, nil, nil
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
		r := result{kind: cfg.kind, video: video, durationMs: durationMs}
		for range cfg.runs {
			elapsed, stats, err := runOnce(ctx, cfg, d, workDir, video, durationMs)
			if err != nil {
				return results, err
			}
			r.runs = append(r.runs, elapsed)
			if stats != nil {
				r.outputs = append(r.outputs, *stats)
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
	label := "動くプレビュー"
	if r.kind == kindSeek {
		label = "シーク用サムネイル"
	}
	fmt.Fprintf(&b, "%s（長さ %.3f 秒、%s）\n", r.video, float64(r.durationMs)/1000, label)
	for i, elapsed := range r.runs {
		fmt.Fprintf(&b, "  %d 回目: %.3f 秒", i+1, elapsed.Seconds())
		if i < len(r.outputs) {
			o := r.outputs[i]
			fmt.Fprintf(&b, "（ファイル数 %d、合計 %.1f MiB = %d バイト）", o.files, float64(o.bytes)/(1024*1024), o.bytes)
		}
		b.WriteString("\n")
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

// stopSignals はベンチマークを中断する信号である。SIGTERM も受けないと、Go の既定の
// 動作で親だけが終わり、GeneratePreview の ffmpeg が動き続けて一時出力も残る。
// どちらもコンテキストのキャンセルに変えて、ffmpeg を止め一時ディレクトリを消す。
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, stopSignals...)
}

func main() {
	ctx, stop := signalContext(context.Background())
	code := run(ctx, os.Args[1:], productionDeps(), os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
