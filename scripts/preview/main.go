// preview はサンプル動画を登録した vv を1つ起動し、ブラウザから触れるようにする。
// task preview の実体。GitHub Codespaces で PR を手元で確かめるための入口で、
// .devcontainer から呼ばれる。手元でもそのまま動く。
//
// 流れは、単一バイナリのビルド → サンプル動画の生成 → vv の起動 → フォルダの
// 登録と取り込み開始 → 中継の公開、である。データは .local/preview/ に置くので、
// task dev の作業場所（.local/data）とは混ざらない。
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

// requiredCommands は build とサンプル生成と vv の起動前確認が使う外部コマンドである。
var requiredCommands = []string{"go", "npm", "ffmpeg", "ffprobe"}

// backendAddress は vv 本体の待ち受けである。外からは中継越しにしか触らせない。
const backendAddress = "127.0.0.1:18080"

// sample はサンプル動画1本の作り方である。形式を散らしておくと、ブラウザで
// 再生できるものとできないものの両方の表示を確かめられる。
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
	// Codespaces は開き直すたびに task preview を呼ぶ。前のものが動いていれば
	// ビルドからやり直さずに案内だけする。
	if running, ok := runningPreview(listen); ok {
		fmt.Println("vv preview はすでに動いている:", running)
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
		devtools.Fail(fmt.Errorf("vv を起動できません: %w", err))
	}
	// 終了は起動待ちと停止処理と最後の select の3か所で見るので、値を送るのではなく
	// close で知らせる。送った値は1か所でしか受け取れない。
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
	// 8080 を開くと Codespaces がブラウザを開くので、先に取り込みを終えておく。
	// 空の一覧が見えても、取り込み中なのか壊れているのか区別できない。
	waitScan(backendURL)
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		stopBackend()
		devtools.Fail(fmt.Errorf("%s で待ち受けられません: %w", listen, err))
	}
	proxy := &http.Server{Handler: newProxy(backendURL), ReadHeaderTimeout: 10 * time.Second}
	proxyExit := make(chan error, 1)
	go func() { proxyExit <- proxy.Serve(listener) }()

	fmt.Println()
	fmt.Println("vv preview:", publicURL(listenPort(listener.Addr())))
	fmt.Println("Media:     ", mediaDir)
	fmt.Println("Data:      ", dataDir)
	fmt.Println("Ctrl+C で止める。")

	select {
	case <-interrupts:
		shutdown(proxy)
		stopBackend()
	case <-backendDone:
		shutdown(proxy)
		devtools.Fail(fmt.Errorf("vv が停止した（%s）。上の出力を確認すること。", exitReason(backendErr)))
	case err := <-proxyExit:
		stopBackend()
		devtools.Fail(fmt.Errorf("中継が停止した: %w", err))
	}
}

// exitReason は vv の終了理由を表示用に返す。正常終了でも止まったこと自体が
// 異常なので、呼び出し側はエラーとして扱う。
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

// ensureSamples は足りないサンプル動画だけを作る。作り直したいときは
// .local/preview を消せばよい。
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
		// 途中で止まっても半端なファイルを本物と取り違えないよう、別名で作ってから移す。
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
			return fmt.Errorf("vv が起動中に停止した（%s）。上の出力を確認すること。", exitReason(*exitErr))
		case <-deadline:
			return errors.New("vv が 60 秒以内に応答しなかった")
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// registerSamples はサンプルのフォルダが未登録なら登録し、取り込みを始める。
// 取り込みは毎回始める。前回が途中で止まっていても、利用者がファイルを足して
// いても、開いたときの一覧が手元のファイルと揃う。
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

// waitScan は取り込みが終わるまで待つ。待ちきれなくても vv は使えるので、
// 失敗にはせず案内だけする。
func waitScan(backend *url.URL) {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		var scan struct {
			State string `json:"state"`
		}
		if err := callJSON(http.MethodGet, backend.JoinPath("api", "scans", "current"), nil, &scan); err == nil && scan.State != "running" {
			fmt.Println("取り込みが終わった:", scan.State)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("取り込みがまだ続いている。一覧は取り込みの進みに合わせて増える。")
}

// backendEnviron は vv に渡す環境から、ファイルを開く機能を有効にする変数を除く。
// その機能はループバックからの要求にだけ許されるが、中継越しの要求はすべて
// ループバックから届くので、preview では最初から使えなくしておく。
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

// runningPreview は listen で preview の中継がすでに応答しているかを返す。
func runningPreview(listen string) (string, bool) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", false
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/api/health")
	if err != nil {
		return "", false
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", false
	}
	return publicURL(port), true
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

// newProxy は外からの要求を vv へ中継する。
//
// Codespaces のポート転送はブラウザの https を終端し、中へは http で渡す。vv の
// 書き込み操作は Origin の scheme と host を自分の要求と突き合わせるので、そのまま
// 通すと https と http が食い違って全部 403 になる。そこで、Origin がブラウザの
// 見ている公開側の origin と同じときに限り、vv から見た origin（http と受けた
// Host）へ書き換える。
// 公開側と異なる Origin（別サイトからの要求）は書き換えずに渡し、vv 自身の確認で
// 断らせる。vv 本体の確認は緩めない。
func newProxy(backend *url.URL) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(backend)
			// Host は書き換えない。vv はループバックの Host をローカルの操作と
			// みなすので、外からの要求をループバック扱いにしてしまう。
			r.Out.Host = r.In.Host
			if sameSiteOrigin(r.In) {
				r.Out.Header.Set("Origin", "http://"+r.In.Host)
			}
		},
		// SSE（/api/events）を溜めずに流す。
		FlushInterval: -1,
	}
}

// sameSiteOrigin は Origin がブラウザの見ている公開側の origin と同じかを返す。
// ブラウザが Sec-Fetch-Site: same-origin を付けていればそれを信じる。付いて
// いなければ、公開側の host を Host か転送元が付ける X-Forwarded-Host から取って
// 比べる。scheme は転送元が終端するので比べない。
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

// publicURL は開く先を案内する。Codespaces では転送先の URL を組み立てる。
func publicURL(port string) string {
	name := os.Getenv("CODESPACE_NAME")
	domain := os.Getenv("GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN")
	if name != "" && domain != "" {
		return fmt.Sprintf("https://%s-%s.%s", name, port, domain)
	}
	return "http://localhost:" + port
}
