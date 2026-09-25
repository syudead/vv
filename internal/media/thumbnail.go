package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// 抽出位置の規則。先頭が黒画面やロゴであることが多いので 10% 地点を
// 採り、長い動画で待たされないよう上限を切る。
const (
	// thumbnailFraction は尺に対する抽出位置の割合。
	thumbnailFraction = 0.10
	// thumbnailMinOffset は抽出位置の下限（秒）。
	thumbnailMinOffset = 1.0
	// thumbnailMaxOffset は抽出位置の上限（秒）。
	thumbnailMaxOffset = 60.0
)

// thumbnailTimeout は ffmpeg 1回に与える上限である。解析（Probe）と同じく、
// 1件で取り込み全体を止めないための上限である。
const thumbnailTimeout = 60 * time.Second

// thumbnailCommand は実行する外部コマンドである。
const thumbnailCommand = "ffmpeg"

// Thumbnail は動画から静止画を1枚取り出し、output へ書く。output の置き場所は
// 呼び出し側（internal/artifacts の一時置き場）が用意する。
//
// 形式は JPEG にする。WebP の方が小さいが、libwebp を含む ffmpeg ビルドを
// 前提にすると実行環境の差で失敗しうる。mjpeg エンコーダはどのビルドにも
// 含まれる。幅 640px でおおむね 30〜60KB であり、一覧 60 件でも 2〜4MB に収まる。
func Thumbnail(ctx context.Context, videoPath string, durationMs int64, output string) error {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	offset := thumbnailOffset(durationMs)
	if err := runThumbnail(ctx, videoPath, offset, output); err != nil {
		// 指定した位置でフレームが取れないことがある（可変フレームレート、
		// 索引の壊れたファイル）。1枚も無いより先頭の1枚の方がよいので、
		// 一度だけ先頭から取り直す。
		if offset == 0 {
			return err
		}
		if retryErr := runThumbnail(ctx, videoPath, 0, output); retryErr != nil {
			return err
		}
	}
	return nil
}

// runThumbnail は ffmpeg を1回実行し、画像が実際に書かれたことまで確かめる。
//
// ffmpeg は指定した位置にフレームが無いとき、終了コード 0 のまま何も出力せずに
// 終わる。出力の有無まで見ないと、生成できていないのに成功として記録される。
func runThumbnail(ctx context.Context, videoPath string, offsetSec float64, output string) error {
	args := thumbnailArgs(videoPath, offsetSec, output)
	if _, err := exec.CommandContext(ctx, thumbnailCommand, args...).Output(); err != nil {
		_ = os.Remove(output)

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf(
				"%s が失敗しました (%s): %s", thumbnailCommand, videoPath, firstLine(exitErr.Stderr))
		}
		return fmt.Errorf("%s を実行できません (%s): %w", thumbnailCommand, videoPath, err)
	}

	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		// 途中まで書かれた画像を残さない。半端な JPEG を配信すると、
		// 生成済みなのか壊れているのかが利用者から区別できない。
		_ = os.Remove(output)
		return fmt.Errorf("サムネイルが生成されませんでした (%s、位置 %.3f 秒)", videoPath, offsetSec)
	}
	return nil
}

// thumbnailOffset は抽出位置（秒）を返す。尺が不明・不正な場合は下限を使う。
//
// 下限（1 秒）が尺を越える短い動画では、丸めずに 10% 地点を使う。下限へ
// 丸めると末尾ちょうど、あるいはその先を指すことになり、ffmpeg は終了コード 0
// のまま1枚も出力しない（失敗として現れないので、原因が分かりにくい）。
func thumbnailOffset(durationMs int64) float64 {
	if durationMs <= 0 {
		return thumbnailMinOffset
	}

	seconds := float64(durationMs) / 1000
	offset := min(max(seconds*thumbnailFraction, thumbnailMinOffset), thumbnailMaxOffset)

	if offset >= seconds {
		// 10% 地点は必ず尺の内側にある。
		return seconds * thumbnailFraction
	}
	return offset
}

// thumbnailScale は、向きを問わず長辺を 640px に揃える。縦長の動画でも横長と同じ
// 大きさの枠に収まるので、縦長だけ画像が大きくなることがない。
const thumbnailScale = "scale=640:640:force_original_aspect_ratio=decrease:force_divisible_by=2"

// thumbnailArgs は ffmpeg に渡す1回分の引数を組み立てる。
//
// -ss を -i の前に置くとキーフレーム単位の高速シークになり、長い動画でも
// 一定時間で終わる。後ろに置くと先頭から復号することになる。
func thumbnailArgs(videoPath string, offsetSec float64, output string) []string {
	return []string{
		"-nostdin",
		"-v", "error",
		"-ss", strconv.FormatFloat(offsetSec, 'f', 3, 64),
		"-i", videoPath,
		"-frames:v", "1",
		"-vf", thumbnailScale,
		"-q:v", "4",
		"-y",
		output,
	}
}
