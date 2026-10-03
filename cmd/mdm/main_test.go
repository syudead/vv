package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServeDoesNotRunStartupHookWhenPortIsInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	called := false
	err = serveUntil(
		notifySignals,
		newReopenableListener(),
		Config{Addr: listener.Addr().String()},
		http.NotFoundHandler(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func() { called = true },
		nil,
	)
	if err == nil {
		t.Fatal("使用中のポートなのに起動できた")
	}
	if !strings.Contains(err.Error(), "cannot listen on") {
		t.Fatalf("err = %v", err)
	}
	// デスクトップ版は、この段階でポートの案内のダイアログを選ぶ。
	if stage := startupStageOf(err); stage != stageListen {
		t.Fatalf("startupStageOf = %v; want stageListen", stage)
	}
	if called {
		t.Fatal("待ち受けに失敗したのに起動時フックが呼ばれた")
	}
}

func TestServeCancelsTranscodesBeforeShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	order := make(chan string, 2)

	err := serveUntil(
		func() (<-chan struct{}, func()) { return ctx.Done(), nil },
		newReopenableListener(),
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

// 停止の指示で戻るとき、受け取りをやめる関数は 1 度だけ呼ばれる。
func TestServeReleasesStopNotifierOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var released atomic.Int32

	err := serveUntil(
		func() (<-chan struct{}, func()) {
			return ctx.Done(), func() { released.Add(1) }
		},
		newReopenableListener(),
		Config{Addr: "127.0.0.1:0"},
		http.NotFoundHandler(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func() { cancel() },
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := released.Load(); got != 1 {
		t.Fatalf("受け取りをやめる関数の呼び出し = %d 回, want 1", got)
	}
}

// 要求に応答したあとの停止の指示でも、正常に止まる。応答したなら Serve が
// 待ち受けを http.Server に登録しているので、先に閉じた待ち受けの登録を Serve が
// 外す前に Shutdown が走ると、Shutdown がもう一度閉じる。その順になるかは
// 時の運なので、何度も繰り返す。
func TestServeStopsCleanlyAfterServingARequest(t *testing.T) {
	for i := range 200 {
		if err := serveOneRequestAndStop(t); err != nil {
			t.Fatalf("%d 回目: 正常な停止なのに誤りが返った: %v", i+1, err)
		}
	}
}

// serveOneRequestAndStop は serveUntil を起動し、要求に 1 度応答してから
// 停止を指示し、serveUntil の戻り値を返す。
func serveOneRequestAndStop(t *testing.T) error {
	t.Helper()
	stop := make(chan struct{})
	listener := newReopenableListener()
	listening := make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		returned <- serveUntil(
			func() (<-chan struct{}, func()) { return stop, nil },
			listener,
			Config{Addr: "127.0.0.1:0"},
			http.NotFoundHandler(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			func() { close(listening) },
			nil,
		)
	}()
	<-listening

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://" + listener.Addr() + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	close(stop)
	return <-returned
}

// 待ち受けを失って戻るときも、処理中の要求を待ってから戻り、待ち受けを失った
// 理由を返す。戻ったあとに呼び出し元はデータベースを閉じる。
func TestServeWaitsForInFlightRequestsWhenListenerIsLost(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-finish
		_, _ = io.WriteString(w, "ok")
	})

	listener := newReopenableListener()
	listening := make(chan struct{})
	var beforeShutdownCalled atomic.Bool
	returned := make(chan error, 1)
	go func() {
		returned <- serveUntil(
			func() (<-chan struct{}, func()) { return nil, nil },
			listener,
			Config{Addr: "127.0.0.1:0"},
			handler,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			func() { close(listening) },
			func() { beforeShutdownCalled.Store(true) },
		)
	}()
	<-listening

	responded := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + listener.Addr() + "/")
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr == nil && string(body) != "ok" {
				readErr = errors.New("body = " + string(body))
			}
			err = readErr
		}
		responded <- err
	}()
	<-started

	// 開き直しで元のアドレスも開けなかったときと同じく、待ち受けの終わりを知らせる。
	cause := errors.New("cannot listen on the previous address again")
	listener.notify(cause)

	select {
	case err := <-returned:
		t.Fatalf("処理中の要求を待たずに戻った: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(finish)
	if err := <-responded; err != nil {
		t.Fatalf("処理中の要求が終わらなかった: %v", err)
	}
	err := <-returned
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want 待ち受けを失った理由", err)
	}
	if !beforeShutdownCalled.Load() {
		t.Fatal("待ち受けを失ったのに停止の前処理が呼ばれなかった")
	}
}

func TestStartupStageOfKeepsTheMessageAndStage(t *testing.T) {
	cause := errors.New("database is locked")
	err := fmt.Errorf("startup: %w", &startupError{stage: stageDatabase, err: cause})
	if stage := startupStageOf(err); stage != stageDatabase {
		t.Fatalf("startupStageOf = %v; want stageDatabase", stage)
	}
	if !errors.Is(err, cause) || err.Error() != "startup: database is locked" {
		t.Fatalf("err = %v", err)
	}
	if stage := startupStageOf(cause); stage != stageOther {
		t.Fatalf("startupStageOf(plain) = %v; want stageOther", stage)
	}
}
