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

	"github.com/syudead/vv/scripts/devtools"
)

// requiredCommands は開発サーバーが使う外部コマンドである。足りない理由は
// task doctor が詳しく出すので、ここでは入口だけを塞ぐ。
var requiredCommands = []string{"go", "npm", "ffmpeg", "ffprobe"}

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
			return 0, err
		}
	}
	return len(p), nil
}

// exit は終了したサーバーを伝える。
type exit struct {
	label string
	err   error
}

func start(root, label string, env []string, name string, args ...string) (*exec.Cmd, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = &prefixWriter{prefix: "[" + label + "] ", out: os.Stdout, mu: &outputLock}
	cmd.Stderr = &prefixWriter{prefix: "[" + label + "] ", out: os.Stderr, mu: &outputLock}
	isolateProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s を起動できません: %w", label, err)
	}
	return cmd, nil
}

// outputLock は2つのサーバーの行が混ざらないようにする。
var outputLock sync.Mutex

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

	fmt.Println("Go:   http://localhost:8080")
	fmt.Println("Vite: http://localhost:5173 (/api proxies to :8080)")
	fmt.Println("Data:", dataDir)
	fmt.Println()

	goServer, err := start(root, "go", []string{"MDM_DATA_DIR=" + dataDir}, "go", "run", "./cmd/mdm")
	if err != nil {
		devtools.Fail(err)
	}
	webServer, err := start(root, "web", nil, "npm", "--prefix", "web", "run", "dev", "--",
		"--host", "127.0.0.1", "--strictPort")
	if err != nil {
		terminateGroup(goServer)
		devtools.Fail(err)
	}

	servers := map[string]*exec.Cmd{"go": goServer, "web": webServer}
	exits := make(chan exit, len(servers))
	for label, cmd := range servers {
		go func() { exits <- exit{label: label, err: cmd.Wait()} }()
	}

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	select {
	case <-interrupts:
		for _, cmd := range servers {
			terminateGroup(cmd)
		}
		drain(exits, len(servers))
		return
	case first := <-exits:
		for label, cmd := range servers {
			if label != first.label {
				terminateGroup(cmd)
			}
		}
		drain(exits, len(servers)-1)
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
// 最後の出力が表示されないまま終わることがある。
func drain(exits <-chan exit, remaining int) {
	for range remaining {
		<-exits
	}
}
