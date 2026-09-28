package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	seekThumbnailCommand = "ffmpeg"
	seekThumbnailTimeout = 30 * time.Minute
	seekSpriteFastScale  = "scale=min(160\\,iw):min(160\\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2"
	seekSpriteParallel   = 4
	// seekSpriteReadParallel は索引から求めたキーフレームを同時に読む数である。
	seekSpriteReadParallel = 8
)

// GenerateSeekSprite はシーク用スプライトのシートを生成する。
// MP4／MOV の H.264／HEVC は索引から各区間のキーフレームだけを読んで作る。
// それ以外の入力は区間ごとに時刻シークし、それもできない入力では全編復号へ戻る。
// 一時データは outputDir 内で片付け、完成物の公開は呼び出し側が受け持つ。
func GenerateSeekSprite(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	processCtx, cancel := context.WithTimeout(ctx, seekThumbnailTimeout)
	defer cancel()
	err := generateSeekSprite(processCtx, videoPath, outputDir, layout)
	if processCtx.Err() != nil {
		return fmt.Errorf("シークサムネイル生成を中断しました: %w", processCtx.Err())
	}
	return err
}

func generateSeekSprite(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	err := generateSeekSpriteFromIndex(ctx, videoPath, outputDir, layout)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !errors.Is(err, errSeekIndexUnsupported) {
		slog.WarnContext(ctx, "シークサムネイルを索引から生成できないため区間ごとに抽出します", "error", err)
	}
	if err := generateSeekSpriteParallel(ctx, videoPath, outputDir, layout); err == nil {
		return nil
	} else if ctx.Err() != nil {
		return ctx.Err()
	} else {
		slog.WarnContext(ctx, "シークサムネイルの区間抽出に失敗したため全編から生成します", "error", err)
	}
	_, err = runSeekFFmpeg(ctx, seekSpriteArgs(videoPath, filepath.Join(outputDir, "%03d.jpg"), layout))
	return err
}

// generateSeekSpriteFromIndex は MP4／MOV の索引を 1 回だけ読み、各コマに使う
// キーフレームを決めてから、そのバイトだけを読んで 1 回の ffmpeg で復号する。
// 索引から取れない入力では errSeekIndexUnsupported を返す。
func generateSeekSpriteFromIndex(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	file, err := os.Open(videoPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	movie, err := readMovieBox(ctx, file, info.Size())
	if err != nil {
		return err
	}
	track, err := parseKeyframeTrack(movie)
	if err != nil {
		return err
	}

	// 同じキーフレームを受け持つコマ（長い GOP や映像の後ろの区間）は、1 回だけ読んで
	// 復号し、画像を複製する。
	picks := track.pick(layout)
	decodeOrder := make(map[int]int, len(picks))
	var unique []int
	for _, pick := range picks {
		if _, ok := decodeOrder[pick]; !ok {
			decodeOrder[pick] = len(unique)
			unique = append(unique, pick)
		}
	}

	temporary, err := os.MkdirTemp(outputDir, ".seek-sprite-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	// 復号の ffmpeg は keys を作業ディレクトリにして動くので、出力先は絶対パスにする。
	if temporary, err = filepath.Abs(temporary); err != nil {
		return err
	}
	keys := filepath.Join(temporary, "keys")
	decoded := filepath.Join(temporary, "decoded")
	for _, dir := range []string{keys, decoded} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return err
		}
	}
	allIDR, err := readKeyframes(ctx, file, track, unique, keys)
	if err != nil {
		return err
	}
	if _, err := runSeekFFmpegIn(ctx, keys, keyframeDecodeArgs(track, len(unique), allIDR, decoded)); err != nil {
		return err
	}
	// 復号できなかったキーフレームがあると、後ろのコマが 1 つずつずれる。枚数が
	// 合わないときは使わない。
	if _, err := os.Stat(filepath.Join(decoded, fmt.Sprintf("%03d.bmp", len(unique)))); err == nil {
		return fmt.Errorf("キーフレーム %d 枚より多い画像が出ました", len(unique))
	}
	for k, pick := range picks {
		source := filepath.Join(decoded, fmt.Sprintf("%03d.bmp", decodeOrder[pick]))
		if err := copyFile(source, filepath.Join(temporary, fmt.Sprintf("%03d.bmp", k))); err != nil {
			return fmt.Errorf("コマ %d のキーフレームを復号できませんでした: %w", k, err)
		}
	}
	return tileSeekSprite(ctx, temporary, outputDir, layout)
}

// keyframeDecodeArgs は、作業ディレクトリに書いた count 枚のキーフレームを順に復号し、
// decoded へ 000.bmp から書く ffmpeg の引数を返す。
//
// IDR は表示順の番号とデコーダの状態を初期化するので、IDR だけなら 1 本の列に
// つないで 1 つのデコーダで復号できる。IDR でないキーフレーム（open GOP の CRA や
// I フレーム）を 1 本の列にすると、番号が前のキーフレームから続けて計算され、
// デコーダが前後を入れ替える。その場合は 1 枚ずつ別の入力にしてデコーダを分け、
// concat で入力の順につなぐ。入力ごとの初期化の分だけ遅い。I フレームは単独で
// 完結するので、IDR でなくても出力させる（showall）。
func keyframeDecodeArgs(track *keyframeTrack, count int, allIDR bool, decoded string) []string {
	format := track.format
	filter := seekSpriteFastScale
	if track.displayFilter != "" {
		filter = track.displayFilter + "," + filter
	}
	args := []string{"-nostdin", "-v", "error"}
	output := []string{"-fps_mode", "passthrough", "-c:v", "bmp", "-start_number", "0", "-y", filepath.Join(decoded, "%03d.bmp")}
	if allIDR {
		names := make([]string, count)
		for i := range names {
			names[i] = keyframeFileName(i, format)
		}
		args = append(args, "-f", format, "-i", "concat:"+strings.Join(names, "|"), "-vf", filter)
		return append(args, output...)
	}
	var inputs strings.Builder
	for i := range count {
		args = append(args, "-threads", "1", "-flags2", "+showall", "-f", format, "-i", keyframeFileName(i, format))
		fmt.Fprintf(&inputs, "[%d:v]", i)
	}
	args = append(args, "-filter_complex", fmt.Sprintf("%sconcat=n=%d:v=1:a=0,%s", inputs.String(), count, filter))
	return append(args, output...)
}

// keyframeFileName は i 番目に読むキーフレームを書くファイルの名前である。
func keyframeFileName(i int, format string) string {
	return fmt.Sprintf("%03d.%s", i, format)
}

// readKeyframes は indexes のキーフレームを並列に読み、ffmpeg が読める形にして
// dir へ 1 枚ずつ書く。すべてが IDR だったかを返す。
func readKeyframes(ctx context.Context, r io.ReaderAt, track *keyframeTrack, indexes []int, dir string) (bool, error) {
	var total int64
	for _, index := range indexes {
		size := track.keyframes[index].size
		if size <= 0 || size > maxKeyframeSize {
			return false, fmt.Errorf("%w: キーフレームの大きさ %d", errSeekIndexUnsupported, size)
		}
		if total += size; total > maxKeyframeTotalSize {
			return false, fmt.Errorf("%w: キーフレームの合計が %d バイトを超える", errSeekIndexUnsupported, int64(maxKeyframeTotalSize))
		}
	}
	semaphore := make(chan struct{}, seekSpriteReadParallel)
	var workers sync.WaitGroup
	var firstErr error
	idr := make([]bool, len(indexes))
	var recordError sync.Once
	for i, index := range indexes {
		frame := track.keyframes[index]
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			workers.Wait()
			return false, ctx.Err()
		}
		workers.Go(func() {
			defer func() { <-semaphore }()
			if ctx.Err() != nil {
				return
			}
			data := make([]byte, frame.size)
			_, err := readAt(ctx, r, data, frame.offset)
			if err != nil {
				err = fmt.Errorf("キーフレームを読めません（位置 %d）: %w", frame.offset, err)
			} else if data, idr[i], err = track.annexB(data); err == nil {
				err = os.WriteFile(filepath.Join(dir, keyframeFileName(i, track.format)), data, 0600)
			}
			if err != nil {
				recordError.Do(func() { firstErr = err })
			}
		})
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return !slices.Contains(idr, false), firstErr
}

// generateSeekSpriteParallel は各区間の先頭へ時刻シークしてコマを並列に取り、シートにする。
// 区間の中にフレームが無い（映像が容器より早く終わるなど）コマは、近いコマの画像を
// 複製する。ffmpeg が失敗したときと、どの区間からも画像が取れないときはエラーを返す。
func generateSeekSpriteParallel(ctx context.Context, videoPath, outputDir string, layout domain.SeekSpriteLayout) error {
	temporary, err := os.MkdirTemp(outputDir, ".seek-sprite-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	semaphore := make(chan struct{}, seekSpriteParallel)
	var workers sync.WaitGroup
	var firstErr error
	var recordError sync.Once
	missing := make([]bool, layout.FrameCount)
frames:
	for frame := range layout.FrameCount {
		select {
		case semaphore <- struct{}{}:
		case <-workCtx.Done():
			break frames
		}
		if workCtx.Err() != nil {
			<-semaphore
			break frames
		}
		workers.Add(1)
		go func(frame int) {
			defer workers.Done()
			defer func() { <-semaphore }()
			at := float64(int64(frame)*layout.IntervalMs) / 1000
			interval := float64(layout.IntervalMs) / 1000
			args := []string{
				"-nostdin", "-v", "error", "-ss", strconv.FormatFloat(at, 'f', 3, 64),
				"-t", strconv.FormatFloat(interval, 'f', 3, 64), "-i", videoPath,
				"-map", "0:V:0?", "-frames:v", "1",
				"-vf", "trim=end=" + strconv.FormatFloat(interval, 'f', 3, 64) + "," + seekSpriteFastScale,
				"-c:v", "bmp", "-f", "rawvideo", "pipe:1",
			}
			data, err := runSeekFFmpeg(workCtx, args)
			if err == nil && len(data) == 0 {
				missing[frame] = true
				return
			}
			if err == nil && (len(data) < 54 || string(data[:2]) != "BM") {
				err = errors.New("シーク位置から画像を抽出できませんでした")
			}
			if err == nil {
				err = os.WriteFile(filepath.Join(temporary, fmt.Sprintf("%03d.bmp", frame)), data, 0600)
			}
			if err != nil {
				recordError.Do(func() { firstErr = fmt.Errorf("コマ %d: %w", frame, err); cancel() })
			}
		}(frame)
	}
	workers.Wait()
	if firstErr != nil {
		return firstErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fillMissingFrames(temporary, missing); err != nil {
		return err
	}
	return tileSeekSprite(ctx, temporary, outputDir, layout)
}

// fillMissingFrames は画像の無いコマに、直前のコマ（無ければ直後のコマ）の画像を置く。
func fillMissingFrames(dir string, missing []bool) error {
	for frame, isMissing := range missing {
		if !isMissing {
			continue
		}
		source := -1
		for earlier := frame - 1; earlier >= 0 && source < 0; earlier-- {
			if !missing[earlier] {
				source = earlier
			}
		}
		for later := frame + 1; later < len(missing) && source < 0; later++ {
			if !missing[later] {
				source = later
			}
		}
		if source < 0 {
			return errors.New("どの区間からも画像を抽出できませんでした")
		}
		if err := copyFile(filepath.Join(dir, fmt.Sprintf("%03d.bmp", source)), filepath.Join(dir, fmt.Sprintf("%03d.bmp", frame))); err != nil {
			return err
		}
	}
	return nil
}

// tileSeekSprite は frameDir のコマ（000.bmp から順に）を並べて outputDir/000.jpg を書く。
func tileSeekSprite(ctx context.Context, frameDir, outputDir string, layout domain.SeekSpriteLayout) error {
	_, err := runSeekFFmpeg(ctx, []string{
		"-nostdin", "-v", "error", "-framerate", "1", "-i", filepath.Join(frameDir, "%03d.bmp"),
		"-vf", fmt.Sprintf("tile=%dx%d,format=yuvj420p", layout.Columns, layout.Rows),
		"-frames:v", "1", "-q:v", "4", "-y", filepath.Join(outputDir, "000.jpg"),
	})
	return err
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0600)
}

func runSeekFFmpeg(ctx context.Context, args []string) ([]byte, error) {
	return runSeekFFmpegIn(ctx, "", args)
}

// runSeekFFmpegIn は dir を作業ディレクトリにして ffmpeg を動かす。入力が多いとき、
// 相対パスで渡してコマンドラインを短く保つ。
func runSeekFFmpegIn(ctx context.Context, dir string, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, seekThumbnailCommand, args...)
	command.Dir = dir
	data, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("ffmpeg がシークサムネイル生成に失敗しました: %w: %s", err, firstLine(exitErr.Stderr))
		}
		return nil, fmt.Errorf("ffmpeg を実行できません: %w", err)
	}
	return data, nil
}

func seekSpriteArgs(videoPath, outputPattern string, layout domain.SeekSpriteLayout) []string {
	filters := []string{
		"tpad=stop_mode=clone:stop=-1",
		"fps=1000/" + strconv.FormatInt(layout.IntervalMs, 10) + ":round=up:eof_action=pass",
		"trim=end_frame=" + strconv.Itoa(layout.FrameCount),
		seekSpriteFastScale,
		"tile=" + strconv.Itoa(layout.Columns) + "x" + strconv.Itoa(layout.Rows),
		"format=yuvj420p",
	}
	return []string{
		"-nostdin",
		"-v", "error",
		"-i", videoPath,
		"-map", "0:V:0?",
		"-vf", strings.Join(filters, ","),
		"-fps_mode", "passthrough",
		"-q:v", "4",
		"-start_number", "0",
		"-y",
		outputPattern,
	}
}

// GenerateSeekThumbnailSet はシーク用サムネイルの一式を outputDir（既にある
// ディレクトリ）へ書く。scripts/previewbench がシーク用の生成を測る境界で、
// 生成方式を変えるときはこの関数の中身を変える。今は durationMs から
// domain.NewSeekSpriteLayout で配置を決め、GenerateSeekSprite でシートを書く。
func GenerateSeekThumbnailSet(ctx context.Context, videoPath, outputDir string, durationMs int64) error {
	return GenerateSeekSprite(ctx, videoPath, outputDir, domain.NewSeekSpriteLayout(durationMs))
}
