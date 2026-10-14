//go:build wasip1

// probe は internal/wasmplugin のテストが使う作品情報プラグインである。search では query、
// fetch では url の最後の要素で振る舞いを選び、どれにも当たらない要求には要求をそのまま映した
// 応答を返す（echo）。作品情報を返す振る舞いは fetch で選ぶ。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

//go:wasmimport vv http_send
func httpSend(ptr unsafe.Pointer, n uint32) int32

type request struct {
	Contract  int               `json:"contract"`
	Operation string            `json:"operation"`
	Query     string            `json:"query"`
	URL       string            `json:"url"`
	Settings  map[string]string `json:"settings"`
}

// kept はメモリを手放さないように持つ。
var kept [][]byte

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	mode := req.Query
	if req.Operation == "fetch" {
		mode = path.Base(req.URL)
	}
	switch mode {
	case "loop":
		for {
		}
	case "sleep":
		time.Sleep(time.Hour)
	case "grow":
		for i := 0; ; i++ {
			chunk := make([]byte, 1<<20)
			chunk[len(chunk)-1] = byte(i)
			kept = append(kept, chunk)
		}
	case "flood":
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", 2<<20)))
	case "stderr":
		fmt.Fprint(os.Stderr, strings.Repeat("e", 100<<10))
		reply(map[string]any{"workInfo": map[string]any{"title": "stderr"}})
	case "hostname":
		b, err := os.ReadFile("/etc/hostname")
		reply(map[string]any{"workInfo": map[string]any{"title": string(b), "tags": []string{errText(err)}}})
	case "env":
		reply(map[string]any{"workInfo": map[string]any{
			"title": os.Getenv("HOME") + os.Getenv("PATH"),
			"tags":  append(os.Environ(), os.Args...),
		}})
	case "http":
		body := []byte(`{"method":"GET","url":"https://example.com/"}`)
		reply(map[string]any{"workInfo": map[string]any{
			"title": strconv.Itoa(int(httpSend(unsafe.Pointer(&body[0]), uint32(len(body))))),
		}})
	case "fail":
		reply(map[string]any{"error": map[string]any{"message": strings.Repeat("m", 400)}})
	case "exit":
		os.Exit(3)
	default:
		echo(req)
	}
}

// echo は search に query を映した候補を、fetch に url を題名とし設定を key=value のタグとした
// 作品情報を返す。
func echo(req request) {
	if req.Operation == "search" {
		reply(map[string]any{"candidates": []map[string]string{{
			"url":    "https://example.com/?q=" + req.Query,
			"title":  req.Query,
			"detail": "contract " + strconv.Itoa(req.Contract),
		}}})
		return
	}
	tags := make([]string, 0, len(req.Settings))
	for k, v := range req.Settings {
		tags = append(tags, k+"="+v)
	}
	sort.Strings(tags)
	reply(map[string]any{"workInfo": map[string]any{"title": req.URL, "tags": tags}})
}

func errText(err error) string {
	if err == nil {
		return "opened"
	}
	return "refused"
}

func reply(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		os.Exit(1)
	}
}
