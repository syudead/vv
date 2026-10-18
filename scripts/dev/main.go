// dev runs Go and Vite together, stopping both when either exits so failures remain visible.
package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/syudead/vv/scripts/devtools"
)

// Fail early for missing commands; task doctor provides detailed diagnostics.
var requiredCommands = []string{"go", "npm", "ffmpeg", "ffprobe"}

// Bound waits for descendants holding output pipes open. This timeout must exceed Unix killGrace so forced termination can finish.
const drainTimeout = 20 * time.Second

// Prevent interleaved server output.
var outputLock sync.Mutex

// prefixWriter labels lines so combined server output remains attributable.
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
			// Buffer incomplete lines until the next Write.
			w.buf.WriteString(line)
			break
		}
		if _, err := fmt.Fprint(w.out, w.prefix, line); err != nil {
			// Report bytes consumed, not bytes flushed, as required by io.Writer.
			return len(p), err
		}
	}
	return len(p), nil
}

// flush preserves an unterminated final line, which may contain the crash diagnostic.
func (w *prefixWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.buf.Len() == 0 {
		return
	}
	_, _ = fmt.Fprint(w.out, w.prefix, w.buf.String(), "\n")
	w.buf.Reset()
}

type server struct {
	label   string
	cmd     *exec.Cmd
	group   *processGroup
	writers []*prefixWriter
}

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

func apiTargetFor(backendAddress, configuredTarget string) (string, error) {
	if configuredTarget != "" {
		return configuredTarget, nil
	}
	host, port, err := net.SplitHostPort(backendAddress)
	if err != nil {
		return "", fmt.Errorf("MDM_ADDR=%q を Vite の接続先へ変換できません: %w", backendAddress, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port), nil
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

	// Child process groups do not receive terminal Ctrl+C directly, so register shutdown handling before starting them.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	backendAddress := os.Getenv("MDM_ADDR")
	if backendAddress == "" {
		backendAddress = ":8080"
	}
	apiTarget, err := apiTargetFor(backendAddress, os.Getenv("MDM_API_TARGET"))
	if err != nil {
		devtools.Fail(err)
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
	webServer, err := start(root, "web", []string{"MDM_API_TARGET=" + apiTarget}, "npm", "--prefix", "web", "run", "dev", "--",
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
		// Stop both servers even after a successful exit; neither should run alone.
		reason := "正常終了"
		if first.err != nil {
			reason = first.err.Error()
		}
		devtools.Fail(fmt.Errorf("%s が停止した（%s）。上の出力を確認すること。", first.label, reason))
	}
}

// drain waits for final output, bounded by drainTimeout in case descendants keep pipes open.
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
