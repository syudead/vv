package main

import (
	"bytes"
	"sync"
	"testing"
)

func TestPrefixWriterLabelsCompleteLines(t *testing.T) {
	var out bytes.Buffer
	var lock sync.Mutex
	writer := &prefixWriter{prefix: "[go] ", out: &out, mu: &lock}

	// 出力は改行の途中で届く。行が揃うまで出さないと、片方のサーバーの行の
	// 途中にもう片方の行が割り込んで読めなくなる。
	if _, err := writer.Write([]byte("listening on ")); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("行が揃う前に出力した: %q", out.String())
	}

	if _, err := writer.Write([]byte(":8080\nready\n")); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "[go] listening on :8080\n[go] ready\n"; got != want {
		t.Errorf("出所を付けた行になっていない:\n got %q\nwant %q", got, want)
	}
}

func TestPrefixWriterFlushesTrailingLine(t *testing.T) {
	var out bytes.Buffer
	var lock sync.Mutex
	writer := &prefixWriter{prefix: "[go] ", out: &out, mu: &lock}

	// 落ちる直前の一行は改行を伴わないことがある。捨てると、こちらが出す
	// 「上の出力を確認すること」に対応する出力が残らない。
	if _, err := writer.Write([]byte("panic: nil map")); err != nil {
		t.Fatal(err)
	}
	writer.flush()
	if got, want := out.String(), "[go] panic: nil map\n"; got != want {
		t.Errorf("改行で終わらない最後の行を出していない:\n got %q\nwant %q", got, want)
	}

	// 出し切ったあとの flush は何も足さない。
	writer.flush()
	if out.String() != "[go] panic: nil map\n" {
		t.Errorf("二重に出力した: %q", out.String())
	}
}
