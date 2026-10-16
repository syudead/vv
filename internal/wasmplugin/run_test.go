package wasmplugin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/wasmplugin/plugintest"
)

// 実行のテストは testdata/probe をビルドしたプラグインを使う。probe は要求の query で振る舞いを選ぶ。

const probeDigest = "probe"

func probeModule(t *testing.T) []byte {
	t.Helper()
	return mustWithManifest(t, plugintest.Build(t, "testdata/probe"), goodManifest)
}

func search(query string) domain.WorkInfoRunRequest {
	return domain.WorkInfoRunRequest{Operation: domain.WorkInfoSearch, Query: query}
}

func fetch() domain.WorkInfoRunRequest {
	return domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, URL: "https://example.com/works/1"}
}

// fetchMode は probe に振る舞い mode を選ばせる fetch の要求である。
func fetchMode(mode string) domain.WorkInfoRunRequest {
	return domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, URL: "https://probe.test/" + mode}
}

// wantFailure は err が失敗 want の *domain.WorkInfoRunError であることを確かめ、
// プラグインが報告した誤りの文を返す。
func wantFailure(t *testing.T, err error, want domain.WorkInfoRunFailure) string {
	t.Helper()
	var runErr *domain.WorkInfoRunError
	if !errors.As(err, &runErr) || runErr.Failure != want {
		t.Fatalf("err = %v, want failure %q", err, want)
	}
	return runErr.PluginMessage
}

// wantEchoWorks は次の実行が成功することを確かめる。
func wantEchoWorks(t *testing.T, rt *Runtime, module []byte) {
	t.Helper()
	got, _, err := rt.Run(context.Background(), probeDigest, module, search("next"), domain.WorkInfoRunNetwork{}, nil)
	if err != nil || len(got.Candidates) != 1 || got.Candidates[0].Title != "next" {
		t.Fatalf("next Run = %+v, %v; want the echoed candidate", got, err)
	}
}

func TestRunReadsBackTheEchoedResponse(t *testing.T) {
	rt := newTestRuntime(t)
	module := probeModule(t)

	got, _, err := rt.Run(context.Background(), probeDigest, module, search("Example work"), domain.WorkInfoRunNetwork{}, nil)
	if err != nil {
		t.Fatalf("Run(search): %v", err)
	}
	want := domain.WorkInfoCandidate{URL: "https://example.com/?q=Example work", Title: "Example work", Detail: "contract 1"}
	if len(got.Candidates) != 1 || got.Candidates[0] != want {
		t.Fatalf("candidates = %+v, want [%+v]", got.Candidates, want)
	}

	req := fetch()
	req.Settings = map[string]string{"apiKey": "secret", "lang": "ja"}
	got, _, err = rt.Run(context.Background(), probeDigest, module, req, domain.WorkInfoRunNetwork{}, nil)
	if err != nil {
		t.Fatalf("Run(fetch): %v", err)
	}
	if got.WorkInfo.Title != req.URL || strings.Join(got.WorkInfo.Tags, ",") != "apiKey=secret,lang=ja" {
		t.Fatalf("work info = %+v, want the URL and the settings echoed", got.WorkInfo)
	}
}

func TestRunStopsAPluginThatNeverEndsAndRefusesASecondRun(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime(t)
	module := probeModule(t)

	first := make(chan error, 1)
	var firstLog domain.WorkInfoRunLog
	start := time.Now()
	go func() {
		_, log, err := rt.Run(context.Background(), probeDigest, module, search("loop"), domain.WorkInfoRunNetwork{}, nil)
		firstLog = log
		first <- err
	}()
	for len(rt.slot) == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	waited := time.Now()
	_, _, err := rt.Run(context.Background(), probeDigest, module, search("second"), domain.WorkInfoRunNetwork{}, nil)
	wantFailure(t, err, domain.WorkInfoRunBusy)
	if d := time.Since(waited); d < domain.WorkInfoPluginBusyWait {
		t.Fatalf("second run refused after %v, want after %v", d, domain.WorkInfoPluginBusyWait)
	}

	wantFailure(t, <-first, domain.WorkInfoRunTimedOut)
	if firstLog.Stderr != "looping\n" || firstLog.Truncated {
		t.Fatalf("timed-out run log = %+v, want the line it wrote", firstLog)
	}
	if d := time.Since(start); d < domain.WorkInfoPluginRunTimeout || d > domain.WorkInfoPluginRunTimeout+5*time.Second {
		t.Fatalf("looping run stopped after %v, want %v", d, domain.WorkInfoPluginRunTimeout)
	}
	wantEchoWorks(t, rt, module)
}

func TestRunStopsAPluginThatGrowsItsMemory(t *testing.T) {
	t.Parallel()
	rt := newTestRuntime(t)
	module := probeModule(t)

	_, _, err := rt.Run(context.Background(), probeDigest, module, search("grow"), domain.WorkInfoRunNetwork{}, nil)
	wantFailure(t, err, domain.WorkInfoRunMemoryExceeded)
	wantEchoWorks(t, rt, module)
}

func TestRunStopsAPluginThatWritesTooMuch(t *testing.T) {
	rt := newTestRuntime(t)
	module := probeModule(t)

	_, _, err := rt.Run(context.Background(), probeDigest, module, search("flood"), domain.WorkInfoRunNetwork{}, nil)
	wantFailure(t, err, domain.WorkInfoRunOutputTooLarge)
	wantEchoWorks(t, rt, module)
}

func TestRunGivesNoFilesNoEnvironmentAndNoArguments(t *testing.T) {
	t.Setenv("VV_WASMPLUGIN_TEST_SECRET", "leaked")
	rt := newTestRuntime(t)
	module := probeModule(t)

	got, _, err := rt.Run(context.Background(), probeDigest, module, fetchMode("hostname"), domain.WorkInfoRunNetwork{}, nil)
	if err != nil {
		t.Fatalf("Run(hostname): %v", err)
	}
	if got.WorkInfo.Title != "" || strings.Join(got.WorkInfo.Tags, ",") != "refused" {
		t.Fatalf("hostname run = %+v, want nothing read and the open refused", got.WorkInfo)
	}

	got, _, err = rt.Run(context.Background(), probeDigest, module, fetchMode("env"), domain.WorkInfoRunNetwork{}, nil)
	if err != nil {
		t.Fatalf("Run(env): %v", err)
	}
	if got.WorkInfo.Title != "" || len(got.WorkInfo.Tags) != 0 {
		t.Fatalf("env run = %+v, want no environment variables and no arguments", got.WorkInfo)
	}
}

func TestRunReportsPluginFailures(t *testing.T) {
	rt := newTestRuntime(t)
	module := probeModule(t)

	_, log, err := rt.Run(context.Background(), probeDigest, module, search("fail"), domain.WorkInfoRunNetwork{}, nil)
	message := wantFailure(t, err, domain.WorkInfoRunFailed)
	if message != strings.Repeat("m", domain.WorkInfoPluginMessageMaxRunes) {
		t.Fatalf("message = %q, want its first %d characters", message, domain.WorkInfoPluginMessageMaxRunes)
	}
	if log.Stderr != "reporting an error\n" || log.Operation != domain.WorkInfoSearch {
		t.Fatalf("failed run log = %+v, want the line it wrote", log)
	}

	_, log, err = rt.Run(context.Background(), probeDigest, module, search("exit"), domain.WorkInfoRunNetwork{}, nil)
	wantFailure(t, err, domain.WorkInfoRunFailed)
	if log.Stderr != "exiting with 3\n" {
		t.Fatalf("exited run log = %+v, want the line it wrote", log)
	}
}

// syncBuffer は複数の goroutine から書かれるログの受け皿である。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRunKeepsTheStartOfStderrInTheServerLogAndTheRunLog(t *testing.T) {
	var log syncBuffer
	rt, err := New(context.Background(), Options{Logger: slog.New(slog.NewJSONHandler(&log, nil))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(context.Background()) })

	var recorded []domain.WorkInfoRunLog
	record := func(l domain.WorkInfoRunLog) error {
		recorded = append(recorded, l)
		return errors.New("the store is down")
	}
	got, runLog, err := rt.Run(context.Background(), probeDigest, probeModule(t), fetchMode("stderr"), domain.WorkInfoRunNetwork{}, record)
	if err != nil || got.WorkInfo.Title != "stderr" {
		t.Fatalf("Run(stderr) = %+v, %v; want success although record failed", got.WorkInfo, err)
	}
	kept := strings.Repeat("e", domain.WorkInfoPluginStderrKeptBytes)
	want := domain.WorkInfoRunLog{Operation: domain.WorkInfoFetch, Stderr: kept, Truncated: true}
	if runLog != want || len(recorded) != 1 || recorded[0] != want {
		t.Fatalf("returned and recorded logs do not hold the first %d bytes of stderr marked truncated",
			domain.WorkInfoPluginStderrKeptBytes)
	}
	logged := log.String()
	if !strings.Contains(logged, "the store is down") {
		t.Fatalf("log does not hold the record failure: %s", logged[:min(len(logged), 200)])
	}
	if !strings.Contains(logged, `"stderr":"`+kept+`"`) || !strings.Contains(logged, `"truncated":true`) {
		t.Fatalf("log does not hold the first %d bytes of stderr marked truncated", domain.WorkInfoPluginStderrKeptBytes)
	}
}

// sleb は i32.const の符号付き LEB128 を符号化する。
func sleb(v int32) []byte {
	var out []byte
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v == 0 && c&0x40 == 0 || v == -1 && c&0x40 != 0 {
			return append(out, c)
		}
		out = append(out, c|0x80)
	}
}

// tableGrowSpec は最大値を宣言しない空のテーブルを持ち、_start でそれを 100,000 要素増やしてから
// さらに 1 要素増やすモジュールである。2 回目の table.grow が -1 を返したときだけ標準出力に
// 応答を書き、そうでなければ trap する。
func tableGrowSpec() moduleSpec {
	s := goodSpec()
	s.imports = []testImport{funcImport("wasi_snapshot_preview1", "fd_write", typeFdWrite)}
	s.tables = [][]byte{{0x70, 0x00, 0x00}}
	tableGrow := []byte{0xfc, 0x0f, 0x00}
	refNull := []byte{0xd0, 0x70}
	var code []byte
	code = append(code, refNull...)
	code = append(append(code, 0x41), sleb(domain.WorkInfoPluginTableMaxElements)...)
	code = append(append(code, tableGrow...), 0x1a) // drop
	code = append(code, refNull...)
	code = append(append(code, 0x41), sleb(1)...)
	code = append(code, tableGrow...)
	code = append(append(code, 0x41), sleb(-1)...)
	code = append(code, 0x47, 0x04, 0x40, 0x00, 0x0b) // i32.ne; if; unreachable; end
	// fd_write(1, iovs=0, 1, nwritten=8)
	code = append(code, 0x41, 0x01, 0x41, 0x00, 0x41, 0x01, 0x41, 0x08, 0x10, 0x00, 0x1a)
	s.code = code

	response := []byte(`{"workInfo":{"title":"capped"}}`)
	data := binary.LittleEndian.AppendUint32(nil, 16)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(response)))
	data = append(data, make([]byte, 8)...)
	s.data = append(data, response...)
	return s
}

func TestRunCapsATableWithoutAMaximum(t *testing.T) {
	rt := newTestRuntime(t)
	module := tableGrowSpec().bytes()
	if _, problem, err := rt.Inspect(context.Background(), module); err != nil || problem != "" {
		t.Fatalf("Inspect = problem %q, err %v; want none", problem, err)
	}
	got, _, err := rt.Run(context.Background(), "table-grow", module, fetch(), domain.WorkInfoRunNetwork{}, nil)
	if err != nil || got.WorkInfo.Title != "capped" {
		t.Fatalf("Run = %+v, %v; want the second table.grow to return -1", got.WorkInfo, err)
	}
}

// memoryGrowSpec は _start で線形メモリを上限を超える大きさに増やし、memory.grow が -1 を
// 返したら標準出力に正しい応答を書いて成功で終わるモジュールである。
func memoryGrowSpec() moduleSpec {
	s := goodSpec()
	s.imports = []testImport{funcImport("wasi_snapshot_preview1", "fd_write", typeFdWrite)}
	var code []byte
	// メモリは 1 ページで始まるので、上限のページ数だけ増やすと上限を 1 ページ超える。
	code = append(append(code, 0x41), sleb(domain.WorkInfoPluginMemoryLimitPages)...)
	code = append(code, 0x40, 0x00) // memory.grow
	code = append(append(code, 0x41), sleb(-1)...)
	code = append(code, 0x47, 0x04, 0x40, 0x00, 0x0b) // i32.ne; if; unreachable; end
	// fd_write(1, iovs=0, 1, nwritten=8)
	code = append(code, 0x41, 0x01, 0x41, 0x00, 0x41, 0x01, 0x41, 0x08, 0x10, 0x00, 0x1a)
	s.code = code

	response := []byte(`{"workInfo":{"title":"survived"}}`)
	data := binary.LittleEndian.AppendUint32(nil, 16)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(response)))
	data = append(data, make([]byte, 8)...)
	s.data = append(data, response...)
	return s
}

func TestRunFailsAPluginThatSurvivesARefusedMemoryGrow(t *testing.T) {
	rt := newTestRuntime(t)
	got, _, err := rt.Run(context.Background(), "memory-grow", memoryGrowSpec().bytes(), fetch(), domain.WorkInfoRunNetwork{}, nil)
	if err == nil {
		t.Fatalf("Run = %+v, want memory_exceeded", got.WorkInfo)
	}
	wantFailure(t, err, domain.WorkInfoRunMemoryExceeded)
}

func TestRunCountsCompilingTowardTheRunTimeout(t *testing.T) {
	rt := newTestRuntime(t)
	module := probeModule(t)
	input, err := json.Marshal(search("echo"))
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	deadline, cancel := context.WithTimeoutCause(context.Background(), 0, errTimedOut)
	defer cancel()

	_, _, err = rt.run.run(context.Background(), deadline, "uncompiled", module, input, &httpSession{})
	wantFailure(t, err, domain.WorkInfoRunTimedOut)
	if _, ok := rt.run.cache["uncompiled"]; ok {
		t.Fatalf("the module was compiled after the run's time ran out")
	}
}

func TestCapTablesKeepsTablesThatNeedNoCap(t *testing.T) {
	s := goodSpec()
	limits := binary.AppendUvarint([]byte{0x70, 0x01, 0x00}, 10)
	s.tables = [][]byte{limits, binary.AppendUvarint([]byte{0x70, 0x00}, domain.WorkInfoPluginTableMaxElements+1)}
	module := s.bytes()
	layout, err := readLayout(module)
	if err != nil {
		t.Fatalf("readLayout: %v", err)
	}
	if got := capTables(module, layout, domain.WorkInfoPluginTableMaxElements); !bytes.Equal(got, module) {
		t.Fatalf("capTables changed a module whose tables need no cap")
	}

	s.tables = append(s.tables, []byte{0x70, 0x00, 0x05})
	module = s.bytes()
	if layout, err = readLayout(module); err != nil {
		t.Fatalf("readLayout: %v", err)
	}
	capped, err := readLayout(capTables(module, layout, domain.WorkInfoPluginTableMaxElements))
	if err != nil {
		t.Fatalf("readLayout(capped): %v", err)
	}
	want := []struct {
		min, max uint64
		hasMax   bool
	}{{0, 10, true}, {domain.WorkInfoPluginTableMaxElements + 1, 0, false}, {5, domain.WorkInfoPluginTableMaxElements, true}}
	if len(capped.tables) != len(want) {
		t.Fatalf("capped tables = %+v", capped.tables)
	}
	for i, w := range want {
		got := capped.tables[i]
		if got.min != w.min || got.hasMax != w.hasMax || got.max != w.max {
			t.Fatalf("table %d = min %d, max %d (%v); want min %d, max %d (%v)", i, got.min, got.max, got.hasMax, w.min, w.max, w.hasMax)
		}
	}
}

// record は実行の枠を持ったまま呼ばれるので、record の中から始めた 2 本目の実行は枠を取れない。2 本目は
// モジュールを動かさないので、record を呼ばない（056 research.md R-6）。
func TestRunRecordsBeforeASecondRunCanStart(t *testing.T) {
	rt := newTestRuntime(t)
	module := probeModule(t)

	var second error
	secondRecorded := false
	record := func(domain.WorkInfoRunLog) error {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, _, second = rt.Run(ctx, probeDigest, module, search("second"), domain.WorkInfoRunNetwork{},
			func(domain.WorkInfoRunLog) error {
				secondRecorded = true
				return nil
			})
		return nil
	}
	if _, _, err := rt.Run(context.Background(), probeDigest, module, search("first"), domain.WorkInfoRunNetwork{}, record); err != nil {
		t.Fatalf("Run(first): %v", err)
	}
	if !errors.Is(second, context.DeadlineExceeded) || secondRecorded {
		t.Fatalf("second run inside record = %v (recorded %v); want it still waiting for the slot", second, secondRecorded)
	}
	wantEchoWorks(t, rt, module)
}
