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

//go:wasmimport vv http_meta
func httpMeta(handle int32, ptr unsafe.Pointer, n uint32) int32

//go:wasmimport vv http_body
func httpBody(handle int32, ptr unsafe.Pointer, n uint32) int32

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
		fetchPages(req.Settings)
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

// fetchPages は設定 request の http_send の要求を count 回（既定 1 回）送り、どの誤りも無視する。
// 題名は最後に成功した応答の本文、タグは各回の http_send の戻り値（codes=）と、その応答の
// http_meta の JSON（meta=）である。設定 fail が空でなければ、そのあと誤りを報告して失敗する。
func fetchPages(settings map[string]string) {
	count, err := strconv.Atoi(settings["count"])
	if err != nil {
		count = 1
	}
	request := []byte(settings["request"])
	codes := make([]string, 0, count)
	last := int32(-1)
	for range count {
		h := httpSend(unsafe.Pointer(&request[0]), uint32(len(request)))
		codes = append(codes, strconv.Itoa(int(h)))
		if h >= 0 {
			last = h
		}
	}
	if settings["fail"] != "" {
		reply(map[string]any{"error": map[string]any{"message": "gave up"}})
		return
	}
	info := map[string]any{"tags": []string{"codes=" + strings.Join(codes, ",")}}
	if last >= 0 {
		info["title"] = string(readAll(last, httpBody))
		info["tags"] = append(info["tags"].([]string), "meta="+string(readAll(last, httpMeta)))
	}
	reply(map[string]any{"workInfo": info})
}

// readAll は http_meta か http_body を、まず空の buffer で呼んで長さを知り、その大きさで呼び直す。
func readAll(handle int32, read func(int32, unsafe.Pointer, uint32) int32) []byte {
	var probe [1]byte
	n := read(handle, unsafe.Pointer(&probe[0]), 0)
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	read(handle, unsafe.Pointer(&buf[0]), uint32(n))
	return buf
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
