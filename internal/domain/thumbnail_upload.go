package domain

import (
	"errors"
	"fmt"
)

// 所有者が手元の画像をサムネイルにするアップロードの上限と誤りである
// （specs/046-thumbnail-image-upload/research.md R-2、contracts/screen-api.md）。

const (
	// ThumbnailUploadMaxBytes はアップロードの image 部分の大きさの上限（64 MiB）である。
	ThumbnailUploadMaxBytes = 64 << 20
	// ThumbnailUploadMaxPixels はアップロードの画像の見出しが言う画素数の上限（1 億 2000 万）である。
	// これを越える画像は最後まで読まずに断る。
	ThumbnailUploadMaxPixels = 120_000_000
)

var (
	// ErrThumbnailUploadTooLarge は、アップロードの画像が大きさか画素数の上限を越えることを表す。
	// 何も書かない。API では invalid_request（400）/ thumbnail_upload_too_large になり、越えた
	// 上限は ThumbnailUploadTooLargeError が持つ。
	ErrThumbnailUploadTooLarge = errors.New("the uploaded image is too large")
	// ErrThumbnailUploadInvalid は、アップロードの画像が PNG・JPEG・静止画の WebP でないか、
	// 最後まで読めないことを表す。何も書かない。API では invalid_request（400）/
	// thumbnail_upload_invalid になる。
	ErrThumbnailUploadInvalid = errors.New("the uploaded image is not a readable PNG, JPEG or still WebP")
)

// ThumbnailUploadTooLargeError は、アップロードの画像が越えた上限 Limit（バイト数か画素数）を
// 持つ ErrThumbnailUploadTooLarge である。
type ThumbnailUploadTooLargeError struct {
	Limit int
}

func (e *ThumbnailUploadTooLargeError) Error() string {
	return fmt.Sprintf("%s: the limit is %d", ErrThumbnailUploadTooLarge.Error(), e.Limit)
}

// Unwrap は errors.Is で ErrThumbnailUploadTooLarge として見分けられるようにする。
func (e *ThumbnailUploadTooLargeError) Unwrap() error {
	return ErrThumbnailUploadTooLarge
}
