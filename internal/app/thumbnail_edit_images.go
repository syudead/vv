package app

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // 参考画像と出力の形式として読めるようにする。
	"image/png"

	_ "golang.org/x/image/webp" // 同上。標準ライブラリには WebP の読み手が無い。

	"github.com/syudead/vv/internal/domain"
)

// サムネイルの編集の画像の確かめ（contracts/screen-api.md の Run an edit、
// docs/design-docs/thumbnail-editing.md）。マスクと参考画像は何も上げる前に、提供元の出力は
// succeeded にする前に、最後まで読めるかを確かめる。

// thumbnailEditMaxPixels は、確かめるために最後まで読む画像の画素数の上限である。見出しだけ
// 大きな画像で読み込みの記憶域が膨らまないよう、これを越える画像は読めないものとして扱う。
const thumbnailEditMaxPixels = 8192 * 8192

// errThumbnailEditImageUnreadable は画像を最後まで読めないことを表す。
var errThumbnailEditImageUnreadable = errors.New("the image cannot be decoded")

// errImageTooManyPixels は画像の見出しが言う画素数が上限を越えることを表す。
var errImageTooManyPixels = errors.New("the image has too many pixels")

// checkThumbnailEditMask は、マスク mask が nil か、コマ frame と同じ大きさの、最後まで読める
// PNG かを確かめる。
func checkThumbnailEditMask(mask []byte, frame domain.ThumbnailFrame) error {
	if mask == nil {
		return nil
	}
	config, err := png.DecodeConfig(bytes.NewReader(mask))
	if err != nil || config.Width != frame.Width || config.Height != frame.Height {
		return domain.ErrThumbnailEditMaskInvalid
	}
	if _, err := png.Decode(bytes.NewReader(mask)); err != nil {
		return domain.ErrThumbnailEditMaskInvalid
	}
	return nil
}

// checkThumbnailEditReference は、参考画像 reference が nil か、上限以下の、最後まで読める
// PNG・JPEG・WebP かを確かめる。
func checkThumbnailEditReference(reference []byte) error {
	if reference == nil {
		return nil
	}
	if len(reference) == 0 || len(reference) > domain.ThumbnailEditMaxReferenceBytes {
		return domain.ErrThumbnailEditReferenceInvalid
	}
	if err := checkThumbnailEditImage(reference); err != nil {
		return domain.ErrThumbnailEditReferenceInvalid
	}
	return nil
}

// checkThumbnailEditImage は data が最後まで読める、上限以下の PNG・JPEG・WebP かを確かめる。
func checkThumbnailEditImage(data []byte) error {
	if err := checkImage(data, thumbnailEditMaxPixels); err != nil {
		return errThumbnailEditImageUnreadable
	}
	return nil
}

// checkImage は data が最後まで読める PNG・JPEG・WebP かを確かめる。どれも見出しで画素数が
// maxPixels 以下かを確かめてから最後まで読み、途中で切れた画像や壊れた画像は読めないものとする。
// 画素数が上限を越えれば errImageTooManyPixels、読めなければ errThumbnailEditImageUnreadable を
// 返す。サムネイルの編集の画像とアップロードの画像（thumbnail_upload.go）が共有する。
func checkImage(data []byte, maxPixels int64) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") {
		return errThumbnailEditImageUnreadable
	}
	if config.Width <= 0 || config.Height <= 0 {
		return errThumbnailEditImageUnreadable
	}
	if int64(config.Width)*int64(config.Height) > maxPixels {
		return errImageTooManyPixels
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return errThumbnailEditImageUnreadable
	}
	return nil
}
