// tagsbench measures the production tags screen; see docs/how-to/tags-admin-benchmark.md.
// Seed through store interfaces using location records without media files; benchmark mutations run on a fresh copy of the seed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/scripts/devtools"
)

// Default to ten videos per tag; use -videos 30000 for the 30,000-tag case to isolate tag-count effects.
const videosPerTag = 10

// Leave roughly 9% of tags unused (one in eleven).
const unusedEvery = 11

// Attach each heavy tag to one tenth of all videos.
const heavyTags = 10

// Uniform upper bound for light-tag video counts.
const maxLightCount = 60

type config struct {
	scale int
	// Defaults to scale * videosPerTag when -videos is omitted.
	videos    int
	skipBuild bool
	chromium  string
}

// Separate usage errors from runtime failures by exit code.
var errUsage = errors.New("使い方の誤り")

func parseArgs(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("tagsbench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "使い方: go run ./scripts/tagsbench -scale N [-videos N] [-skip-build] [-chromium PATH]")
		flags.PrintDefaults()
	}
	scale := flags.Int("scale", 0, "規模の名前（タグの数）（例: 1000、3000、30000）")
	videos := flags.Int("videos", 0, "動画の数。省けば -scale の 10 倍（例: -scale 30000 -videos 30000）")
	skipBuild := flags.Bool("skip-build", false, "bin/mdm をビルドし直さずに使う")
	chromium := flags.String("chromium", "", "Playwright が持つものの代わりに使う Chromium の実行ファイル")
	if err := flags.Parse(args); err != nil {
		return config{}, fmt.Errorf("%w: %w", errUsage, err)
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("%w: 余分な引数があります（%v）", errUsage, flags.Args())
	}
	if *scale < 1 {
		return config{}, fmt.Errorf("%w: -scale は 1 以上にしてください（%d）", errUsage, *scale)
	}
	// Distinguish omitted -videos from an explicit zero.
	videoCount := *scale * videosPerTag
	videosSet := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "videos" {
			videosSet = true
		}
	})
	if videosSet {
		if *videos < 1 {
			return config{}, fmt.Errorf("%w: -videos は 1 以上にしてください（%d）", errUsage, *videos)
		}
		videoCount = *videos
	}
	return config{scale: *scale, videos: videoCount, skipBuild: *skipBuild, chromium: *chromium}, nil
}

// Include nondefault video counts in the directory name to avoid reusing a different seed.
func (c config) dataName() string {
	if c.videos == c.scale*videosPerTag {
		return fmt.Sprint(c.scale)
	}
	return fmt.Sprintf("%d-videos-%d", c.scale, c.videos)
}

// videos contains zero-based indices; an empty list represents an unused tag.
type tagPlan struct {
	name      string
	tentative bool
	videos    []int
}

// Mix Latin and Japanese names to exercise both forms of foldForMatch.
var nameWords = []string{
	"action", "アニメ", "drama", "旅行", "comedy", "料理", "documentary", "音楽",
	"horror", "スポーツ", "romance", "ゲーム", "thriller", "ライブ", "fantasy", "自然",
	"mystery", "ニュース", "western", "子ども", "musical", "歴史", "sci-fi", "インタビュー",
}

// Deterministic plan: alternate confirmed and tentative tags, leaving every unusedEvery-th tag unused.
func planTags(scale, videoCount int) []tagPlan {
	rng := rand.New(rand.NewPCG(uint64(scale), 36))
	plans := make([]tagPlan, 0, scale)
	for i := range scale {
		plan := tagPlan{
			name:      fmt.Sprintf("%s %d", nameWords[i%len(nameWords)], i+1),
			tentative: i%2 == 1,
		}
		if i%unusedEvery != unusedEvery-1 {
			count := 1 + rng.IntN(maxLightCount)
			if i < heavyTags {
				// Keep heavy tags nonempty even with fewer than ten videos.
				count = max(1, videoCount/10)
			}
			plan.videos = pickVideos(rng, videoCount, min(count, videoCount))
		}
		plans = append(plans, plan)
	}
	return plans
}

// Pick distinct indices in [0, n).
func pickVideos(rng *rand.Rand, n, count int) []int {
	seen := make(map[int]bool, count)
	picked := make([]int, 0, count)
	for len(picked) < count {
		v := rng.IntN(n)
		if !seen[v] {
			seen[v] = true
			picked = append(picked, v)
		}
	}
	return picked
}

type seedSummary struct {
	tags      int
	unused    int
	tentative int
	videos    int
}

// Seed locations directly under mediaDir to avoid tags derived from ancestor folders.
func seed(ctx context.Context, dataDir, mediaDir string, scale, videoCount int, progress io.Writer) (seedSummary, error) {
	db, err := store.OpenContext(ctx, dataDir)
	if err != nil {
		return seedSummary{}, err
	}
	defer func() { _ = db.Close() }()
	if _, err := store.Migrate(ctx, db); err != nil {
		return seedSummary{}, err
	}
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		return seedSummary{}, fmt.Errorf("登録フォルダを書けません: %w", err)
	}

	ids := make([]int64, videoCount)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range videoCount {
		file := domain.VideoFile{
			Path:       filepath.Join(mediaDir, fmt.Sprintf("bench-%06d.mp4", i+1)),
			Title:      fmt.Sprintf("bench-%06d", i+1),
			ContentKey: fmt.Sprintf("tagsbench-%d-%06d", scale, i+1),
			SizeBytes:  int64(1_000_000 + i),
			MTime:      base.Add(time.Duration(i) * time.Minute),
			Container:  "mp4",
			AddedAt:    base.Add(time.Duration(i) * time.Minute),
		}
		result, err := db.ScanIndex().UpsertVideo(ctx, file)
		if err != nil {
			return seedSummary{}, fmt.Errorf("動画の行を書けません (%s): %w", file.Path, err)
		}
		ids[i] = result.ID
		if (i+1)%5000 == 0 {
			_, _ = fmt.Fprintf(progress, "  動画 %d / %d\n", i+1, videoCount)
		}
	}

	summary := seedSummary{videos: videoCount}
	tags := db.Tags()
	for i, plan := range planTags(scale, videoCount) {
		refs := make([]domain.VideoRef, 0, max(len(plan.videos), 1))
		for _, v := range plan.videos {
			refs = append(refs, domain.VideoRef{ID: ids[v]})
		}
		if len(refs) == 0 {
			// Create unused tags by attaching and then detaching them; detaching preserves the tag.
			refs = append(refs, domain.VideoRef{ID: ids[0]})
		}
		if _, err := tags.ApplyVideoTags(ctx, refs, domain.VideoTagsAdd, []string{plan.name}, plan.tentative); err != nil {
			return seedSummary{}, fmt.Errorf("タグを付けられません (%s): %w", plan.name, err)
		}
		if len(plan.videos) == 0 {
			if _, err := tags.ApplyVideoTags(ctx, refs, domain.VideoTagsRemove, []string{plan.name}, false); err != nil {
				return seedSummary{}, fmt.Errorf("タグを外せません (%s): %w", plan.name, err)
			}
		}
		if (i+1)%5000 == 0 || (scale <= 5000 && (i+1)%500 == 0) {
			_, _ = fmt.Fprintf(progress, "  タグ %d / %d\n", i+1, scale)
		}
	}

	listedPage, err := tags.ListTags(ctx, domain.TagListQuery{})
	if err != nil {
		return seedSummary{}, err
	}
	listed := listedPage.Items
	for _, tag := range listed {
		summary.tags++
		if tag.VideoCount == 0 {
			summary.unused++
		}
		if tag.Tentative {
			summary.tentative++
		}
	}
	if summary.tags != scale {
		return summary, fmt.Errorf("タグの数が規模と合いません（%d 個、期待 %d 個）", summary.tags, scale)
	}
	return summary, nil
}

// Build missing seeds under a temporary name, then rename to avoid reusing partial data.
func ensureSeed(ctx context.Context, scaleDir string, scale, videoCount int, stdout io.Writer) (string, error) {
	seedDir := filepath.Join(scaleDir, "seed")
	if _, err := os.Stat(seedDir); err == nil {
		_, _ = fmt.Fprintln(stdout, "規模のデータを使う:", seedDir)
		return seedDir, nil
	}
	mediaDir := filepath.Join(scaleDir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return "", err
	}
	partial := seedDir + ".partial"
	if err := os.RemoveAll(partial); err != nil {
		return "", err
	}
	if err := os.MkdirAll(partial, 0o755); err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(stdout, "規模のデータを作る（タグ %d 個・動画 %d 本）: %s\n", scale, videoCount, seedDir)
	started := time.Now()
	summary, err := seed(ctx, partial, mediaDir, scale, videoCount, stdout)
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(stdout, "作った: タグ %d 個（本数 0 が %d 個、仮が %d 個）・動画 %d 本（%.0f 秒）\n",
		summary.tags, summary.unused, summary.tentative, summary.videos, time.Since(started).Seconds())
	if err := os.Rename(partial, seedDir); err != nil {
		return "", err
	}
	return seedDir, nil
}

// copyDir replaces dst before copying.
func copyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func freeAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	return address, listener.Close()
}

type server struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func startServer(root, address, dataDir string, stdout, stderr io.Writer) (*server, error) {
	binary := filepath.Join(root, "bin", "mdm")
	if _, err := os.Stat(binary); err != nil {
		if _, exeErr := os.Stat(binary + ".exe"); exeErr == nil {
			binary += ".exe"
		} else {
			return nil, fmt.Errorf("ビルド済みの単一バイナリがありません (%s)。-skip-build を外してください: %w", binary, err)
		}
	}
	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "MDM_ADDR="+address, "MDM_DATA_DIR="+dataDir)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("VVMDM を起動できません: %w", err)
	}
	s := &server{cmd: cmd, done: make(chan struct{})}
	go func() {
		s.err = cmd.Wait()
		close(s.done)
	}()
	return s, nil
}

func (s *server) stop() {
	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = s.cmd.Process.Kill()
	}
	select {
	case <-s.done:
	case <-time.After(20 * time.Second):
		_ = s.cmd.Process.Kill()
		<-s.done
	}
}

func (s *server) waitHealthy(ctx context.Context, address string) error {
	health := "http://" + address + "/api/health"
	deadline := time.After(60 * time.Second)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, health, nil)
		if err != nil {
			return err
		}
		if response, err := http.DefaultClient.Do(request); err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return fmt.Errorf("VVMDM が起動中に停止しました（%w）。上の出力を確認してください", s.err)
		case <-deadline:
			return errors.New("VVMDM が 60 秒以内に応答しませんでした")
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// runBench writes the Playwright results table to resultPath.
func runBench(ctx context.Context, root string, cfg config, baseURL, resultPath string, stdout, stderr io.Writer) error {
	webRoot := filepath.Join(root, "web")
	cli := filepath.Join(webRoot, "node_modules", "@playwright", "test", "cli.js")
	if _, err := os.Stat(cli); err != nil {
		return fmt.Errorf("web の Playwright がありません。task setup を実行してください: %w", err)
	}
	cmd := exec.CommandContext(ctx, "node", cli, "test", "--config", filepath.Join("bench", "playwright.config.ts"))
	cmd.Dir = webRoot
	cmd.Env = append(os.Environ(),
		"TAGSBENCH_BASE_URL="+baseURL,
		fmt.Sprintf("TAGSBENCH_SCALE=%d", cfg.scale),
		fmt.Sprintf("TAGSBENCH_VIDEOS=%d", cfg.videos),
		"TAGSBENCH_RESULT="+resultPath,
	)
	if cfg.chromium != "" {
		cmd.Env = append(cmd.Env, "TAGSBENCH_CHROMIUM="+cfg.chromium)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("計測が失敗しました: %w", err)
	}
	return nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if err := bench(ctx, cfg, stdout, stderr); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func bench(ctx context.Context, cfg config, stdout, stderr io.Writer) error {
	root, err := devtools.RepositoryRoot()
	if err != nil {
		return err
	}
	scaleDir := filepath.Join(root, ".local", "tagsbench", cfg.dataName())
	seedDir, err := ensureSeed(ctx, scaleDir, cfg.scale, cfg.videos, stdout)
	if err != nil {
		return err
	}
	if !cfg.skipBuild {
		if err := devtools.Run(root, "go", "run", "./scripts/build"); err != nil {
			return err
		}
	}
	runDir := filepath.Join(scaleDir, "run")
	if err := copyDir(seedDir, runDir); err != nil {
		return fmt.Errorf("規模のデータを写せません: %w", err)
	}
	address, err := freeAddress()
	if err != nil {
		return err
	}
	srv, err := startServer(root, address, runDir, io.Discard, stderr)
	if err != nil {
		return err
	}
	defer srv.stop()
	if err := srv.waitHealthy(ctx, address); err != nil {
		return err
	}
	resultPath := filepath.Join(scaleDir, "result.md")
	_ = os.Remove(resultPath)
	if err := runBench(ctx, root, cfg, "http://"+address, resultPath, stdout, stderr); err != nil {
		return err
	}
	result, err := os.ReadFile(resultPath)
	if err != nil {
		return fmt.Errorf("計測の結果を読めません (%s): %w", resultPath, err)
	}
	_, _ = fmt.Fprintf(stdout, "\n%s\n結果: %s\n", result, resultPath)
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
