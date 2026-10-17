package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/wasmplugin"
	"github.com/syudead/vv/internal/wasmplugin/plugintest"
)

// mdm plugin run のテスト（specs/056-plugin-tooling/contracts/plugin-cli.md）。
// 実行には plugintest の WASI fixture、引数検証には testdata/minimal.wasm を使う。CI はインターネットに
// 出ないので、site.test を公開のアドレスに解決し、その接続を loopback に立てたサイトへつなぐ。

const runManifest = `{"contract":1,"name":"run-test","version":"0.1.0","operations":["search","fetch"],` +
	`"settings":[{"key":"token","label":"API token","secret":true,"required":true},` +
	`{"key":"region","label":"Region","default":"jp","options":["jp","us"]}]}`

// runModulePath は CLI fixture にマニフェストを足したモジュールを一時ファイルに書き、そのパスを返す。
// manifest が空ならマニフェストを足さない。
func runModulePath(t *testing.T, manifest string) string {
	t.Helper()
	return writeRunModulePath(t, plugintest.CLI(), manifest)
}

func writeRunModulePath(t *testing.T, module []byte, manifest string) string {
	t.Helper()
	if manifest != "" {
		var err error
		if module, err = wasmplugin.WithManifest(module, []byte(manifest)); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "plugin.wasm")
	if err := os.WriteFile(path, module, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// newRunSite は /page に hello と答える loopback のサイトを立てる。
func newRunSite(t *testing.T) *httptest.Server {
	t.Helper()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(site.Close)
	return site
}

// siteOptions は site.test を公開のアドレスに解決し、規則の許した接続を site へつなぐ選択肢である。
func siteOptions(site *httptest.Server) wasmplugin.Options {
	return wasmplugin.Options{
		Resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "site.test" {
				return []netip.Addr{netip.MustParseAddr("93.184.215.14")}, nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		},
		Connect: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, site.Listener.Addr().String())
		},
	}
}

type pluginRunResult struct {
	code   int
	stdout string
	stderr string
}

// runPluginCommand は mdm plugin run を args で実行する。MDM_DATA_DIR を読めばテストを止める。
func runPluginCommand(t *testing.T, opts wasmplugin.Options, args ...string) pluginRunResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := accountEnv{
		Getenv: func(key string) string {
			t.Errorf("plugin run read the environment variable %s", key)
			return ""
		},
		Stdin:         strings.NewReader(""),
		Stdout:        &stdout,
		Stderr:        &stderr,
		PluginRuntime: opts,
	}
	code := runCommand(context.Background(), append([]string{"plugin", "run"}, args...), env)
	return pluginRunResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func wantRun(t *testing.T, got pluginRunResult, code int, stderr ...string) {
	t.Helper()
	if got.code != code {
		t.Fatalf("exit = %d, want %d; stdout %q, stderr %q", got.code, code, got.stdout, got.stderr)
	}
	for _, want := range stderr {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr %q does not contain %q", got.stderr, want)
		}
	}
	if code != exitAccountOK && got.stdout != "" {
		t.Errorf("stdout = %q, want nothing on a failure", got.stdout)
	}
}

type keptFetch struct {
	WorkInfo struct {
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
	} `json:"workInfo"`
}

func decodeFetch(t *testing.T, stdout string) keptFetch {
	t.Helper()
	var got keptFetch
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout %q is not the kept result: %v", stdout, err)
	}
	return got
}

func TestPluginRunFetchPrintsTheKeptWorkInfoAndThePluginStderr(t *testing.T) {
	t.Parallel()
	module := runModulePath(t, runManifest)
	site := newRunSite(t)

	got := runPluginCommand(t, siteOptions(site), module, "fetch", "site.test/page", "--set", "token=s3cret")
	wantRun(t, got, exitAccountOK, "fetching http://site.test/page\n")
	if !strings.HasPrefix(got.stdout, "{\n  \"workInfo\": {") {
		t.Errorf("stdout %q is not indented JSON with workInfo", got.stdout)
	}
	// region は値を渡していないので、要求は既定値 jp を運ぶ。秘密の値は伏せない。
	kept := decodeFetch(t, got.stdout)
	if kept.WorkInfo.Title != "hello" || !slices.Equal(kept.WorkInfo.Tags, []string{"region=jp", "token=s3cret"}) {
		t.Fatalf("workInfo = %+v, want the page and the effective settings", kept.WorkInfo)
	}

	got = runPluginCommand(t, siteOptions(site), "--set=region=us", module, "fetch", "site.test/page", "--set", "token=s3cret")
	wantRun(t, got, exitAccountOK)
	if kept := decodeFetch(t, got.stdout); !slices.Equal(kept.WorkInfo.Tags, []string{"region=us", "token=s3cret"}) {
		t.Fatalf("tags = %q, want the given region", kept.WorkInfo.Tags)
	}
}

func TestPluginRunSearchPrintsTheCandidates(t *testing.T) {
	t.Parallel()
	got := runPluginCommand(t, wasmplugin.Options{}, runModulePath(t, runManifest), "search", "  words  ", "--set", "token=x")
	wantRun(t, got, exitAccountOK)
	var kept struct {
		Candidates []struct{ ID, URL, Title string } `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &kept); err != nil || len(kept.Candidates) != 1 ||
		kept.Candidates[0].Title != "words" || kept.Candidates[0].ID != "site.test/works/1" ||
		kept.Candidates[0].URL != "https://site.test/works/1" {
		t.Fatalf("stdout %q, err %v; want one candidate with the trimmed words", got.stdout, err)
	}
}

func TestPluginRunSearchTakesWordsThatStartWithTwoHyphensAfterTheSeparator(t *testing.T) {
	t.Parallel()
	got := runPluginCommand(t, wasmplugin.Options{}, runModulePath(t, runManifest),
		"--set", "token=x", "--", "search", "--draft")
	wantRun(t, got, exitAccountOK)
	if !strings.Contains(got.stdout, `"title": "--draft"`) {
		t.Fatalf("stdout %q, want a candidate titled --draft", got.stdout)
	}
}

func TestPluginRunPutsTheFailureOnItsOwnLine(t *testing.T) {
	t.Parallel()
	got := runPluginCommand(t, wasmplugin.Options{}, runModulePath(t, runManifest), "search", "fail", "--set", "token=x")
	wantRun(t, got, exitAccountError, "no newline\nRun failed: plugin_failed: failed on purpose\n")
}

func TestPluginRunAPluginThatSleepsPastTheLimitTimesOut(t *testing.T) {
	t.Parallel()
	got := runPluginCommand(t, wasmplugin.Options{}, runModulePath(t, runManifest), "search", "sleep", "--set", "token=x")
	wantRun(t, got, exitAccountError, "Run failed: timed_out")
}

func TestPluginRunARequestToLoopbackIsBlocked(t *testing.T) {
	t.Parallel()
	site := newRunSite(t)
	got := runPluginCommand(t, wasmplugin.Options{}, runModulePath(t, runManifest),
		"fetch", strings.TrimPrefix(site.URL, "http://")+"/page", "--set", "token=x")
	wantRun(t, got, exitAccountError, "fetching "+site.URL, "Run failed: blocked_address, host 127.0.0.1")
}

func TestPluginRunSendsTheRequestsThroughTheProxy(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Host)
		mu.Unlock()
		_, _ = w.Write([]byte("proxied"))
	}))
	t.Cleanup(proxy.Close)
	// Connect は無いので、プロキシを通らない要求はどこにも届かない。
	opts := siteOptions(newRunSite(t))
	opts.Connect = nil

	got := runPluginCommand(t, opts, runModulePath(t, runManifest),
		"fetch", "site.test/page", "--set", "token=x", "--proxy", proxy.URL)
	wantRun(t, got, exitAccountOK)
	if kept := decodeFetch(t, got.stdout); kept.WorkInfo.Title != "proxied" {
		t.Fatalf("title = %q, want the proxy's answer", kept.WorkInfo.Title)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(seen, []string{"site.test"}) {
		t.Fatalf("proxy saw %q, want site.test", seen)
	}
}

func TestPluginRunRefusesWithUsage(t *testing.T) {
	t.Parallel()
	const fixture = "testdata/minimal.wasm"
	module := writeRunModulePath(t, readFile(t, fixture), runManifest)
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"no manifest", []string{fixture, "fetch", "site.test/1", "--set", "token=x"},
			[]string{"manifest_missing", "mdm plugin pack"}},
		{"undeclared setting", []string{module, "fetch", "site.test/1", "--set", "token=x", "--set", "colour=red"},
			[]string{`no setting "colour"`}},
		{"missing required setting", []string{module, "fetch", "site.test/1"},
			[]string{"no value and no default: token"}},
		{"201-character query", []string{module, "search", strings.Repeat("q", 201), "--set", "token=x"},
			[]string{"1 to 200 characters"}},
		{"blank fetch input", []string{module, "fetch", "   ", "--set", "token=x"},
			[]string{"work's ID must be 1 to 200 characters"}},
		{"undeclared operation", []string{module, "lookup", "x", "--set", "token=x"},
			[]string{`does not declare the operation "lookup"`, "search, fetch"}},
		{"proxy with a path", []string{module, "fetch", "site.test/1", "--set", "token=x", "--proxy", "http://p.test/x"},
			[]string{"--proxy must be"}},
		{"setting value longer than the screen saves", []string{module, "fetch", "site.test/1", "--set", "token=" + strings.Repeat("v", 2001)},
			[]string{`value of "token"`, "at most 2000 characters"}},
		{"set without a value", []string{module, "fetch", "site.test/1", "--set"}, []string{"--set needs a value"}},
		{"set without KEY=", []string{module, "fetch", "site.test/1", "--set", "token"}, []string{"KEY=VALUE"}},
		{"unknown option", []string{module, "fetch", "site.test/1", "--verbose"}, []string{"Unknown option: --verbose"}},
		{"two arguments", []string{module, "fetch"}, []string{"exactly three arguments"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runPluginCommand(t, wasmplugin.Options{}, tc.args...)
			wantRun(t, got, exitAccountUsage, append(tc.want, "Usage:", "mdm plugin run")...)
		})
	}
}

func TestPluginRunCannotReadTheModule(t *testing.T) {
	t.Parallel()
	got := runPluginCommand(t, wasmplugin.Options{}, filepath.Join(t.TempDir(), "missing.wasm"), "search", "x")
	wantRun(t, got, exitAccountError, "Cannot read the module")
	if strings.Contains(got.stderr, "Usage:") {
		t.Errorf("stderr %q shows usage for an unreadable file", got.stderr)
	}
}
