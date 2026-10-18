// preview builds and starts vv with sample media for Codespaces or local review. Data stays in .local/preview, separate from task dev.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/syudead/vv/scripts/devtools"
)

var requiredCommands = []string{"go", "npm", "ffmpeg", "ffprobe"}

// Expose the backend only through the proxy.
const backendAddress = "127.0.0.1:18080"

// Distinguish the preview proxy from other servers that also answer /api/health.
const (
	markerPath   = "/__vv-preview"
	markerHeader = "X-Vv-Preview"
)

// Mix formats to exercise both directly playable and unsupported media.
type sample struct {
	path string
	args []string
}

var samples = []sample{
	{"映画/テストパターン.mp4", []string{
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=20",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest",
	}},
	{"映画/カラーバー.mp4", []string{
		"-f", "lavfi", "-i", "smptebars=size=640x360:rate=30:duration=15",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
	}},
	{"アニメ/第01話.webm", []string{
		"-f", "lavfi", "-i", "mandelbrot=size=640x360:rate=24",
		"-t", "10", "-c:v", "libvpx-vp9", "-deadline", "realtime", "-b:v", "500k",
	}},
	{"アニメ/第02話.mkv", []string{
		"-f", "lavfi", "-i", "life=size=640x360:rate=24:mold=10",
		"-t", "10", "-c:v", "mpeg4", "-q:v", "5",
	}},
	{"clips/short.mov", []string{
		"-f", "lavfi", "-i", "rgbtestsrc=size=320x240:rate=30:duration=5",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
	}},
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

	listen := os.Getenv("PREVIEW_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	// Reuse a running preview when Codespaces reopens.
	switch running, err := runningPreview(listen); {
	case err != nil:
		devtools.Fail(err)
	case running != "":
		fmt.Println("VVMDM preview はすでに動いている:", running)
		return
	}

	previewDir := filepath.Join(root, ".local", "preview")
	mediaDir := filepath.Join(previewDir, "media")
	dataDir := filepath.Join(previewDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		devtools.Fail(err)
	}
	if err := ensureSamples(mediaDir); err != nil {
		devtools.Fail(err)
	}
	if err := devtools.Run(root, "go", "run", "./scripts/build"); err != nil {
		devtools.Fail(err)
	}

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	backend := exec.Command(filepath.Join(root, "bin", "mdm"))
	backend.Dir = root
	backend.Env = append(backendEnviron(os.Environ()), "MDM_ADDR="+backendAddress, "MDM_DATA_DIR="+dataDir)
	backend.Stdout = os.Stdout
	backend.Stderr = os.Stderr
	if err := backend.Start(); err != nil {
		devtools.Fail(fmt.Errorf("VVMDM を起動できません: %w", err))
	}
	// Close the channel to broadcast termination to all three waiters.
	backendDone := make(chan struct{})
	var backendErr error
	go func() {
		backendErr = backend.Wait()
		close(backendDone)
	}()
	stopBackend := func() {
		_ = backend.Process.Signal(syscall.SIGTERM)
		select {
		case <-backendDone:
		case <-time.After(20 * time.Second):
			_ = backend.Process.Kill()
		}
	}

	backendURL := &url.URL{Scheme: "http", Host: backendAddress}
	if err := waitHealthy(backendURL, backendDone, &backendErr); err != nil {
		stopBackend()
		devtools.Fail(err)
	}
	if err := registerSamples(backendURL, mediaDir); err != nil {
		stopBackend()
		devtools.Fail(err)
	}
	// Finish importing before opening the browser so an empty library cannot be mistaken for a failure.
	if err := waitScan(backendURL); err != nil {
		stopBackend()
		devtools.Fail(err)
	}
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		stopBackend()
		devtools.Fail(fmt.Errorf("%s で待ち受けられません: %w", listen, err))
	}
	proxy := &http.Server{Handler: newProxy(backendURL), ReadHeaderTimeout: 10 * time.Second}
	proxyExit := make(chan error, 1)
	go func() { proxyExit <- proxy.Serve(listener) }()

	fmt.Println()
	fmt.Println("VVMDM preview:", publicURL(listenPort(listener.Addr())))
	fmt.Println("Media:     ", mediaDir)
	fmt.Println("Data:      ", dataDir)
	fmt.Println("Ctrl+C で止める。")

	select {
	case <-interrupts:
		shutdown(proxy)
		stopBackend()
	case <-backendDone:
		shutdown(proxy)
		devtools.Fail(fmt.Errorf("VVMDM が停止した（%s）。上の出力を確認すること。", exitReason(backendErr)))
	case err := <-proxyExit:
		stopBackend()
		devtools.Fail(fmt.Errorf("中継が停止した: %w", err))
	}
}

// Any server exit is unexpected, even with status zero.
func exitReason(err error) string {
	if err == nil {
		return "正常終了"
	}
	return err.Error()
}

func shutdown(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

// Generate only missing samples; remove .local/preview to rebuild them.
func ensureSamples(mediaDir string) error {
	for _, s := range samples {
		target := filepath.Join(mediaDir, filepath.FromSlash(s.path))
		if _, err := os.Stat(target); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		fmt.Println("サンプル動画を作る:", s.path)
		// Rename completed output atomically so interrupted files are never reused.
		partial := target + ".partial" + filepath.Ext(target)
		args := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, s.args...)
		args = append(args, partial)
		cmd := exec.Command("ffmpeg", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			_ = os.Remove(partial)
			return fmt.Errorf("サンプル動画 %s を作れません: %w", s.path, err)
		}
		if err := os.Rename(partial, target); err != nil {
			return err
		}
	}
	return nil
}

func waitHealthy(backend *url.URL, exited <-chan struct{}, exitErr *error) error {
	health := backend.JoinPath("api", "health").String()
	deadline := time.After(60 * time.Second)
	for {
		response, err := http.Get(health)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-exited:
			return fmt.Errorf("VVMDM が起動中に停止した（%s）。上の出力を確認すること。", exitReason(*exitErr))
		case <-deadline:
			return errors.New("VVMDM が 60 秒以内に応答しなかった")
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// Rescan on every launch to recover interrupted imports and include added files.
func registerSamples(backend *url.URL, mediaDir string) error {
	var folders []struct {
		Path string `json:"path"`
	}
	if err := callJSON(http.MethodGet, backend.JoinPath("api", "media-folders"), nil, &folders); err != nil {
		return err
	}
	registered := false
	for _, folder := range folders {
		if filepath.Clean(folder.Path) == filepath.Clean(mediaDir) {
			registered = true
		}
	}
	if !registered {
		if err := callJSON(http.MethodPost, backend.JoinPath("api", "media-folders"), map[string]string{"path": mediaDir}, nil); err != nil {
			return fmt.Errorf("サンプルのフォルダを登録できません: %w", err)
		}
		fmt.Println("サンプルのフォルダを登録した。")
	}
	if err := callJSON(http.MethodPost, backend.JoinPath("api", "scans"), map[string]string{}, nil); err != nil {
		return fmt.Errorf("取り込みを開始できません: %w", err)
	}
	return nil
}

// Fail on scan errors rather than advertising an incomplete preview; timeouts only warn because the server remains usable.
func waitScan(backend *url.URL) error {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var scan struct {
			State  string `json:"state"`
			Failed int    `json:"failed"`
			Error  string `json:"error"`
		}
		err := callJSON(http.MethodGet, backend.JoinPath("api", "scans", "current"), nil, &scan)
		switch {
		case err != nil || scan.State == "running":
		case scan.State == "failed":
			return fmt.Errorf("取り込みが失敗した: %s。上の出力を確認すること。", scan.Error)
		default:
			if scan.Failed > 0 {
				fmt.Printf("取り込みは終わったが、%d 件は取り込めなかった。上の出力を確認すること。\n", scan.Failed)
			} else {
				fmt.Println("取り込みが終わった。")
			}
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("取り込みがまだ続いている。一覧は取り込みの進みに合わせて増える。")
	return nil
}

// Disable local-file opening: every proxied request appears to come from loopback.
func backendEnviron(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if name == "DISPLAY" || name == "WAYLAND_DISPLAY" {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// Reject unrelated servers on the requested port instead of mistaking them for an existing preview.
func runningPreview(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("PREVIEW_ADDR=%q を解釈できません: %w", listen, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + markerPath)
	if err != nil {
		return "", nil
	}
	_ = response.Body.Close()
	if response.Header.Get(markerHeader) == "" {
		return "", fmt.Errorf("%s は preview 以外のものが使っている。止めるか PREVIEW_ADDR で別のポートを指定すること。", listen)
	}
	return publicURL(port), nil
}

func callJSON(method string, target *url.URL, body, out any) error {
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			return err
		}
	}
	request, err := http.NewRequest(method, target.String(), &payload)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= http.StatusBadRequest {
		var detail bytes.Buffer
		_, _ = detail.ReadFrom(response.Body)
		return fmt.Errorf("%s %s: %s %s", method, target.Path, response.Status, strings.TrimSpace(detail.String()))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

// Codespaces terminates HTTPS before forwarding HTTP. Rewrite only matching public Origins to the backend origin; reject writes with missing or foreign Origins without weakening backend checks.
func newProxy(backend *url.URL) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(backend)
			// Preserve Host so remote requests never acquire loopback-only privileges.
			r.Out.Host = r.In.Host
			if sameSiteOrigin(r.In) {
				r.Out.Header.Set("Origin", "http://"+r.In.Host)
			}
		},
		// Flush SSE events immediately.
		FlushInterval: -1,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(markerHeader, "1")
		if r.URL.Path == markerPath {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// The public proxy accepts writes only with a same-origin browser signal, unlike the backend CLI path.
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !sameSiteOrigin(r) {
				http.Error(w, "same-originの操作だけを受け付けます", http.StatusForbidden)
				return
			}
		}
		proxy.ServeHTTP(w, r)
	})
}

// Trust Sec-Fetch-Site: same-origin when present; otherwise compare Host or X-Forwarded-Host without scheme because the proxy terminates TLS.
func sameSiteOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "same-origin":
		return true
	case "":
	default:
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	candidates := []string{r.Host}
	for forwarded := range strings.SplitSeq(r.Header.Get("X-Forwarded-Host"), ",") {
		if host := strings.TrimSpace(forwarded); host != "" {
			candidates = append(candidates, host)
		}
	}
	for _, host := range candidates {
		if strings.EqualFold(parsed.Host, host) {
			return true
		}
	}
	return false
}

func listenPort(addr net.Addr) string {
	if tcp, ok := addr.(*net.TCPAddr); ok {
		return fmt.Sprint(tcp.Port)
	}
	return "8080"
}

// publicURL builds the forwarded Codespaces URL when applicable.
func publicURL(port string) string {
	name := os.Getenv("CODESPACE_NAME")
	domain := os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN")
	if name != "" && domain != "" {
		return fmt.Sprintf("https://%s-%s.%s", name, port, domain)
	}
	return "http://localhost:" + port
}
