package main

import (
	"context"
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
		nil,
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

func TestServeCancelsTranscodesBeforeShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	order := make(chan string, 2)

	err := serveUntil(
		ctx.Done(),
		nil,
		Config{Addr: "127.0.0.1:0"},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func() { cancel() },
		func() { order <- "transcodes" },
	)
	if err != nil {
		t.Fatal(err)
	}
	order <- "shutdown-complete"
	if first := <-order; first != "transcodes" {
		t.Fatalf("最初の停止処理 = %q", first)
	}
}
