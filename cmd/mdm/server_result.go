package main

// Broadcast run completion by closing done so concurrent desktop shutdown handlers cannot consume
// each other's notification.
type serverResult struct {
	done chan struct{}
	err  error
}

func startServer(fn func() error) *serverResult {
	r := &serverResult{done: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.err = fn()
	}()
	return r
}

// Done closes when fn returns.
func (r *serverResult) Done() <-chan struct{} { return r.done }

// Read Err only after Done closes.
func (r *serverResult) Err() error {
	<-r.done
	return r.err
}
