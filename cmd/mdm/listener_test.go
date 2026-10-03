package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ownNonLoopbackIPv4 はこの機械の、ループバックでない IPv4 アドレスを 1 つ返す。
// 無ければ試験を飛ばす。
func ownNonLoopbackIPv4(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			return ip.String()
		}
	}
	t.Skip("ループバックでない IPv4 アドレスが無い")
	return ""
}

// freePort は今は使われていない TCP のポートを返す。
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// sseHandler は /events で最初の知らせを書き、next に値が来るたびに次の知らせを
// 書く。それ以外の経路は ok を返す。
func sseHandler(next <-chan string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			_, _ = io.WriteString(w, "ok")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: first\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case message := <-next:
				_, _ = fmt.Fprintf(w, "data: %s\n\n", message)
				flusher.Flush()
			}
		}
	})
}

// startReopenable は addr で待ち受けて handler で応答し、試験の終わりに閉じる。
func startReopenable(t *testing.T, addr string, handler http.Handler) *reopenableListener {
	t.Helper()
	listener := newReopenableListener()
	if err := listener.Listen(addr); err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: readHeaderTimeout}
	listener.Serve(srv)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = srv.Close()
	})
	return listener
}

// getOK は url に GET し、ok が返るかを確かめる。接続できなければその失敗を返す。
func getOK(url string) error {
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if string(body) != "ok" {
		return fmt.Errorf("body = %q", body)
	}
	return nil
}

// readEvent は SSE の次の data 行を読む。
func readEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("SSE の接続が切れた: %v", err)
		}
		if data, ok := strings.CutPrefix(strings.TrimSpace(line), "data: "); ok {
			return data
		}
	}
}

// ループバックから全てのアドレスへ開き直すと、ループバックでない自分のアドレスから
// 接続でき、戻すと拒まれる。そのあいだ、確立済みの SSE の接続は切れない
// （specs/037-windows-app/research.md R-14）。
func TestReopenableListenerSwitchesAddressAndKeepsConnections(t *testing.T) {
	lanIP := ownNonLoopbackIPv4(t)
	port := freePort(t)
	loopback := net.JoinHostPort("127.0.0.1", port)
	wildcard := net.JoinHostPort("0.0.0.0", port)
	lanURL := "http://" + net.JoinHostPort(lanIP, port) + "/"

	next := make(chan string)
	listener := startReopenable(t, loopback, sseHandler(next))

	if err := getOK("http://" + loopback + "/"); err != nil {
		t.Fatalf("ループバックから接続できない: %v", err)
	}
	if err := getOK(lanURL); err == nil {
		t.Fatal("ループバックだけの待ち受けに、ループバックでないアドレスから接続できた")
	}

	// 確立済みの SSE の接続。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+loopback+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	events := bufio.NewReader(resp.Body)
	if got := readEvent(t, events); got != "first" {
		t.Fatalf("最初の知らせ = %q", got)
	}

	if err := listener.Reopen(wildcard); err != nil {
		t.Fatalf("全てのアドレスへ開き直せない: %v", err)
	}
	if err := getOK(lanURL); err != nil {
		t.Fatalf("開き直したあと、ループバックでないアドレスから接続できない: %v", err)
	}
	next <- "after-open"
	if got := readEvent(t, events); got != "after-open" {
		t.Fatalf("開き直したあとの知らせ = %q", got)
	}

	if err := listener.Reopen(loopback); err != nil {
		t.Fatalf("ループバックへ戻せない: %v", err)
	}
	err = getOK(lanURL)
	if err == nil {
		t.Fatal("ループバックへ戻したのに、ループバックでないアドレスから接続できた")
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("戻したあとの接続の失敗 = %v, want 拒否", err)
	}
	if err := getOK("http://" + loopback + "/"); err != nil {
		t.Fatalf("戻したあと、ループバックから接続できない: %v", err)
	}
	next <- "after-close"
	if got := readEvent(t, events); got != "after-close" {
		t.Fatalf("戻したあとの知らせ = %q", got)
	}

	select {
	case err := <-listener.Failed():
		t.Fatalf("開き直しで待ち受けの終わりが知らされた: %v", err)
	default:
	}
}

// 開き直しに失敗したら、元のアドレスで待ち受けが残る。
func TestReopenableListenerKeepsPreviousAddressWhenReopenFails(t *testing.T) {
	listener := startReopenable(t, "127.0.0.1:0", sseHandler(nil))
	previous := listener.Addr()

	// 他が使っているアドレスへは開き直せない。
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()

	err = listener.Reopen(busy.Addr().String())
	if err == nil {
		t.Fatal("使用中のアドレスへ開き直せた")
	}
	if !strings.Contains(err.Error(), "cannot listen on") {
		t.Fatalf("err = %v", err)
	}
	if got := listener.Addr(); got != previous {
		t.Fatalf("Addr() = %q, want 元の %q", got, previous)
	}
	if err := getOK("http://" + previous + "/"); err != nil {
		t.Fatalf("元のアドレスで接続できない: %v", err)
	}
	select {
	case err := <-listener.Failed():
		t.Fatalf("開き直しの失敗で待ち受けの終わりが知らされた: %v", err)
	default:
	}
}

// 停止のために閉じたあとは開き直さない。
func TestReopenableListenerRefusesReopenAfterClose(t *testing.T) {
	listener := startReopenable(t, "127.0.0.1:0", sseHandler(nil))
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := listener.Reopen("127.0.0.1:0"); err == nil {
		t.Fatal("閉じたあとに開き直せた")
	}
}
