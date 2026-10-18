package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Compute the expected hash formula independently rather than calling the implementation under
// test.
func wantKey(t *testing.T, content []byte) string {
	t.Helper()

	size := int64(len(content))
	head := content
	if size > readWindow {
		head = content[:readWindow]
	}
	tail := content
	if size > readWindow {
		tail = content[size-readWindow:]
	}

	sum := sha256.New()
	sum.Write(head)
	sum.Write(tail)
	return hex.EncodeToString(sum.Sum(nil)) + ":" + fmt.Sprint(size)
}

func writeFile(t *testing.T, name string, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pattern(n int, seed byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i%251) ^ seed
	}
	return out
}

func TestContentKeyMatchesFormula(t *testing.T) {
	sizes := []struct {
		name string
		size int
	}{
		{"空", 0},
		{"1KiB", 1024},
		{"1MiB ちょうど", readWindow},
		{"1MiB + 1 バイト（先頭と末尾が重なる）", readWindow + 1},
		{"2MiB ちょうど（重ならない境界）", 2 * readWindow},
		{"2MiB + 1 バイト（間を読み飛ばす）", 2*readWindow + 1},
		{"5MiB", 5 * readWindow},
	}

	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			content := pattern(tc.size, 0x5a)
			path := writeFile(t, "a.mp4", content)

			got, err := ContentKey(path)
			if err != nil {
				t.Fatalf("鍵を計算できない: %v", err)
			}
			if want := wantKey(t, content); got != want {
				t.Errorf("ContentKey = %q, want %q", got, want)
			}
		})
	}
}

func TestContentKeyIgnoresPath(t *testing.T) {
	content := pattern(3*readWindow, 0x11)

	first := writeFile(t, "海辺の散歩.mp4", content)
	second := writeFile(t, "2026/別名.mp4", content)

	keyA, err := ContentKey(first)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := ContentKey(second)
	if err != nil {
		t.Fatal(err)
	}
	if keyA != keyB {
		t.Errorf("パスが違うだけで鍵が変わった:\n%s\n%s", keyA, keyB)
	}
}

// Tail differences matter because many containers store their index at the end.
func TestContentKeyDetectsTailDifference(t *testing.T) {
	base := pattern(4*readWindow, 0x22)

	changed := make([]byte, len(base))
	copy(changed, base)
	changed[len(changed)-1] ^= 0xff

	keyA, err := ContentKey(writeFile(t, "a.mp4", base))
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := ContentKey(writeFile(t, "b.mp4", changed))
	if err != nil {
		t.Fatal(err)
	}
	if keyA == keyB {
		t.Error("末尾 1 バイトの違いで鍵が変わらなかった")
	}
}

func TestContentKeyDetectsHeadDifference(t *testing.T) {
	base := pattern(4*readWindow, 0x33)

	changed := make([]byte, len(base))
	copy(changed, base)
	changed[0] ^= 0xff

	keyA, err := ContentKey(writeFile(t, "a.mp4", base))
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := ContentKey(writeFile(t, "b.mp4", changed))
	if err != nil {
		t.Fatal(err)
	}
	if keyA == keyB {
		t.Error("先頭 1 バイトの違いで鍵が変わらなかった")
	}
}

func TestContentKeyIncludesSize(t *testing.T) {
	short := pattern(1024, 0x44)
	long := append(append([]byte{}, short...), short...)

	keyA, err := ContentKey(writeFile(t, "a.mp4", short))
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := ContentKey(writeFile(t, "b.mp4", long))
	if err != nil {
		t.Fatal(err)
	}
	if keyA == keyB {
		t.Error("大きさが違うのに鍵が同じになった")
	}
}

func TestContentKeyReadsSmallFileOnce(t *testing.T) {
	content := pattern(readWindow+512, 0x55)
	counter := &countingReaderAt{data: content}

	got, err := ContentKeyFrom(counter, int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if want := wantKey(t, content); got != want {
		t.Errorf("ContentKeyFrom = %q, want %q", got, want)
	}
	if counter.calls != 1 {
		t.Errorf("読み出し回数 = %d, want 1（2MiB 以下は全体を1度だけ読む）", counter.calls)
	}
}

func TestContentKeyReadsLargeFileTwice(t *testing.T) {
	content := pattern(8*readWindow, 0x66)
	counter := &countingReaderAt{data: content}

	got, err := ContentKeyFrom(counter, int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if want := wantKey(t, content); got != want {
		t.Errorf("ContentKeyFrom = %q, want %q", got, want)
	}
	if counter.calls != 2 {
		t.Errorf("読み出し回数 = %d, want 2（先頭と末尾だけ）", counter.calls)
	}
	if counter.bytes != 2*readWindow {
		t.Errorf("読み出した量 = %d バイト, want %d（数 TB を読み直さない）",
			counter.bytes, 2*readWindow)
	}
}

type countingReaderAt struct {
	data  []byte
	calls int
	bytes int
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.calls++
	n := copy(p, c.data[off:])
	c.bytes += n
	if n < len(p) {
		return n, fmt.Errorf("末尾を越えて読もうとした: off=%d len=%d", off, len(p))
	}
	return n, nil
}
