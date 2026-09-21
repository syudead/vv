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
