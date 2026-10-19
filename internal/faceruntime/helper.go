package faceruntime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

var (
	// ErrHelperStart reports a helper that did not start or did not greet as expected.
	ErrHelperStart = errors.New("the face helper did not start")
	// ErrHelperExited reports a helper whose stream broke during a request; the next request starts a new one.
	ErrHelperExited = errors.New("the face helper exited")
)

// HelperCommand is the program and arguments that start the helper.
type HelperCommand struct {
	Path string
	Args []string
}

// Process is a started helper: its standard input and output, and Stop, which ends it and waits for it.
// Stop must unblock a read of Out that is in progress, and may be called more than once.
type Process struct {
	In   io.WriteCloser
	Out  io.Reader
	Stop func() error
}

// Launcher starts a helper process.
type Launcher func(cmd HelperCommand) (Process, error)

// stopGrace is how long a helper may take to exit after its input closes before it is killed.
const stopGrace = 2 * time.Second

// handshakeTimeout bounds how long a started helper may take to greet. Loading the largest model on a slow
// machine takes seconds, so a helper still silent after this is treated as hung.
const handshakeTimeout = 60 * time.Second

// stderrLimit bounds the helper's standard error kept for an error message.
const stderrLimit = 4 << 10

// ExecLauncher starts the helper with command, which applies the OS's child-process settings
// (CREATE_NO_WINDOW on Windows). A helper outlives the request that started it; Stop or the end of ctx ends it.
func ExecLauncher(ctx context.Context, command func(ctx context.Context, name string, args ...string) *exec.Cmd) Launcher {
	return func(hc HelperCommand) (Process, error) {
		cmd := command(ctx, hc.Path, hc.Args...)
		stderr := &tailBuffer{limit: stderrLimit}
		cmd.Stderr = stderr
		in, err := cmd.StdinPipe()
		if err != nil {
			return Process{}, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			return Process{}, err
		}
		if err := cmd.Start(); err != nil {
			return Process{}, err
		}
		stop := sync.OnceValue(func() error { return stopCommand(cmd, in, stderr) })
		return Process{In: in, Out: out, Stop: stop}, nil
	}
}

func stopCommand(cmd *exec.Cmd, in io.Closer, stderr *tailBuffer) error {
	_ = in.Close()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	var err error
	select {
	case err = <-exited:
	case <-time.After(stopGrace):
		_ = cmd.Process.Kill()
		err = <-exited
	}
	if err != nil && stderr.Len() > 0 {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return err
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	bytes.Buffer
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n, _ := t.Buffer.Write(p)
	if extra := t.Len() - t.limit; extra > 0 {
		t.Next(extra)
	}
	return n, nil
}

// Helper is a client of one recognizer's helper. It starts the helper on first use and again after the helper
// exited, and serves one request at a time.
type Helper struct {
	command    HelperCommand
	launch     Launcher
	recognizer domain.FaceRecognizer
	// verify, when set, checks the helper's files before each launch.
	verify func() error
	// startFailed is told about every failed start, so a stale runtime marks the download state failed.
	startFailed func(error)

	mu   sync.Mutex
	proc *helperProcess
}

type helperProcess struct {
	Process
	out *bufio.Reader
}

// Start starts the helper and reads its Hello unless it is running.
func (h *Helper) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.running(ctx)
	return err
}

// Detect returns every face of one packed BGR frame with its vector.
func (h *Helper) Detect(ctx context.Context, req faceproto.DetectRequest) (faceproto.Detections, error) {
	reply, err := h.roundTrip(ctx, req)
	if err != nil {
		return faceproto.Detections{}, err
	}
	d, ok := reply.(faceproto.Detections)
	if !ok {
		return faceproto.Detections{}, fmt.Errorf("the face helper answered a detection with %T", reply)
	}
	return d, nil
}

// Embed returns the vector of one aligned face.
func (h *Helper) Embed(ctx context.Context, req faceproto.EmbedRequest) (faceproto.Vector, error) {
	reply, err := h.roundTrip(ctx, req)
	if err != nil {
		return faceproto.Vector{}, err
	}
	v, ok := reply.(faceproto.Vector)
	if !ok {
		return faceproto.Vector{}, fmt.Errorf("the face helper answered an embedding with %T", reply)
	}
	return v, nil
}

// Close stops the helper; a later request starts it again.
func (h *Helper) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stop()
}

func (h *Helper) stop() error {
	if h.proc == nil {
		return nil
	}
	err := h.proc.Stop()
	h.proc = nil
	return err
}

// roundTrip sends req and reads its reply. An Error reply fails only this request; a broken stream or a
// cancelled ctx stops the helper.
func (h *Helper) roundTrip(ctx context.Context, req faceproto.Message) (faceproto.Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, err := h.running(ctx)
	if err != nil {
		return nil, err
	}
	reply, err := p.call(ctx, func() (faceproto.Message, error) {
		if err := faceproto.Write(p.In, req); err != nil {
			return nil, err
		}
		return faceproto.Read(p.out)
	})
	if err != nil {
		_ = h.stop()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", ErrHelperExited, err)
	}
	if e, ok := reply.(faceproto.Error); ok {
		return nil, fmt.Errorf("the face helper failed: %s", e.Message)
	}
	return reply, nil
}

func (h *Helper) running(ctx context.Context) (*helperProcess, error) {
	if h.proc != nil {
		return h.proc, nil
	}
	p, err := h.start(ctx)
	if err != nil {
		if ctx.Err() == nil && h.startFailed != nil {
			h.startFailed(err)
		}
		return nil, err
	}
	h.proc = p
	return p, nil
}

func (h *Helper) start(ctx context.Context) (*helperProcess, error) {
	if h.verify != nil {
		if err := h.verify(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrHelperStart, err)
		}
	}
	proc, err := h.launch(h.command)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHelperStart, err)
	}
	p := &helperProcess{Process: proc, out: bufio.NewReaderSize(proc.Out, 1<<16)}
	handshake, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	hello, err := p.call(handshake, func() (faceproto.Message, error) { return faceproto.Read(p.out) })
	if err == nil {
		err = h.checkHello(hello)
	}
	if err != nil {
		stopErr := p.Stop()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", ErrHelperStart, errors.Join(err, stopErr))
	}
	return p, nil
}

func (h *Helper) checkHello(m faceproto.Message) error {
	switch hello := m.(type) {
	case faceproto.Hello:
		if hello.Recognizer != string(h.recognizer) {
			return fmt.Errorf("the helper loaded %q, want %q", hello.Recognizer, h.recognizer)
		}
		return nil
	case faceproto.Error:
		return errors.New(hello.Message)
	}
	return fmt.Errorf("the helper greeted with %T", m)
}

// call runs exchange, stopping the process when ctx ends first so the blocked read returns.
func (p *helperProcess) call(ctx context.Context, exchange func() (faceproto.Message, error)) (faceproto.Message, error) {
	type result struct {
		m   faceproto.Message
		err error
	}
	done := make(chan result, 1)
	go func() {
		m, err := exchange()
		done <- result{m, err}
	}()
	select {
	case r := <-done:
		return r.m, r.err
	case <-ctx.Done():
		_ = p.Stop()
		<-done
		return nil, ctx.Err()
	}
}
