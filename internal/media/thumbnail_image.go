package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // 適用する画像の形式として読めるようにする。
	_ "image/jpeg" // 同上。
	"image/png"    // 同上。ComfyUI の出力は PNG である。
	"math"
	"os/exec"
	"strconv"

	_ "golang.org/x/image/webp" // 同上。参考画像と同じく、提供元の出力に WebP を受け付ける。

	"github.com/syudead/vv/internal/domain"
)

// thumbnailLongSide は代表サムネイルの長辺（px）である。抽出したサムネイル（thumbnailScale）と
// 同じにする。
const thumbnailLongSide = 640

// thumbnailSourceLongSide は適用した画像の元の画像の長辺の上限（px）である
// （specs/046-thumbnail-image-upload/research.md R-3）。
const thumbnailSourceLongSide = 2048

// FitThumbnailImage は、所有者が適用する画像 data を動画の表示上の縦横比 aspect（横÷縦）に
// 合わせ、代表サムネイルの JPEG を返す（specs/045-comfyui-thumbnails/research.md R-7）。
//
// aspect が正なら、長辺 640px のその比率の枠を覆うまで拡大縮小し、はみ出した分を中央で
// 切り抜く。帯を足すと、カードの object-contain の枠の中に2つ目の縁が出る。aspect が 0 なら
// 画像自身の縦横比を保ち、長辺を 640px にする。品質は抽出したサムネイル（thumbnailArgs）と
// 同じ -q:v 4 である。
//
// data を画像として読めなければ domain.ErrThumbnailImageInvalid を返す。WebP は Go で読んだ画像を
// PNG にして ffmpeg へ渡し、ffmpeg の WebP の読み手の有無に左右されないようにする。
func FitThumbnailImage(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	_, data, err := decodeThumbnailImage(data)
	if err != nil {
		return nil, err
	}
	filter := thumbnailScale
	if aspect > 0 {
		filter = fitThumbnailFilter(fitThumbnailSize(aspect))
	}
	return runFitThumbnail(ctx, data, filter, "4")
}

// FitThumbnailSource は、所有者が適用する画像 data を FitThumbnailImage と同じく動画の表示上の
// 縦横比 aspect（横÷縦、0 は画像自身の比率）に中央で切り抜き、長辺を最大 2048px にした JPEG を
// 返す（specs/046-thumbnail-image-upload/research.md R-3）。今のサムネイルからの編集の元になる。
//
// 切り抜いた大きさが 2048px 以下なら拡大しない。品質は -q:v 2 である。読めない画像は
// domain.ErrThumbnailImageInvalid を返す。
func FitThumbnailSource(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	img, data, err := decodeThumbnailImage(data)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	width, height := fitThumbnailSourceSize(bounds.Dx(), bounds.Dy(), aspect)
	return runFitThumbnail(ctx, data, fitThumbnailFilter(width, height), "2")
}

// decodeThumbnailImage は適用する画像 data を読み、ffmpeg に渡すバイト列とともに返す。WebP は
// PNG にする。読めなければ domain.ErrThumbnailImageInvalid を返す。
func decodeThumbnailImage(data []byte) (image.Image, []byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, errors.Join(domain.ErrThumbnailImageInvalid, err)
	}
	if format == "webp" {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, nil, fmt.Errorf("cannot convert the WebP thumbnail image to PNG: %w", err)
		}
		data = buf.Bytes()
	}
	return img, data, nil
}

// runFitThumbnail は data を ffmpeg の絵の加工 filter に通し、品質 quality（-q:v）の JPEG を返す。
func runFitThumbnail(ctx context.Context, data []byte, filter, quality string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	cmd := command(ctx, thumbnailCommand, fitThumbnailArgs(filter, quality)...)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s failed to fit the thumbnail image: %s", thumbnailCommand, firstLine(exitErr.Stderr))
		}
		return nil, fmt.Errorf("cannot run %s: %w", thumbnailCommand, err)
	}
	if len(output) == 0 {
		return nil, errors.New("the fitted thumbnail image is empty")
	}
	return output, nil
}

// fitThumbnailArgs は runFitThumbnail が ffmpeg に渡す引数を組み立てる。入力は標準入力、
// 出力は標準出力の JPEG 1枚である。
func fitThumbnailArgs(filter, quality string) []string {
	return []string{
		"-v", "error",
		"-i", "pipe:0",
		"-frames:v", "1",
		"-vf", filter,
		"-q:v", quality,
		"-c:v", "mjpeg",
		"-f", "image2pipe",
		"pipe:1",
	}
}

// fitThumbnailFilter は width×height の枠を覆うまで拡大縮小し、はみ出した分を中央で切り抜く
// ffmpeg の絵の加工である。
func fitThumbnailFilter(width, height int) string {
	w, h := strconv.Itoa(width), strconv.Itoa(height)
	return "scale=" + w + ":" + h + ":force_original_aspect_ratio=increase,crop=" + w + ":" + h + ",setsar=1"
}

// fitThumbnailSize は縦横比 aspect（正）の、長辺 640px の大きさを返す。短辺は偶数に丸める
// （thumbnailScale の force_divisible_by=2 と同じ）。
func fitThumbnailSize(aspect float64) (width, height int) {
	short := func(value float64) int {
		even := int(math.Round(value/2)) * 2
		return max(even, 2)
	}
	if aspect >= 1 {
		return thumbnailLongSide, short(thumbnailLongSide / aspect)
	}
	return short(thumbnailLongSide * aspect), thumbnailLongSide
}

// fitThumbnailSourceSize は width×height の画像を縦横比 aspect（0 以下は画像自身の比率）に中央で
// 切り抜き、長辺が 2048px を超えれば 2048px に縮めた大きさを返す。拡大はしない。辺は偶数に
// 丸める（fitThumbnailSize と同じ）。
func fitThumbnailSourceSize(width, height int, aspect float64) (int, int) {
	w, h := float64(width), float64(height)
	if aspect > 0 && w/h > aspect {
		w = h * aspect
	} else if aspect > 0 {
		h = w / aspect
	}
	if long := math.Max(w, h); long > thumbnailSourceLongSide {
		w, h = w*thumbnailSourceLongSide/long, h*thumbnailSourceLongSide/long
	}
	even := func(value float64) int {
		return max(int(math.Round(value/2))*2, 2)
	}
	return even(w), even(h)
}

// FitThumbnailImage は FitThumbnailImage を呼ぶ。
func (a *Assets) FitThumbnailImage(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	return FitThumbnailImage(ctx, data, aspect)
}

// FitThumbnailSource は FitThumbnailSource を呼ぶ。
func (a *Assets) FitThumbnailSource(ctx context.Context, data []byte, aspect float64) ([]byte, error) {
	return FitThumbnailSource(ctx, data, aspect)
}
