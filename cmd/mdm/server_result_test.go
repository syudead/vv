package main

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// 閉じる操作の goroutine と WebView2 の失敗の処理が同時に待っても、どちらも
// 終わりを受け取れる（片方が知らせを奪って、もう片方が待ち続けない）。
func TestServerResultNotifiesEveryWaiter(t *testing.T) {
	stop := make(chan struct{})
	want := errors.New("stopped")
	result := startServer(func() error {
		<-stop
		return want
	})

	const waiters = 3
	var wg sync.WaitGroup
	got := make(chan error, waiters)
	for range waiters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-result.Done()
			got <- result.Err()
		}()
	}
	close(stop)

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a waiter did not see the server stop")
	}
	close(got)
	for err := range got {
		if !errors.Is(err, want) {
			t.Fatalf("Err() = %v; want %v", err, want)
		}
	}
}
