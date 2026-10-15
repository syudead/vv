//go:build wasip1

// Command wikipedia は Wikipedia を引く作品情報プラグインである。契約は
// docs/how-to/work-info-plugins.md、使い方は README.md にある。
package main

import (
	"encoding/json"
	"os"
	"unsafe"

	"github.com/syudead/vv/plugins/wikipedia/wiki"
)

//go:wasmimport vv http_send
func httpSend(req unsafe.Pointer, reqLen uint32) int32

//go:wasmimport vv http_meta
func httpMeta(handle int32, buf unsafe.Pointer, bufCap uint32) int32

//go:wasmimport vv http_body
func httpBody(handle int32, buf unsafe.Pointer, bufCap uint32) int32

type request struct {
	Operation string            `json:"operation"`
	Query     string            `json:"query"`
	URL       string            `json:"url"`
	Settings  map[string]string `json:"settings"`
}

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fail("the request could not be read")
		return
	}
	switch req.Operation {
	case "search":
		lang, err := wiki.Lang(req.Settings["lang"])
		if err != nil {
			fail(err.Error())
			return
		}
		candidates, err := wiki.Search(get, lang, req.Query)
		if err != nil {
			fail(err.Error())
			return
		}
		reply(map[string]any{"candidates": candidates})
	case "fetch":
		info, err := wiki.Fetch(get, req.URL)
		if err != nil {
			fail(err.Error())
			return
		}
		reply(map[string]any{"workInfo": info})
	default:
		fail("unknown operation " + req.Operation)
	}
}

// get は GET を送り、200 の応答の本文を返す。
func get(url string) ([]byte, bool) {
	req, _ := json.Marshal(map[string]string{"method": "GET", "url": url})
	handle := httpSend(unsafe.Pointer(&req[0]), uint32(len(req)))
	if handle < 0 {
		return nil, false
	}
	var meta struct {
		Status int `json:"status"`
	}
	if json.Unmarshal(read(httpMeta, handle), &meta) != nil || meta.Status != 200 {
		return nil, false
	}
	return read(httpBody, handle), true
}

// read は空のバッファで長さを尋ね、その大きさのバッファに読み直す。
func read(fn func(int32, unsafe.Pointer, uint32) int32, handle int32) []byte {
	var probe [1]byte
	n := fn(handle, unsafe.Pointer(&probe[0]), 0)
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	fn(handle, unsafe.Pointer(&buf[0]), uint32(n))
	return buf
}

func fail(message string) {
	reply(map[string]any{"error": map[string]any{"message": message}})
}

func reply(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		os.Exit(1)
	}
}
