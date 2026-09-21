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

func TestAPITargetFor(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{name: "wildcard", address: ":18081", want: "http://localhost:18081"},
		{name: "IPv4 wildcard", address: "0.0.0.0:8080", want: "http://localhost:8080"},
		{name: "IPv6 wildcard", address: "[::]:8080", want: "http://localhost:8080"},
		{name: "explicit host", address: "127.0.0.1:9000", want: "http://127.0.0.1:9000"},
		{name: "IPv6 host", address: "[::1]:9000", want: "http://[::1]:9000"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := apiTargetFor(test.address, "")
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("apiTargetFor(%q) = %q, want %q", test.address, got, test.want)
			}
		})
	}
}

func TestAPITargetForRejectsInvalidAddress(t *testing.T) {
	if _, err := apiTargetFor("localhost", ""); err == nil {
		t.Fatal("ポートのないアドレスを受理しました")
	}
}

func TestAPITargetForPrefersConfiguredTarget(t *testing.T) {
	got, err := apiTargetFor(":18081", "https://api.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.example.test" {
		t.Fatalf("明示した MDM_API_TARGET が優先されていません: %q", got)
	}
}
