---
source: specs/054-auto-dup/quickstart.md
sourceHash: fa155eb0d77aae8d33a10ff22564f736440d34e30cf80f37f036e99d3a672103
---

# クイックスタート: 実際のモデルで重複を判定する {#quickstart-judging-duplicates-with-a-real-model}

この手順は、偽の分類器を使う自動テストでは示せないことを示す: 実際の Clef のモデルが [R-3](research.md#r-3-the-question-wording-and-two-fixed-thresholds) のしきい値で実際の重複を似ているだけのものから分けること、そして 4 つの受け入れ条件が端から端まで成り立つことである。

## 前提条件 {#prerequisites}

- このブランチから動かした VVMDM ([running-vv.md](../../docs/how-to/running-vv.md))。
- Ollama として届く Clef のモデルで、Settings の Ollama にモデル `clef` で設定してある。Ollama がなければ、`go run ./scripts/clefrelay` が Workers AI へ中継する (使い方は [`scripts/clefrelay/main.go`](../../scripts/clefrelay/main.go) の先頭にある)。
- 次を持つメディアフォルダ: 実際の動画 `A`、低い解像度の同じ動画 `B` (`ffmpeg -i A.mp4 -vf scale=-2:360 B.mp4`)、そして同じ長さで、最初の数秒が `A` のもので残りが別の映像である動画 `C`。
- Settings で重複の自動判定をオンにしてある。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| スキャンし、取り込みが終わるまで待つ | `A` と `B` がバージョンの印付きの 1 枚のカードとして現れる。動画ページは 2 つのバージョンを一覧にする。代表は `A` である | 1 |
| Duplicates を開く | `A` と `C` は一覧になく、`C` の動画ページはバージョンを示さない | 2 |
| `A` の動画ページで `B` をバージョンから外し、もう一度スキャンする | `B` は再び自分のカードを持つ。Duplicates は `A` と `B` を一覧にし、スキャンの後も一覧に残る | 3 |
| 設定をオフにし、`A` の別の解像度のコピーである `D` を加え、スキャンする | Duplicates は `D` を `A` とともに一覧にし、何もまとめられない | 4 |
| モデルを止め、設定をオンにし、もう 1 つのコピー `E` を加え、スキャンする | Duplicates は `E` の組を一覧にする。モデルを再び起動し、再試行の時刻が過ぎた後、`E` は `A` のバージョンに加わる | エッジケース: AI に届かない |

各組の確率をサーバーのログから PR に記録する。実際のファイルで同じ組が 0.2 と 0.8 の間に入ったら、単位がマージされる前に、しきい値は Plan に差し戻される。
