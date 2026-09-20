package main

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestServeDoesNotRunStartupHookWhenPortIsInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	called := false
	err = serve(
		Config{Addr: listener.Addr().String()},
		http.NotFoundHandler(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func() { called = true },
	)
	if err == nil {
		t.Fatal("使用中のポートなのに起動できた")
	}
	if !strings.Contains(err.Error(), "待ち受けに失敗しました") {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("待ち受けに失敗したのに起動時フックが呼ばれた")
	}
}
