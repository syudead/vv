package main

// serverResult は別の goroutine で動く run の終わりを、待つ全員に知らせる
// （Windows デスクトップ版の desktop_windows.go が使う）。終わりは done を閉じて
// 知らせるので、何か所で待っても互いの知らせを奪わない。
type serverResult struct {
	done chan struct{}
	err  error
}

// startServer は fn を別の goroutine で動かし、その終わりを待てる serverResult を返す。
func startServer(fn func() error) *serverResult {
	r := &serverResult{done: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.err = fn()
	}()
	return r
}

// Done は fn が返ったら閉じる。
func (r *serverResult) Done() <-chan struct{} { return r.done }

// Err は fn が返した誤りである。Done が閉じてから読む。
func (r *serverResult) Err() error {
	<-r.done
	return r.err
}
