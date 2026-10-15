---
source: specs/053-ai-tag-generation/quickstart.md
sourceHash: f70975e36ccbf43e26509ca692a63c62a301b1e651c89c95849777cf080e5d98
---

# クイックスタート: 実際のモデルでタグを作成する {#quickstart-tag-creation-with-a-real-model}

この手順は、既定の生成モデルが実際の動画から許容できる時間で使えるタグを作成することを確かめる。自動の検査は偽の生成器を使うので、これを確かめられない。

## 前提 {#prerequisites}

- [running-vv.md](../../docs/how-to/running-vv.md) のとおりに用意した、feature ブランチのビルド。
- 判定のモデルと `gemma4:e4b` を持つ Ollama。または、Ollama のないコンテナでは、生成の中継を持つ `go run ./scripts/clefrelay` ([R-2](research.md#r-2-the-ollama-provider-uses-apichat-with-a-vision-model))。
- 主題がはっきり異なる 3 本の短い動画を含むメディアフォルダ。そのうち 1 本は猫の動画。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| Settings、Ollama: "Save and test connection" | 利用できる。Ollama から `gemma4:e4b` を取り除くと、利用できず、メッセージがそれを名指しする | — |
| Settings、Auto-tagging: "Use auto-tagging" と "Tag new videos automatically" をオンにし、"Create new tags" はオンのままにする。空のライブラリをスキャンする | 各動画が種類付きの仮のタグを得て、タグの画面の仮の絞り込みがそれらを挙げる。ログで動画 1 本あたりの秒数を記録する | 1 |
| タグ `猫` を作成し、猫の動画を開き、"Suggest tags" | `猫` が動画に付き、`ねこ` や `ネコ` のようなタグは作成されない | 2 |
| タグの画面で作成されたタグを 1 つ却下し、その動画で "Suggest tags" | その名前のタグは戻ってこない | 3 |
| "Create new tags" をオフにし、4 本目の動画を加える | 新しいタグは作成されず、既存のタグが判定される | 4 |
| Ollama を止め、5 本目の動画を加え、Ollama を起動する | 動画の行は `failed` になり、その後、何も操作しなくても約 10 分後に判定される | エッジケース |
