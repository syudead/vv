package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"

	"github.com/syudead/vv/internal/domain"
)

// 映像の指紋（specs/030-video-versions/research.md R-6、data-model.md §6）。完成した
// シーク用スプライトのシートから作るので、ffmpeg を起動せず、元の動画も読み直さない。

// fingerprintBlackLevel は、平均の輝度がこれ未満の端の行と列を黒い帯として落とす値である。
const fingerprintBlackLevel = 16

// ErrSpriteMismatch はシートの枚数や大きさがスプライトの配置情報と合わないことを表す。
var ErrSpriteMismatch = errors.New("the seek sprite sheets do not match its layout")

// SpriteFingerprint はシーク用スプライトの各コマから映像の指紋を作る。sheets は
// シート 0 から順の JPEG である。各コマを切り出し、上下左右の黒い帯を落として 32×32 の
// 輝度に縮め、domain.HashFrame を並べる。
func SpriteFingerprint(sprite domain.SeekSprite, sheets [][]byte) (domain.Fingerprint, error) {
	perSheet := sprite.Columns * sprite.Rows
	if sprite.IntervalMs <= 0 || sprite.FrameCount <= 0 || perSheet <= 0 ||
		sprite.FrameWidth <= 0 || sprite.FrameHeight <= 0 {
		return domain.Fingerprint{}, fmt.Errorf("%w: %+v", ErrSpriteMismatch, sprite)
	}
	needed := (sprite.FrameCount + perSheet - 1) / perSheet
	if len(sheets) < needed {
		return domain.Fingerprint{}, fmt.Errorf("%w: %d sheets for %d frames", ErrSpriteMismatch, len(sheets), sprite.FrameCount)
	}
	frames := make([]domain.FrameHash, 0, sprite.FrameCount)
	for sheetIndex := range needed {
		sheet, err := jpeg.Decode(bytes.NewReader(sheets[sheetIndex]))
		if err != nil {
			return domain.Fingerprint{}, fmt.Errorf("cannot decode seek sprite sheet %d: %w", sheetIndex, err)
		}
		luma := lumaPlane(sheet)
		for slot := range perSheet {
			frame := sheetIndex*perSheet + slot
			if frame >= sprite.FrameCount {
				break
			}
			origin := luma.rect.Min.Add(image.Pt(
				(slot%sprite.Columns)*sprite.FrameWidth, (slot/sprite.Columns)*sprite.FrameHeight))
			rect := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(sprite.FrameWidth, sprite.FrameHeight))}
			if !rect.In(luma.rect) {
				return domain.Fingerprint{}, fmt.Errorf("%w: frame %d is outside sheet %d", ErrSpriteMismatch, frame, sheetIndex)
			}
			frames = append(frames, domain.HashFrame(luma.shrink(luma.trimBlackBands(rect))))
		}
	}
	return domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: sprite.IntervalMs, Frames: frames}, nil
}

// luma は 1 枚のシートの輝度（0〜255）である。
type luma struct {
	rect   image.Rectangle
	stride int
	pix    []uint8
}

// lumaPlane はシートの輝度を取り出す。スプライトの JPEG（yuvj420p）は *image.YCbCr で、
// その Y をそのまま使う。
func lumaPlane(img image.Image) luma {
	bounds := img.Bounds()
	switch typed := img.(type) {
	case *image.YCbCr:
		return luma{rect: bounds, stride: typed.YStride, pix: typed.Y[typed.YOffset(bounds.Min.X, bounds.Min.Y):]}
	case *image.Gray:
		return luma{rect: bounds, stride: typed.Stride, pix: typed.Pix[typed.PixOffset(bounds.Min.X, bounds.Min.Y):]}
	}
	out := luma{rect: bounds, stride: bounds.Dx(), pix: make([]uint8, bounds.Dx()*bounds.Dy())}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray, _ := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			out.pix[(y-bounds.Min.Y)*out.stride+(x-bounds.Min.X)] = gray.Y
		}
	}
	return out
}

func (l luma) at(x, y int) uint8 {
	return l.pix[(y-l.rect.Min.Y)*l.stride+(x-l.rect.Min.X)]
}

// rowMean と columnMean は rect の中の 1 行・1 列の平均の輝度である。
func (l luma) rowMean(rect image.Rectangle, y int) int {
	total := 0
	for x := rect.Min.X; x < rect.Max.X; x++ {
		total += int(l.at(x, y))
	}
	return total / rect.Dx()
}

func (l luma) columnMean(rect image.Rectangle, x int) int {
	total := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		total += int(l.at(x, y))
	}
	return total / rect.Dy()
}

// trimBlackBands は rect の上下左右から、平均の輝度が fingerprintBlackLevel 未満の行と列を
// 落とす。レターボックスやピラーボックスの有無が違う版どうしで同じ場面を比べるためである。
// 全体が暗いコマは落とさずにそのまま返す（単色の印が付いて比較から外れる）。
func (l luma) trimBlackBands(rect image.Rectangle) image.Rectangle {
	trimmed := rect
	for trimmed.Dy() > 0 && l.rowMean(trimmed, trimmed.Min.Y) < fingerprintBlackLevel {
		trimmed.Min.Y++
	}
	for trimmed.Dy() > 0 && l.rowMean(trimmed, trimmed.Max.Y-1) < fingerprintBlackLevel {
		trimmed.Max.Y--
	}
	if trimmed.Dy() == 0 {
		return rect
	}
	for trimmed.Dx() > 0 && l.columnMean(trimmed, trimmed.Min.X) < fingerprintBlackLevel {
		trimmed.Min.X++
	}
	for trimmed.Dx() > 0 && l.columnMean(trimmed, trimmed.Max.X-1) < fingerprintBlackLevel {
		trimmed.Max.X--
	}
	if trimmed.Dx() == 0 {
		return rect
	}
	return trimmed
}

// shrink は rect を面積の平均で 32×32 の輝度に縮める。各升目は rect を 32 等分した区間の
// 画素の平均で、区間が 1 画素に満たなければその位置の 1 画素を使う。
func (l luma) shrink(rect image.Rectangle) [domain.FingerprintFrameSize][domain.FingerprintFrameSize]uint8 {
	const size = domain.FingerprintFrameSize
	var out [size][size]uint8
	for cy := range size {
		y0, y1 := span(rect.Min.Y, rect.Dy(), cy)
		for cx := range size {
			x0, x1 := span(rect.Min.X, rect.Dx(), cx)
			total, count := 0, 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					total += int(l.at(x, y))
					count++
				}
			}
			out[cy][cx] = uint8((total + count/2) / count)
		}
	}
	return out
}

// span は長さ length を 32 等分した index 番目の区間 [start, end) を返す。
func span(origin, length, index int) (int, int) {
	start := origin + index*length/domain.FingerprintFrameSize
	end := origin + (index+1)*length/domain.FingerprintFrameSize
	if end <= start {
		end = start + 1
	}
	return start, end
}
