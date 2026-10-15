package wasmsection

import (
	"bytes"
	"testing"
)

func TestCustom(t *testing.T) {
	got := Custom("vv.plugin", []byte("{}"))
	// id 0、中身の長さ 12、名前の長さ 9、名前、中身。
	want := append([]byte{0, 12, 9}, "vv.plugin{}"...)
	if !bytes.Equal(got, want) {
		t.Errorf("Custom = %v; want %v", got, want)
	}
}

func TestCustomLongPayload(t *testing.T) {
	got := Custom("a", make([]byte, 200))
	// 中身は 1+1+200 = 202 バイトで、LEB128 では 0xca 0x01 の 2 バイトになる。
	if !bytes.Equal(got[:4], []byte{0, 0xca, 0x01, 1}) || len(got) != 3+202 {
		t.Errorf("Custom header = %v, length %d", got[:4], len(got))
	}
}
