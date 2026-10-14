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

// memoryLimitBytes は 1 回の実行の線形メモリの上限である。
const memoryLimitBytes = domain.WorkInfoPluginMemoryLimitPages * 65536

var (
	errTimedOut       = errors.New("work-info plugin run timed out")
	errOutputTooLarge = errors.New("work-info plugin output exceeded its limit")
)

// Run はモジュールを新しいインスタンスで 1 回動かす。要求の JSON を標準入力に書いて閉じ、
// 標準出力の応答を読む。同時に動く実行は 1 本で、2 本目は枠を最大 domain.WorkInfoPluginBusyWait 待つ
// （research.md R-2・R-4）。失敗は *domain.WorkInfoRunError で返し、呼び出し元の ctx が
// 終わったときはその誤りを返す。
func (r *Runtime) Run(ctx context.Context, digest string, module []byte, req domain.WorkInfoRunRequest) (domain.WorkInfoRunResult, error) {
	input, err := json.Marshal(req)
	if err != nil {
		return domain.WorkInfoRunResult{}, fmt.Errorf("encode work-info plugin request: %w", err)
	}
	release, err := r.acquire(ctx)
	if err != nil {
		return domain.WorkInfoRunResult{}, err
	}
	defer release()
	output, err := r.run.run(ctx, digest, module, input)
	if err != nil {
		return domain.WorkInfoRunResult{}, err
	}
	return domain.ParseWorkInfoResponse(req.Operation, output)
}

// acquire は実行の枠を取り、返す関数で空ける。
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

// runner は実行の wazero のランタイムと、コンパイル済みのモジュールを持つ。Runtime の枠を
// 取ったときだけ使うので、それ自身は排他を持たない。
type runner struct {
	logger *slog.Logger
	rt     wazero.Runtime
	cache  map[string]wazero.CompiledModule
	order  []string
}

func newRunner(ctx context.Context, logger *slog.Logger) (*runner, error) {
	// ランタイムのページの上限は wazero の既定（4 GiB）のままにする。wazero は上限を超える
	// memory.grow をメモリの確保に渡さずに断るので、上限を 128 MiB に置くと、超えたことを
	// cappedMemory が記録できない。実際に使える大きさは cappedMemory が
	// domain.WorkInfoPluginMemoryLimitPages で止める。
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
	return &runner{logger: logger, rt: rt, cache: map[string]wazero.CompiledModule{}}, nil
}

func (rn *runner) close(ctx context.Context) error {
	return rn.rt.Close(ctx)
}

func (rn *runner) run(ctx context.Context, digest string, module, input []byte) ([]byte, error) {
	compiled, err := rn.compiled(ctx, digest, module)
	if err != nil {
		return nil, err
	}
	runCtx, cancelTimeout := context.WithTimeoutCause(ctx, domain.WorkInfoPluginRunTimeout, errTimedOut)
	defer cancelTimeout()
	runCtx, cancel := context.WithCancelCause(runCtx)
	defer cancel(nil)

	memory := &cappedMemory{limit: memoryLimitBytes}
	stdout := &limitedOutput{limit: domain.WorkInfoPluginOutputMaxBytes, exceed: func() { cancel(errOutputTooLarge) }}
	stderr := &keptOutput{limit: domain.WorkInfoPluginStderrKeptBytes}
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
		_ = mod.Close(ctx)
	}
	rn.logStderr(digest, stderr)
	if failure := runFailure(ctx, runCtx, err, stdout.exceeded, memory.exceeded); failure != nil {
		if err != nil {
			rn.logger.Info("work-info plugin run failed", "digest", digest, "error", err)
		}
		return nil, failure
	}
	return stdout.buf.Bytes(), nil
}

// runFailure は実行の終わり方を失敗に対応させる。出力の超過は、プラグインが書き込みの誤りを
// 無視して成功で終わっても失敗にする。
func runFailure(parent, runCtx context.Context, err error, outputExceeded, memoryExceeded bool) error {
	switch {
	case outputExceeded:
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunOutputTooLarge}
	case err == nil:
		return nil
	case parent.Err() != nil:
		return parent.Err()
	case errors.Is(context.Cause(runCtx), errTimedOut):
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunTimedOut}
	case memoryExceeded:
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunMemoryExceeded}
	default:
		return &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed}
	}
}

// compiled はダイジェストのコンパイル済みのモジュールを返す。無ければコンパイルして持ち、
// 持つ数が compiledCacheSize を超えたら最も古いものを閉じる。
func (rn *runner) compiled(ctx context.Context, digest string, module []byte) (wazero.CompiledModule, error) {
	if c, ok := rn.cache[digest]; ok {
		return c, nil
	}
	layout, err := readLayout(module)
	if err != nil {
		rn.logger.Info("work-info plugin is malformed", "digest", digest, "error", err)
		return nil, &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed}
	}
	c, err := rn.rt.CompileModule(ctx, capTables(module, layout, domain.WorkInfoPluginTableMaxElements))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		rn.logger.Info("work-info plugin did not compile", "digest", digest, "error", err)
		return nil, &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed}
	}
	// 初めの大きさが上限を超えるメモリは cappedMemory が用意できない。検査では上限を
	// ちょうどに置いたランタイムでコンパイルが失敗するので、ここに来るのは検査を経ないモジュールだけである。
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

// sleep はプラグインの眠りを実行の終わりで打ち切る。打ち切らないと、眠っている実行が時間の
// 上限を過ぎても止まらない。
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

// limitedOutput は標準出力を limit バイトまで持つ。超える書き込みが来たら exceed を呼んで
// 実行を止め、以後の書き込みを断る。
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

// keptOutput は標準エラーの先頭 limit バイトを持ち、残りを捨てる。プラグインには
// 書けたように見せる。
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

// cappedMemory はプラグインの線形メモリを limit バイトまで確保する。超える大きさを求められたら
// 断り（memory.grow は -1 を返す）、超えたことを記録する。wazero が実行ごとに 1 つ使う。
type cappedMemory struct {
	limit    uint64
	buf      []byte
	exceeded bool
}

func (m *cappedMemory) Allocate(capacity, _ uint64) experimental.LinearMemory {
	m.buf = make([]byte, 0, min(capacity, m.limit))
	return m
}

func (m *cappedMemory) Reallocate(size uint64) []byte {
	if size > m.limit {
		m.exceeded = true
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
