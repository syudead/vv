package wasmplugin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/syudead/vv/internal/domain"
)

const memoryLimitBytes = domain.WorkInfoPluginMemoryLimitPages * 65536

// wazero checks context between compiled functions only with at least two workers; one worker would
// make compilation uncancelable.
const compileWorkers = 2

var (
	errTimedOut       = errors.New("work-info plugin run timed out")
	errOutputTooLarge = errors.New("work-info plugin output exceeded its limit")
	errMemoryExceeded = errors.New("work-info plugin memory exceeded its limit")
)

// Run serializes fresh instances and includes compilation in the execution deadline; contention
// waits at most WorkInfoPluginBusyWait. Return stderr on every outcome, but record it before
// releasing the slot only after a started module completes without caller cancellation; recording
// errors never alter the run result.
func (r *Runtime) Run(
	ctx context.Context, digest string, module []byte, req domain.WorkInfoRunRequest, network domain.WorkInfoRunNetwork,
	record func(domain.WorkInfoRunLog) error,
) (domain.WorkInfoRunResult, domain.WorkInfoRunLog, error) {
	input, err := json.Marshal(req)
	if err != nil {
		return domain.WorkInfoRunResult{}, domain.WorkInfoRunLog{}, fmt.Errorf("encode work-info plugin request: %w", err)
	}
	release, err := r.acquire(ctx)
	if err != nil {
		return domain.WorkInfoRunResult{}, domain.WorkInfoRunLog{}, err
	}
	defer release()
	deadline, cancel := context.WithTimeoutCause(ctx, domain.WorkInfoPluginRunTimeout, errTimedOut)
	defer cancel()
	session := r.run.net.session(network)
	defer session.close()
	output, stderr, started, err := r.run.run(ctx, deadline, digest, module, input, session)
	var result domain.WorkInfoRunResult
	if err == nil {
		result, err = domain.ParseRunResponse(req, output)
	}
	switch {
	case err != nil || req.Operation != domain.WorkInfoFetch:
	case req.Kind == domain.WorkInfoPluginKindPerson:
		result.PersonPluginImages = session.images(deadline, result.Person.ImageURLs)
	default:
		result.CoverImages = session.images(deadline, result.WorkInfo.CoverURLs)
		result.PersonImages = session.personImages(deadline, result.WorkInfo.People)
	}
	err = session.outcome(err)
	log := domain.WorkInfoRunLog{Operation: req.Operation, Stderr: stderr.buf.String(), Truncated: stderr.dropped}
	var runErr *domain.WorkInfoRunError
	if errors.As(err, &runErr) {
		log.Failure = runErr.Failure
	}
	if record != nil && started && (err == nil || runErr != nil) {
		if recordErr := record(log); recordErr != nil {
			r.run.logger.Warn("work-info plugin run log was not saved", "digest", digest, "error", recordErr)
		}
	}
	if err != nil {
		return domain.WorkInfoRunResult{}, log, err
	}
	return result, log, nil
}

// Caller cancellation wins; blocked HTTP destinations override plugin output, while unreachable
// requests become network_error only when the plugin fails.
func (s *httpSession) outcome(err error) error {
	var runErr *domain.WorkInfoRunError
	if err != nil && !errors.As(err, &runErr) {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.refused != "":
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunBlockedAddress, Host: s.refused}
	case runErr != nil && runErr.Failure == domain.WorkInfoRunFailed && s.unreachable != "":
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunNetworkError, Host: s.unreachable, PluginMessage: runErr.PluginMessage}
	}
	return err
}

func (r *Runtime) acquire(ctx context.Context) (func(), error) {
	timer := time.NewTimer(domain.WorkInfoPluginBusyWait)
	defer timer.Stop()
	select {
	case r.slot <- struct{}{}:
		return func() { <-r.slot }, nil
	case <-timer.C:
		return nil, &domain.WorkInfoRunError{Failure: domain.WorkInfoRunBusy}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// The Runtime execution slot serializes runner access, so no separate mutex is needed.
type runner struct {
	logger *slog.Logger
	rt     wazero.Runtime
	net    *netClient
	cache  map[string]wazero.CompiledModule
	order  []string
}

func newRunner(ctx context.Context, logger *slog.Logger, nc *netClient) (*runner, error) {
	// Leave wazero's page limit at 4 GiB so cappedMemory can observe and record excess growth; the
	// allocator enforces the actual WorkInfoPluginMemoryLimitPages bound.
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("instantiate WASI: %w", err)
	}
	if _, err := hostModule(rt).Instantiate(ctx); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("instantiate vv host module: %w", err)
	}
	return &runner{logger: logger, rt: rt, net: nc, cache: map[string]wazero.CompiledModule{}}, nil
}

func (rn *runner) close(ctx context.Context) error {
	rn.net.close()
	return rn.rt.Close(ctx)
}

// The execution deadline covers compilation and instantiation; retain stderr on failure, and
// started=false distinguishes failures before module startup.
func (rn *runner) run(
	parent, deadline context.Context, digest string, module, input []byte, session *httpSession,
) (output []byte, stderr *keptOutput, started bool, err error) {
	stderr = &keptOutput{limit: domain.WorkInfoPluginStderrKeptBytes}
	compiled, err := rn.compiled(deadline, digest, module)
	if err != nil {
		if stop := stopped(parent, deadline); stop != nil {
			return nil, stderr, false, stop
		}
		return nil, stderr, false, err
	}
	runCtx, cancel := context.WithCancelCause(context.WithValue(deadline, sessionKey{}, session))
	defer cancel(nil)

	memory := &cappedMemory{limit: memoryLimitBytes, exceed: func() { cancel(errMemoryExceeded) }}
	stdout := &limitedOutput{limit: domain.WorkInfoPluginOutputMaxBytes, exceed: func() { cancel(errOutputTooLarge) }}
	config := wazero.NewModuleConfig().
		WithName("").
		WithStdin(bytes.NewReader(input)).
		WithStdout(stdout).
		WithStderr(stderr).
		WithSysWalltime().
		WithSysNanotime().
		WithNanosleep(func(ns int64) { sleep(runCtx, time.Duration(ns)) }).
		WithRandSource(rand.Reader)
	mod, err := rn.rt.InstantiateModule(experimental.WithMemoryAllocator(runCtx, memory), compiled, config)
	if mod != nil {
		_ = mod.Close(parent)
	}
	rn.logStderr(digest, stderr)
	if failure := runFailure(parent, runCtx, err); failure != nil {
		if err != nil {
			rn.logger.Info("work-info plugin run failed", "digest", digest, "error", err)
		}
		return nil, stderr, true, failure
	}
	return stdout.buf.Bytes(), stderr, true, nil
}

// The first output/memory limit becomes the cancellation cause and fails execution even if the
// plugin ignores write errors or failed memory.grow.
func runFailure(parent, runCtx context.Context, err error) error {
	cause := context.Cause(runCtx)
	switch {
	case errors.Is(cause, errOutputTooLarge):
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunOutputTooLarge}
	case errors.Is(cause, errMemoryExceeded):
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunMemoryExceeded}
	case err == nil:
		return nil
	}
	if stop := stopped(parent, runCtx); stop != nil {
		return stop
	}
	return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed}
}

// Caller cancellation takes precedence over execution timeout.
func stopped(parent, runCtx context.Context) error {
	switch {
	case parent.Err() != nil:
		return parent.Err()
	case errors.Is(context.Cause(runCtx), errTimedOut):
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunTimedOut}
	default:
		return nil
	}
}

// Cache compiled modules by digest and close the oldest when capacity is exceeded.
func (rn *runner) compiled(ctx context.Context, digest string, module []byte) (wazero.CompiledModule, error) {
	if c, ok := rn.cache[digest]; ok {
		return c, nil
	}
	layout, err := readLayout(module)
	if err != nil {
		rn.logger.Info("work-info plugin is malformed", "digest", digest, "error", err)
		return nil, &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed}
	}
	c, err := rn.rt.CompileModule(experimental.WithCompilationWorkers(ctx, compileWorkers), capTables(module, layout, domain.WorkInfoPluginTableMaxElements))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		rn.logger.Info("work-info plugin did not compile", "digest", digest, "error", err)
		return nil, &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed}
	}
	// Oversized initial memory reaches this path only for modules bypassing inspection, which already
	// rejects it at compile time.
	if mem, ok := c.ExportedMemories()[memoryExport]; !ok || mem.Min() > domain.WorkInfoPluginMemoryLimitPages {
		_ = c.Close(ctx)
		return nil, &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed}
	}
	if len(rn.order) == compiledCacheSize {
		oldest := rn.order[0]
		rn.order = rn.order[1:]
		_ = rn.cache[oldest].Close(ctx)
		delete(rn.cache, oldest)
	}
	rn.cache[digest] = c
	rn.order = append(rn.order, digest)
	return c, nil
}

func (rn *runner) logStderr(digest string, stderr *keptOutput) {
	if stderr.buf.Len() == 0 {
		return
	}
	rn.logger.Info("work-info plugin stderr",
		"digest", digest, "stderr", stderr.buf.String(), "truncated", stderr.dropped)
}

// Interrupt plugin sleep on execution completion so sleeping cannot evade the deadline.
func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// Excess stdout stops execution and rejects later writes.
type limitedOutput struct {
	buf      bytes.Buffer
	limit    int
	exceeded bool
	exceed   func()
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if w.exceeded || w.buf.Len()+len(p) > w.limit {
		if !w.exceeded {
			w.exceeded = true
			w.exceed()
		}
		return 0, errOutputTooLarge
	}
	return w.buf.Write(p)
}

// Retain only the stderr prefix while reporting successful writes for discarded bytes.
type keptOutput struct {
	buf     bytes.Buffer
	limit   int
	dropped bool
}

func (w *keptOutput) Write(p []byte) (int, error) {
	room := w.limit - w.buf.Len()
	if len(p) > room {
		w.dropped = true
		w.buf.Write(p[:max(room, 0)])
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

// Excess memory growth returns -1 and stops execution; each wazero instance receives its own
// allocator.
type cappedMemory struct {
	limit    uint64
	buf      []byte
	exceeded bool
	exceed   func()
}

func (m *cappedMemory) Allocate(capacity, _ uint64) experimental.LinearMemory {
	m.buf = make([]byte, 0, min(capacity, m.limit))
	return m
}

func (m *cappedMemory) Reallocate(size uint64) []byte {
	if size > m.limit {
		if !m.exceeded {
			m.exceeded = true
			m.exceed()
		}
		return nil
	}
	if size > uint64(cap(m.buf)) {
		grown := make([]byte, size, min(max(size, 2*uint64(cap(m.buf))), m.limit))
		copy(grown, m.buf)
		m.buf = grown
		return m.buf
	}
	m.buf = m.buf[:size]
	return m.buf
}

func (m *cappedMemory) Free() {
	m.buf = nil
}
