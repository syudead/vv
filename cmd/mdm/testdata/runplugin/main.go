//go:build wasip1

// runplugin は mdm plugin run のテスト（cmd/mdm/plugin_run_test.go）が使う作品情報プラグインである。
// fetch は標準エラーに 1 行書いてから作品の URL を http_send で取り、本文を題名、受け取った設定を
// key=value のタグにして返す。search は語が sleep なら眠り続け、fail なら改行なしで標準エラーに書いてから
// 誤りを返し、そうでなければ語を題名にした候補を返す。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"
	"unsafe"
)

//go:wasmimport vv http_send
func httpSend(ptr unsafe.Pointer, n uint32) int32

//go:wasmimport vv http_body
func httpBody(handle int32, ptr unsafe.Pointer, n uint32) int32

type request struct {
	Operation string            `json:"operation"`
	Query     string            `json:"query"`
	URL       string            `json:"url"`
	Settings  map[string]string `json:"settings"`
}

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	if req.Operation == "search" {
		switch req.Query {
		case "sleep":
			time.Sleep(time.Hour)
		case "fail":
			fmt.Fprint(os.Stderr, "no newline")
			reply(map[string]any{"error": map[string]string{"message": "failed on purpose"}})
			return
		}
		reply(map[string]any{"candidates": []map[string]string{{"url": "https://site.test/works/1", "title": req.Query}}})
		return
	}
	fmt.Fprintln(os.Stderr, "fetching", req.URL)
	send, _ := json.Marshal(map[string]string{"method": "GET", "url": req.URL})
	h := httpSend(unsafe.Pointer(&send[0]), uint32(len(send)))
	if h < 0 {
		reply(map[string]any{"error": map[string]string{"message": "http_send returned " + strconv.Itoa(int(h))}})
		return
	}
	tags := make([]string, 0, len(req.Settings))
	for k, v := range req.Settings {
		tags = append(tags, k+"="+v)
	}
	sort.Strings(tags)
	reply(map[string]any{"workInfo": map[string]any{"title": string(body(h)), "tags": tags}})
}

// body は応答の本文を、まず空の buffer で呼んで長さを知り、その大きさで呼び直して読む。
func body(handle int32) []byte {
	var probe [1]byte
	n := httpBody(handle, unsafe.Pointer(&probe[0]), 0)
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	httpBody(handle, unsafe.Pointer(&buf[0]), uint32(n))
	return buf
}

func reply(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		os.Exit(1)
	}
}
