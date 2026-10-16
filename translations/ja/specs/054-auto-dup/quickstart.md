---
source: specs/054-auto-dup/quickstart.md
sourceHash: 5bcd53eae5dbbaa52e90f35b587bb84e2805cf5ba189e770659b3271255c7f1d
---

# クイックスタート: 実際のモデルで重複を判定する {#quickstart-judging-duplicates-with-a-real-model}

この手順は、偽の分類器を使う自動テストでは示せないことを示す: 実際のファイルで、[R-1](research.md#r-1-the-fingerprints-decide-first-the-model-is-asked-only-when-a-section-differs) のフレームの規則が再エンコードしたものを自分だけでまとめること、既定の Clef のモデルが [R-3](research.md#r-3-the-question-wording-and-two-fixed-thresholds) のしきい値で、ある区間を差し替えたコピーを退けること、そして 4 つの受け入れ条件が端から端まで成り立つことである。

## 前提条件 {#prerequisites}

- このブランチから動かした VVMDM ([running-vv.md](../../docs/how-to/running-vv.md))。
- Ollama として届く Clef のモデルで、Settings の Ollama に既定のモデル `clef-flash` で設定してある。Ollama がなければ、`go run ./scripts/clefrelay` が Workers AI へ中継する (使い方は [`scripts/clefrelay/main.go`](../../scripts/clefrelay/main.go) の先頭にある)。
- 数分の実際の動画 `A` と、次を持つメディアフォルダ:
  - `B`: 低い解像度の `A` (`ffmpeg -i A.mp4 -vf scale=-2:360 B.mp4`)。
  - `C`: 途中のおよそ 10 分の 1 を同じ長さの別の映像に差し替えた `A`。フレームの距離の中央値により、それでも候補になる。
- Settings で重複の自動判定をオンにしてある。

## 手順 {#steps}

| 手順 | 期待する結果 | 受け入れ条件 |
| --- | --- | --- |
| スキャンし、取り込みが終わるまで待つ | `A` と `B` がバージョンの印付きの 1 枚のカードとして現れる。動画ページは 2 つのバージョンを一覧にする。代表は `A` である。ログは、フレームの規則がリクエストなしで決めたことを示す | 1 |
| Duplicates を開く | `A` と `C` は一覧にない。ログは、`C` について 0.2 以下の確率の 1 回のリクエストを示し、`C` は自分のカードを持つ | 2 |
| `A` の動画ページで `B` をバージョンから外し、もう一度スキャンする | `B` は再び自分のカードを持つ。Duplicates は `A` と `B` を一覧にし、スキャンの後も一覧に残る | 3 |
| 設定をオフにし、`A` の別の解像度のもう 1 つのコピーである `D` を加え、スキャンする | Duplicates は `D` を `A` とともに一覧にし、何もまとめられない | 4 |
| モデルを止め、設定をオンにし、`C` と同じように別の区間を差し替えて作った `E` を加え、スキャンする | `D` はリクエストなしで `A` のバージョンに加わる。Duplicates は `E` の組を残す。モデルを再び起動し、再試行の時刻が過ぎた後、組は Duplicates から消える | エッジケース: AI に届かない |

各組の規則の結果と確率をサーバーのログから PR に記録する。規則がモデルに送った組が 0.2 と 0.8 の間に入ったら、または実際の再エンコードが少しでもモデルに送られたら、単位がマージされる前に、規則かしきい値は Plan に差し戻される。
