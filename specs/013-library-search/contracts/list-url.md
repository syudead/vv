# Contract: 一覧の条件を表す URL

親 Issue: #195。Plan: [plan.md](../plan.md)。

ライブラリ（`/`）、フォルダ画面の最上位（`/folders`）、各フォルダ
（`/folders/{rootId}/{段}…`）は、同じクエリパラメータで一覧の条件を表す（要件 16・18）。
フォルダの位置の表し方は変えない（[specs/011-folder-browser/plan.md](../../011-folder-browser/plan.md)
Structural Decisions 3）。見た目と操作の配置は、design 工程の `ui-design.md` が決める。

## 1. パラメータ

| 名前 | 値 | 省略時 |
| --- | --- | --- |
| `q` | 検索語。前後の空白を落とし、100 符号位置で切る（サーバーの `[]rune` の数え方と同じ） | 検索しない |
| `watch` | `unwatched` \| `inProgress` \| `watched` | すべて |
| `playable` | `1` | 絞らない |
| `sort` | [list-api.md §3](list-api.md#3-videosort-の値) の値 | 端末に保存した並べ替え（既定 `addedDesc`） |
| `seed` | 1 以上 2147483647 以下の整数 | `sort=random` のときは画面が作って足す |

- 既定の値は URL に書かない（`watch=all`、`playable` の偽）。`sort` は、条件を一度でも変えたあとは既定の
  `addedDesc` でも書く。省くと「端末に保存した並べ替え」の意味になり、戻る/進むで前の並べ替えに
  戻れないためである。条件を変える前の `/` は、そのまま `sort` を書かない。同じ一覧が2つの URL を持たない
  ようにし、[listSnapshot](../../../web/src/api/listSnapshot.ts) の鍵の一致を保つ。
- 解釈できない値（未知の `sort`・`watch`、数でない `seed`、`playable` の `1` 以外）は
  既定として扱い、誤りを出さない。今の形式の URL（`?q=…&sort=addedDesc`）は同じ意味の
  一覧になる（Edge Case「古い URL」）。
- `sort=random` で `seed` が無いか壊れているときは、画面が新しい `seed` を作り、履歴を
  増やさずに URL へ書き足す。以後、再読み込み・共有・戻る/進むで同じ並びになる。
- 「並べ直す」は新しい `seed` を作って URL を変える。

## 2. フォルダ画面での意味

- `q` が空でないとき、そのフォルダと配下すべてを対象に検索する（`listFolderVideos` を
  `scope=subtree` で呼ぶ）。最上位（`/folders`）では `listVideos` で全体を検索する。
- `q` が空で `watch`・`playable` だけがあるときは、直下の動画だけを絞る（`scope=direct`）。
  子フォルダのカードはそのまま出す（要件 20）。
- 最上位（`/folders`）で `q` が空のときは、登録フォルダのカードだけを出し、`watch`・
  `playable` は URL に残っても効かない。それらの操作を出すか隠すかは `ui-design.md` が
  決める。
- 「条件を解除」は `q`・`watch`・`playable` を消し、`sort`・`seed` とフォルダの位置は残す
  （要件 21）。

## 3. 履歴

- 視聴状態・再生可否・並べ替え・向き・並べ直す・条件を解除は、それぞれ履歴を1つ増やす
  （受け入れ条件 12 の戻る/進む）。今の画面は並べ替えと検索語を `replace` で書き換えて
  おり、戻るで前の条件に戻れない。受け入れ条件 12 を満たすため、これを改める。
- 検索語の入力は、検索欄にフォーカスが入ってから外れるか Esc で抜けるまでの一続きの入力で
  履歴を1つだけ増やす。続けて打った文字ごとには増やさない。
- 端末に保存するのは並べ替え（`sort`）だけで、検索語・絞り込み・`seed` は保存しない。
  ランダムを保存した端末で URL に `sort` が無いときは、新しい `seed` で開く。
