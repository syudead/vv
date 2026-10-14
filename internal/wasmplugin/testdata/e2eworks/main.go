//go:build wasip1

// e2eworks はブラウザテスト（web/e2e/work-info-plugins.e2e.ts）が使う作品情報プラグインである。
// ネットワークを使わず、決まったデータで答える。マニフェストは必須の秘密の設定 token を宣言し、
// fetch は token が e2eSecret のときだけ、それを受け取った印のタグを足す
// （specs/049-work-info-plugins/research.md R-12）。マニフェストは web/e2e/work-info-plugin.mjs が
// ビルドしたモジュールに足す。
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// e2eSecret はテストが設定画面で入れる秘密の値である。
const e2eSecret = "e2e-secret-value"

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
		reply(map[string]any{"candidates": []map[string]string{
			{"url": "https://works.example/w/e2e-1", "title": "プラグインe2e作品", "detail": "2024 · e2e"},
			{"url": "https://works.example/w/e2e-2", "title": "プラグインe2e別作品"},
		}})
		return
	}
	tags := []string{"プラグインe2eタグ"}
	if req.Settings["token"] == e2eSecret {
		tags = append(tags, "プラグインe2e鍵タグ")
	}
	reply(map[string]any{"workInfo": map[string]any{"title": "プラグインe2e題名", "tags": tags}})
}

func reply(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		os.Exit(1)
	}
}
