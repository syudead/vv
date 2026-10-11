package artifacts

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/syudead/vv/internal/domain"
)

// seekFrameQuality は切り出したコマを JPEG にするときの品質である。
const seekFrameQuality = 85

// SeekFrames はシーク用プレビューのシートから、動画全体に等間隔に n コマを選び
// （domain.SpreadFrameIndexes）、1 コマずつの JPEG にして時間の順に返す。置き場か配置情報が
// 無ければ fs.ErrNotExist を包んだ誤りを返す。自動タグ付けが判定に添える
// （docs/design-docs/auto-tagging.md）。
func (s *Store) SeekFrames(contentKey string, n int) ([][]byte, error) {
	sprite, err := s.SeekSprite(contentKey)
	if err != nil {
		return nil, err
	}
	perSheet := sprite.Columns * sprite.Rows
	sheets := map[int]image.Image{}
	var frames [][]byte
	for _, index := range domain.SpreadFrameIndexes(sprite.FrameCount, n) {
		sheet := index / perSheet
		img, ok := sheets[sheet]
		if !ok {
			data, err := s.SeekSpriteSheet(contentKey, sheet)
			if err != nil {
				return nil, err
			}
			if img, err = jpeg.Decode(bytes.NewReader(data)); err != nil {
				return nil, fmt.Errorf("cannot decode seek preview sheet %d: %w", sheet, err)
			}
			sheets[sheet] = img
		}
		frame, err := cropSeekFrame(img, sprite, index%perSheet)
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

// cropSeekFrame はシートの position 番目（左上から行優先）のコマを切り出して JPEG にする。
func cropSeekFrame(sheet image.Image, sprite domain.SeekSprite, position int) ([]byte, error) {
	x := (position % sprite.Columns) * sprite.FrameWidth
	y := (position / sprite.Columns) * sprite.FrameHeight
	rect := image.Rect(x, y, x+sprite.FrameWidth, y+sprite.FrameHeight).Add(sheet.Bounds().Min)
	if !rect.In(sheet.Bounds()) {
		return nil, fmt.Errorf("seek preview frame %d is outside the sheet", position)
	}
	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for py := 0; py < rect.Dy(); py++ {
		for px := 0; px < rect.Dx(); px++ {
			cropped.Set(px, py, sheet.At(rect.Min.X+px, rect.Min.Y+py))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: seekFrameQuality}); err != nil {
		return nil, fmt.Errorf("cannot encode seek preview frame %d: %w", position, err)
	}
	return buf.Bytes(), nil
}
