// dev は Go サーバーと Vite の開発サーバーを同時に動かす。
// task dev の実体。片方が落ちたらもう片方も止める。片肺のまま
// 動き続けると、手元の失敗に気付かないまま作業を続けてしまう。
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/syudead/vv/scripts/devtools"
)

// requiredCommands は開発サーバーが使う外部コマンドである。足りない理由は
// task doctor が詳しく出すので、ここでは入口だけを塞ぐ。
var requiredCommands = []string{"go", "npm", "ffmpeg", "ffprobe"}

// drainTimeout は停止を指示したサーバーの後始末を待つ上限である。孫を残す
// 経路（Windows）では出力の口が閉じず Wait が返らないので、待ち続けると
// 端末が固まる。unix の killGrace より長く取らないと、強制終了が起きる前に
// 待つのをやめてしまう。
const drainTimeout = 20 * time.Second

// outputLock は2つのサーバーの行が混ざらないようにする。
var outputLock sync.Mutex

// prefixWriter は行頭に出所を付ける。2つのサーバーの出力が混ざるので、
// どちらの行かが分からないと読めない。
type prefixWriter struct {
	prefix string
	out    io.Writer
	mu     *sync.Mutex
	buf    bytes.Buffer
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// 改行までの途中なら、次の Write と合わせて出す。
			w.buf.WriteString(line)
			break
		}
		if _, err := fmt.Fprint(w.out, w.prefix, line); err != nil {
			// 受け取った分は取り込み済みなので、書けた量ではなく
			// 消費した量を返す（io.Writer の約束）。
			return len(p), err
		}
	}
	return len(p), nil
}

// flush は改行で終わらなかった最後の行を出す。落ちる直前の一行は改行を
// 伴わないことがあり、捨てると「上の出力を確認すること」が嘘になる。
func (w *prefixWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.buf.Len() == 0 {
		return
	}
	_, _ = fmt.Fprint(w.out, w.prefix, w.buf.String(), "\n")
	w.buf.Reset()
}

// server は起動した開発サーバー1つである。
type server struct {
	label   string
	cmd     *exec.Cmd
	group   *processGroup
	writers []*prefixWriter
}

// exit は終了したサーバーを伝える。
type exit struct {
	label string
	err   error
}

func start(root, label string, env []string, name string, args ...string) (*server, error) {
	stdout := &prefixWriter{prefix: "[" + label + "] ", out: os.Stdout, mu: &outputLock}
	stderr := &prefixWriter{prefix: "[" + label + "] ", out: os.Stderr, mu: &outputLock}

	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	group, err := newProcessGroup(cmd)
	if err != nil {
		return nil, fmt.Errorf("%s の停止管理を準備できません: %w", label, err)
	}
	if err := cmd.Start(); err != nil {
		group.terminate(cmd)
		return nil, fmt.Errorf("%s を起動できません: %w", label, err)
	}
	if err := group.attach(cmd); err != nil {
		_ = cmd.Process.Kill()
		group.terminate(cmd)
		_ = cmd.Wait()
		return nil, fmt.Errorf("%s を停止管理へ登録できません: %w", label, err)
	}
	return &server{label: label, cmd: cmd, group: group, writers: []*prefixWriter{stdout, stderr}}, nil
}

func (s *server) stop() { s.group.terminate(s.cmd) }

func (s *server) flush() {
	for _, w := range s.writers {
		w.flush()
	}
}

func main() {
	root, err := devtools.RepositoryRoot()
	if err != nil {
		devtools.Fail(err)
	}

	for _, name := range requiredCommands {
		if _, err := exec.LookPath(name); err != nil {
			devtools.Fail(fmt.Errorf("%s が必要である。task doctor で不足を確認すること。", name))
		}
	}

	dataDir := os.Getenv("DEV_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(root, ".local", "data")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		devtools.Fail(err)
	}

	// サーバーは独立したプロセスグループに入り、端末の Ctrl+C を直接は
	// 受け取らない。止める責任はこちらにあるので、1つ目を起動する前に
	// 合図を受け取れるようにしておく。
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	backendAddress := os.Getenv("MDM_ADDR")
	if backendAddress == "" {
		backendAddress = ":8080"
	}
	apiTarget := os.Getenv("MDM_API_TARGET")
	if apiTarget == "" {
		apiTarget = "http://localhost:8080"
	}
	fmt.Println("Go:  ", backendAddress)
	fmt.Printf("Vite: http://localhost:5173 (/api proxies to %s)\n", apiTarget)
	fmt.Println("Data:", dataDir)
	fmt.Println()

	airPath, err := devtools.GoToolPath(root, "air")
	if err != nil {
		devtools.Fail(err)
	}
	goServer, err := start(root, "go", []string{"MDM_DATA_DIR=" + dataDir}, airPath, "-c", ".air.toml")
	if err != nil {
		devtools.Fail(err)
	}
	webServer, err := start(root, "web", nil, "npm", "--prefix", "web", "run", "dev", "--",
		"--host", "127.0.0.1", "--strictPort")
	if err != nil {
		goServer.stop()
		goServer.flush()
		devtools.Fail(err)
	}

	servers := []*server{goServer, webServer}
	exits := make(chan exit, len(servers))
	for _, s := range servers {
		go func() { exits <- exit{label: s.label, err: s.cmd.Wait()} }()
	}

	defer func() {
		for _, s := range servers {
			s.flush()
		}
	}()

	select {
	case <-interrupts:
		for _, s := range servers {
			s.stop()
		}
		drain(exits, len(servers))
	case first := <-exits:
		for _, s := range servers {
			s.stop()
		}
		drain(exits, len(servers)-1)
		for _, s := range servers {
			s.flush()
		}
		// 正常終了でも止める。開発サーバーは動き続けるのが正しい状態なので、
		// 落ちた理由が何であれ片肺では続けない。
		reason := "正常終了"
		if first.err != nil {
			reason = first.err.Error()
		}
		devtools.Fail(fmt.Errorf("%s が停止した（%s）。上の出力を確認すること。", first.label, reason))
	}
}

// drain は残りの Wait を回収してから戻る。回収しないと、止めたサーバーの
// 最後の出力が表示されないまま終わる。孫が出力の口を握ったまま残ると Wait は
// 返らないので、待つのは drainTimeout までにする。
func drain(exits <-chan exit, remaining int) {
	deadline := time.After(drainTimeout)
	for range remaining {
		select {
		case <-exits:
		case <-deadline:
			return
		}
	}
}
