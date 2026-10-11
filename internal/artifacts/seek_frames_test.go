package artifacts

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// numberedSheetWriter は 1 コマ 32 × 18 の 10 × 10 のシートを sheets 枚書く。コマ k は
// 明るさ k の一色で塗るので、切り出したコマの明るさからどのコマかが分かる。
func numberedSheetWriter(t *testing.T, sheets int) func(string) error {
	t.Helper()
	return func(dir string) error {
		for s := range sheets {
			img := image.NewGray(image.Rect(0, 0, 320, 180))
			for position := range 100 {
				x, y := (position%10)*32, (position/10)*18
				shade := color.Gray{Y: uint8(s*100 + position)}
				for py := y; py < y+18; py++ {
					for px := x; px < x+32; px++ {
						img.SetGray(px, py, shade)
					}
				}
			}
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.jpg", s)), buf.Bytes(), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
}

// 2 シートにまたがるコマから等間隔に選び、1 コマずつ切り出す。
func TestSeekFramesSpreadAcrossSheets(t *testing.T) {
	store := New(t.TempDir())
	if err := publishSeek(store, key, spriteLayout, numberedSheetWriter(t, 2)); err != nil {
		t.Fatal(err)
	}
	frames, err := store.SeekFrames(key, 3)
	if err != nil {
		t.Fatal(err)
	}
	// 150 コマから 3 つ: 25, 75, 125（2 枚目のシートの 25 番目）。
	want := []int{25, 75, 125}
	if len(frames) != len(want) {
		t.Fatalf("コマの数 = %d", len(frames))
	}
	for i, data := range frames {
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 18 {
			t.Fatalf("コマ %d の大きさ = %v", i, b)
		}
		gray := color.GrayModel.Convert(img.At(16, 9)).(color.Gray)
		if diff := int(gray.Y) - want[i]; diff < -3 || diff > 3 {
			t.Errorf("コマ %d の明るさ = %d, want %d", i, gray.Y, want[i])
		}
	}
}

func TestSeekFramesWithoutSprite(t *testing.T) {
	if _, err := New(t.TempDir()).SeekFrames(key, 8); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
