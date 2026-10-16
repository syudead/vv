//go:build wasip1

// e2eworks はブラウザテスト（web/e2e/work-info-plugins.e2e.ts）が使う作品情報プラグインである。
// ネットワークを使わず、決まったデータで答える。マニフェストは必須の秘密の設定 token を宣言し、
// fetch は token が e2eSecret のときだけ、それを受け取った印のタグを足す
// （specs/049-work-info-plugins/research.md R-12）。fetch は題名とタグに加えて品番・発売日・外部 ID
// つきの出演者・メーカーを返し、2 つ目の作品（e2e-2）は別の出演者だけを返す
// （specs/052-plugin-work-info-fields）。どの操作も受け取った設定を標準エラーに書く
// （specs/056-plugin-tooling）。マニフェストは web/e2e/work-info-plugin.mjs が
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
	ID        string            `json:"id"`
	Settings  map[string]string `json:"settings"`
}

func main() {
	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	// 受け取った設定を標準エラーに書く。設定画面の直近の実行のダイアログは、秘密の値を
	// [secret] に伏せて見せる（specs/056-plugin-tooling/research.md R-6）。
	fmt.Fprintf(os.Stderr, "%s: token=%s\n", req.Operation, req.Settings["token"])
	if req.Operation == "search" {
		reply(map[string]any{"candidates": []map[string]string{
			{"id": "e2e-1", "url": "https://works.example/w/e2e-1", "title": "プラグインe2e作品", "detail": "2024 · e2e"},
			{"id": "e2e-2", "url": "https://works.example/w/e2e-2", "title": "プラグインe2e別作品"},
		}})
		return
	}
	tags := []string{"プラグインe2eタグ"}
	if req.Settings["token"] == e2eSecret {
		tags = append(tags, "プラグインe2e鍵タグ")
	}
	reply(map[string]any{"workInfo": workInfo(req.ID, tags)})
}

// secondWork は別の出演者だけを返す作品の ID である。
const secondWork = "e2e-2"

func workInfo(id string, tags []string) map[string]any {
	info := map[string]any{
		"title": "プラグインe2e題名",
		"tags":  tags,
		"code":  "PE2E-052",
		"dates": []map[string]string{{"kind": "release", "date": "2024-03-01"}},
		"organizations": []map[string]any{
			{"role": "maker", "name": "プラグインe2eメーカー"},
		},
	}
	performer := credit("プラグインe2e出演者", "performer-1")
	if id == secondWork {
		performer = credit("プラグインe2e出演者二", "performer-2")
	}
	info["people"] = []map[string]any{performer}
	return info
}

func credit(name, id string) map[string]any {
	return map[string]any{
		"role":        "performer",
		"name":        name,
		"externalIds": []map[string]string{{"source": "e2eworks", "value": id}},
	}
}

func reply(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		os.Exit(1)
	}
}
