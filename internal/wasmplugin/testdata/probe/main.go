//go:build wasip1

// The fixture selects search behavior by query and fetch behavior by ID, defaulting to echo.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
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
	ID        string            `json:"id"`
	Settings  map[string]string `json:"settings"`
}

// Keep allocations reachable to prevent reclamation.
var kept [][]byte

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	mode := req.Query
	if req.Operation == "fetch" {
		mode = req.ID
	}
	switch mode {
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
	case "cover":
		reply(map[string]any{"workInfo": map[string]any{"title": "cover", "cover": req.Settings["cover"]}})
	case "fail":
		fmt.Fprintln(os.Stderr, "reporting an error")
		reply(map[string]any{"error": map[string]any{"message": strings.Repeat("m", 400)}})
	case "exit":
		fmt.Fprintln(os.Stderr, "exiting with 3")
		os.Exit(3)
	default:
		echo(req)
	}
}

// echo mirrors search queries, or fetch IDs as titles with key=value setting tags.
func echo(req request) {
	if req.Operation == "search" {
		reply(map[string]any{"candidates": []map[string]string{{
			"id":     "q:" + req.Query,
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
	reply(map[string]any{"workInfo": map[string]any{"title": req.ID, "tags": tags}})
}

// fetchPages repeats HTTP requests while ignoring failures, returning the last successful body and
// response codes/metadata; fail then forces a plugin error.
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

// Read once with an empty buffer for length, then again with sufficient capacity.
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
