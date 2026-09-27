# Contract: 処理状況とシーク用サムネイルの状態

親 Issue #388。正本は [api/openapi.yaml](../../../api/openapi.yaml)
で、ここには変える箇所だけを書く。

## 1. `Processing.seekThumbnail`

`GET /api/processing` の応答と `/api/events` の `processing` イベントの `Processing` に、
`seekThumbnail`（integer、required）を足す。

| 項目 | 意味 |
| --- | --- |
| `probe` | 解析の残り（変えない） |
| `thumbnail` | 代表サムネイルの残り（説明を「サムネイルとシーク用プレビューの残り」から改める） |
| `seekThumbnail` | シーク用サムネイルの残り。`queued` / `running` の `seek_thumbnail` のうち登録済みの所在がある動画のもの |
| `preview` | 一覧用プレビューの残り（変えない） |

すべて 0 なら準備は終わっている（`seekThumbnail` を含む）。web の `processingRemaining` と
「準備中」の判定はこの合計を使う。処理状況の内訳は「解析・サムネイル・シーク用・プレビュー」の
4 列になる。

## 2. `seekThumbnailState` の意味

`Video.seekThumbnailState`（`GET /api/videos/{id}` にだけ入り、`seekThumbnailUrl` と同じ条件）の
値の意味を、[data-model.md §3](../data-model.md#3-状態遷移) に合わせて改める。schema（enum
`pending` / `done` / `failed`）は変えない。

| 値 | 意味 |
| --- | --- |
| `done` | 置き場がある |
| `pending` | 生成を待っている、または生成中。保存した状態が `done` なのに置き場が無いときは、サーバーが作り直しを積んで `pending` として返す |
| `failed` | 生成に失敗し、再試行の上限に達した |

`reprobeVideo`（`POST /api/videos/{id}/probe`）の説明: 読み取りの状態を `pending` に戻して
読み取りのジョブを積み、代表サムネイル・シーク用サムネイル・一覧用プレビューのうち `failed`
のものを `pending` に戻して、代表サムネイルとシーク用サムネイルは戻したときだけそのジョブを
積む（一覧用プレビューは読み取りの成功後に積まれる）。「シーク用プレビューの置き場が無いときは
サムネイルのジョブも積む」の記述は無くなる。
