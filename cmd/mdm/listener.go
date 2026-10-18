package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
)

// Reopening replaces the listener on the same http.Server, preserving established connections such
// as SSE. Close the old listener before binding the same port to loopback or wildcard.
type reopenableListener struct {
	mu sync.Mutex
	// Nil until Serve.
	srv *http.Server
	// Nil during reopening and after closure; ignore Serve failures from superseded listeners.
	current net.Listener
	// Actual bound address, including an assigned ephemeral port; used for rollback.
	addr   string
	closed bool
	// Reports the current listener's terminal result once.
	failed chan error
}

func newReopenableListener() *reopenableListener {
	return &reopenableListener{failed: make(chan error, 1)}
}

// Listen must be called once before Serve.
func (l *reopenableListener) Listen(addr string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.current != nil || l.closed {
		return errors.New("the listener is already open")
	}
	ln, err := listenTCP(addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	l.current, l.addr = ln, ln.Addr().String()
	return nil
}

// Serve starts accepting asynchronously and reports termination through Failed.
func (l *reopenableListener) Serve(srv *http.Server) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.srv = srv
	if l.current != nil {
		go l.serve(l.current)
	}
}

// Failed reports http.ErrServerClosed on server shutdown.
func (l *reopenableListener) Failed() <-chan error {
	return l.failed
}

func (l *reopenableListener) Addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.addr
}

// Reopen restores the old address on failure; if rollback also fails, report both errors and notify
// Failed.
func (l *reopenableListener) Reopen(addr string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil || l.closed {
		return errors.New("the listener is not serving")
	}

	previous := l.addr
	if l.current != nil {
		old := l.current
		// Clear current before closing so its Serve completion is ignored.
		l.current = nil
		_ = old.Close()
	}

	ln, err := listenTCP(addr)
	if err == nil {
		l.current, l.addr = ln, ln.Addr().String()
		go l.serve(ln)
		return nil
	}
	reopenErr := fmt.Errorf("cannot listen on %s: %w", addr, err)

	back, backErr := listenTCP(previous)
	if backErr != nil {
		lost := errors.Join(reopenErr, fmt.Errorf("cannot listen on %s again: %w", previous, backErr))
		l.notify(lost)
		return lost
	}
	l.current = back
	go l.serve(back)
	return reopenErr
}

// Use tcp4 for IPv4 addresses; "tcp" can bind 0.0.0.0 as dual-stack [::], violating the configured
// access scope.
func listenTCP(addr string) (net.Listener, error) {
	return net.Listen(listenNetwork(addr), addr)
}

func listenNetwork(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "tcp"
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.Is4() {
		return "tcp4"
	}
	return "tcp"
}

// Close prevents reopening but leaves existing connections to http.Server.Shutdown.
func (l *reopenableListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.current == nil {
		return nil
	}
	ln := l.current
	l.current = nil
	return ln.Close()
}

func (l *reopenableListener) serve(ln net.Listener) {
	err := l.srv.Serve(ln)
	l.mu.Lock()
	defer l.mu.Unlock()
	if ln != l.current {
		return
	}
	l.notify(err)
}

// Keep the first terminal result.
func (l *reopenableListener) notify(err error) {
	select {
	case l.failed <- err:
	default:
	}
}
