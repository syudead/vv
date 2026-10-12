package app

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg" // 参考画像と出力の形式として読めるようにする。
	"image/png"

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

// checkThumbnailEditImage は data が最後まで読める PNG・JPEG・WebP かを確かめる。PNG と JPEG は
// 標準ライブラリで読み切る。WebP は標準ライブラリで読めないので、RIFF のチャンクがすべて揃い、
// 画像のチャンクの見出しが正しいかを確かめる。
func checkThumbnailEditImage(data []byte) error {
	if isWebPContainer(data) {
		return checkWebP(data)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return errThumbnailEditImageUnreadable
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > thumbnailEditMaxPixels {
		return errThumbnailEditImageUnreadable
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return errThumbnailEditImageUnreadable
	}
	return nil
}

// isWebPContainer は data が WebP の RIFF の見出しで始まるかを返す。
func isWebPContainer(data []byte) bool {
	return len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

// checkWebP は WebP の data の RIFF のチャンクを順に読み、どれも途中で切れておらず、画像の
// チャンク（VP8・VP8L・ANMF）を 1 つ以上持ち、VP8 と VP8L の見出しが正しいかを確かめる。
func checkWebP(data []byte) error {
	size := int64(binary.LittleEndian.Uint32(data[4:8]))
	if size < 4 || size+8 > int64(len(data)) {
		return errThumbnailEditImageUnreadable
	}
	body := data[12 : 8+size]
	hasImage := false
	for len(body) > 0 {
		if len(body) < 8 {
			return errThumbnailEditImageUnreadable
		}
		fourCC := string(body[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(body[4:8]))
		if chunkSize > int64(len(body)-8) {
			return errThumbnailEditImageUnreadable
		}
		payload := body[8 : 8+chunkSize]
		switch fourCC {
		case "VP8 ":
			if !validVP8(payload) {
				return errThumbnailEditImageUnreadable
			}
			hasImage = true
		case "VP8L":
			if !validVP8L(payload) {
				return errThumbnailEditImageUnreadable
			}
			hasImage = true
		case "ANMF":
			hasImage = true
		}
		next := 8 + chunkSize + chunkSize%2
		if next > int64(len(body)) {
			next = int64(len(body))
		}
		body = body[next:]
	}
	if !hasImage {
		return errThumbnailEditImageUnreadable
	}
	return nil
}

// validVP8 は VP8 の非可逆の画像のチャンクが、キーフレームの見出しと開始符号、0 でない大きさを
// 持ち、最初の区画がチャンクに収まるかを返す。
func validVP8(payload []byte) bool {
	if len(payload) < 10 {
		return false
	}
	tag := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16
	keyFrame := tag&1 == 0
	firstPartition := int(tag >> 5)
	if !keyFrame || firstPartition > len(payload)-10 {
		return false
	}
	if payload[3] != 0x9d || payload[4] != 0x01 || payload[5] != 0x2a {
		return false
	}
	width := binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff
	height := binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff
	return width > 0 && height > 0
}

// validVP8L は VP8L の可逆の画像のチャンクが、署名と版 0 の見出しを持つかを返す。
func validVP8L(payload []byte) bool {
	return len(payload) >= 5 && payload[0] == 0x2f && payload[4]>>5 == 0
}
