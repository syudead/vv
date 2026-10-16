package artifacts

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"

	"github.com/syudead/vv/internal/domain"
)

// seekFrameQuality は切り出したコマを JPEG にするときの品質である。
const seekFrameQuality = 85

// SeekFrames はシーク用プレビューのシートから、動画全体に等間隔に n コマを選び
// （domain.SpreadFrameIndexes）、1 コマずつの JPEG にして時間の順に返す。置き場か配置情報が
// 無ければ fs.ErrNotExist を包んだ誤りを返す。自動タグ付けが判定に添える
// （docs/design-docs/auto-tagging.md）。
func (s *Store) SeekFrames(contentKey string, n int) ([][]byte, error) {
	reader, err := s.seekFrameReader(contentKey)
	if err != nil {
		return nil, err
	}
	var frames [][]byte
	for _, index := range domain.SpreadFrameIndexes(reader.sprite.FrameCount, n) {
		frame, err := reader.frame(index)
		if err != nil {
			return nil, err
		}
		data, err := encodeSeekFrame(frame, index)
		if err != nil {
			return nil, err
		}
		frames = append(frames, data)
	}
	return frames, nil
}

// SeekFramePairs は 2 つの内容のシーク用プレビューから、組ごとに a のコマ pair.A を左、b のコマ
// pair.B を右に並べた 1 枚の JPEG を、組の順に返す（specs/054-auto-dup/research.md R-2）。b のコマは
// a のコマの大きさに合わせるので、1 枚は a の 1 コマの 2 倍の幅になる。メディアファイルは読まず、
// ffmpeg も使わない。置き場か配置情報が無ければ fs.ErrNotExist を包んだ誤りを返す。
func (s *Store) SeekFramePairs(keyA, keyB string, pairs []domain.FramePair) ([][]byte, error) {
	a, err := s.seekFrameReader(keyA)
	if err != nil {
		return nil, err
	}
	b, err := s.seekFrameReader(keyB)
	if err != nil {
		return nil, err
	}
	images := make([][]byte, 0, len(pairs))
	for _, pair := range pairs {
		left, err := a.frame(pair.A)
		if err != nil {
			return nil, err
		}
		right, err := b.frame(pair.B)
		if err != nil {
			return nil, err
		}
		width, height := left.Bounds().Dx(), left.Bounds().Dy()
		canvas := image.NewRGBA(image.Rect(0, 0, 2*width, height))
		draw.Draw(canvas, image.Rect(0, 0, width, height), left, left.Bounds().Min, draw.Src)
		draw.ApproxBiLinear.Scale(canvas, image.Rect(width, 0, 2*width, height), right, right.Bounds(), draw.Src, nil)
		data, err := encodeSeekFrame(canvas, pair.A)
		if err != nil {
			return nil, err
		}
		images = append(images, data)
	}
	return images, nil
}

// seekFrames は 1 つの内容のシーク用プレビューのコマを読む。読んだシートは覚えて使い回す。
type seekFrames struct {
	store      *Store
	contentKey string
	sprite     domain.SeekSprite
	sheets     map[int]image.Image
}

// seekFrameReader は内容 contentKey のシーク用プレビューの配置情報を読み、コマを読む準備をする。
func (s *Store) seekFrameReader(contentKey string) (*seekFrames, error) {
	sprite, err := s.SeekSprite(contentKey)
	if err != nil {
		return nil, err
	}
	return &seekFrames{store: s, contentKey: contentKey, sprite: sprite, sheets: map[int]image.Image{}}, nil
}

// frame はコマ index（動画の先頭から 0 始まり）を切り出した画像を返す。
func (r *seekFrames) frame(index int) (image.Image, error) {
	if index < 0 || index >= r.sprite.FrameCount {
		return nil, fmt.Errorf("seek preview frame %d is outside the %d frames", index, r.sprite.FrameCount)
	}
	perSheet := r.sprite.Columns * r.sprite.Rows
	sheet := index / perSheet
	img, ok := r.sheets[sheet]
	if !ok {
		data, err := r.store.SeekSpriteSheet(r.contentKey, sheet)
		if err != nil {
			return nil, err
		}
		if img, err = jpeg.Decode(bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("cannot decode seek preview sheet %d: %w", sheet, err)
		}
		r.sheets[sheet] = img
	}
	return cropSeekFrame(img, r.sprite, index%perSheet)
}

// cropSeekFrame はシートの position 番目（左上から行優先）のコマを切り出す。
func cropSeekFrame(sheet image.Image, sprite domain.SeekSprite, position int) (image.Image, error) {
	x := (position % sprite.Columns) * sprite.FrameWidth
	y := (position / sprite.Columns) * sprite.FrameHeight
	rect := image.Rect(x, y, x+sprite.FrameWidth, y+sprite.FrameHeight).Add(sheet.Bounds().Min)
	if !rect.In(sheet.Bounds()) {
		return nil, fmt.Errorf("seek preview frame %d is outside the sheet", position)
	}
	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), sheet, rect.Min, draw.Src)
	return cropped, nil
}

// encodeSeekFrame は切り出したコマ（または並べた 1 枚）を JPEG にする。index は誤りの文に入れる番号。
func encodeSeekFrame(img image.Image, index int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: seekFrameQuality}); err != nil {
		return nil, fmt.Errorf("cannot encode seek preview frame %d: %w", index, err)
	}
	return buf.Bytes(), nil
}
