# Contract: 動画詳細画面のための API の差分

正本は [api/openapi.yaml](../../../api/openapi.yaml) である。ここに書くのは、この feature が
足す項目・経路・誤りの意味だけである。各実装単位が、自分の担当分をそこへ移す。

`Error` の形（`code`・`message`）と、既存の経路の意味は変えない。`Error.code` の enum には
`probe_not_failed`・`open_unavailable`・`file_missing` の 3 つを足す。403 には既存の
`forbidden` を使う。

## `Video` の追加項目

どちらも任意項目で、`GET /api/videos/{id}` の応答にだけ入る。次の応答には入れない。

- `GET /api/videos`
- `GET /api/folders/{rootId}/videos`
- 関連動画

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `location` | object | 代表の所在。登録フォルダの下に所在が無い動画はそもそも 404 なので、この応答では常に入る |
| `location.path` | string | 代表の所在の絶対パス。サーバーから見たパスで、コンテナ内ならコンテナ内のパスになる |
| `location.openable` | boolean | 次の両方を満たすとき `true`。要求元がループバックであること、サーバーが既定アプリを起動できる環境であること |
| `seekThumbnailState` | `pending` \| `done` \| `failed` | シーク用プレビューの状態。既存の `seekThumbnailUrl` と同じ条件（読み取り済み・長さが正・映像コーデックあり・内容鍵あり）のときだけ入る |

代表の所在の選び方は、既存の `GetVideo` と同じとする。登録フォルダの下にある所在のうち、
パスが最小のものである。

`seekThumbnailState` の導き方は次のとおり。

- `done`：置き場（`thumbnails/seek/<prefix>/<contentKey>/`）が存在する
- `pending`：置き場が無く、`thumbnail_state` が `pending` であるか、その動画の `thumbnail`
  ジョブが `queued`・`running` である
- `failed`：それ以外（ジョブが `failed`、またはジョブの行が保持期間を過ぎて消えている）

## 関連動画: `GET /api/videos/{id}/related`

応答は 200 で、`RelatedVideos` を返す。

```yaml
RelatedVideos:
  type: object
  required: [items]
  additionalProperties: false
  properties:
    items:
      type: array
      maxItems: 20
      items: { $ref: "#/components/schemas/Video" }
    nextId:
      type: integer
      format: int64
      description: 同じディレクトリで自然順の次の動画。無ければ省く
```

`items` の各 `Video` には、一覧と同じく `progress` を付ける。

並びは次の順で、最大 20 件とする。

1. 代表の所在と同じディレクトリの直下にある動画のうち、ファイル名の自然順
   （`domain.CompareNatural`）でこの動画より後のもの
2. 同じディレクトリの直下にある、この動画より前のもの
3. 1 と 2 に入らなかった動画を、追加日時の差の小さい順に並べたもの。差が同じなら `id` の
   大きい方を先にする

補足:

- 同じディレクトリに所在を 2 つ持つ動画は、パスの小さい方の所在のファイル名で並べ、
  1 件として数える。
- 1 と 2 の比較に使うこの動画自身のファイル名は、代表の所在のものである。
- この動画自身は含めない。登録フォルダの下に所在の無い動画も含めない。
- `nextId` は 1 の先頭の `id` で、1 が空なら省く。画面はこれを「次の動画」として使う。
  - 却下: 画面が `items[0]` を次の動画とみなす案。1 が空のとき `items[0]` は先行や
    追加日時の近い動画になり、要件 14「次の動画が無いときは『もう一度見る』だけ」を満たせない。
    また、関連動画の各項目は `location` を持たないので、画面でディレクトリを比べることも
    できない。

誤り:

| 状態 | 状況 | `code` |
| --- | --- | --- |
| 404 | 知らない id、または登録フォルダの下に所在が無い | `not_found` |

## 読み取りのやり直し: `POST /api/videos/{id}/probe`

要求の本文は無い。

- 成功は 202 で、更新後の `Video`（`probeState: pending`）を返す。
- サーバーは 1 つの取引の中で次を行う（`RequeuePreviewRepair` と同じ形）。
  - `where probe_state='failed'` を付けて、`probe_state` を `pending` に戻し、`probe_error` を
    消す。更新が 0 行なら 409 とする。
  - `thumbnail_state` が `done` でなければ `pending` に戻す。
  - `preview_state` が `failed` なら `pending` に戻す。
  - `probe` ジョブを積む。`thumbnail_state` を戻したときは `thumbnail` ジョブも積む。
    どちらも、その種類の終わった行を消してから `on conflict … do nothing` で挿入する。
- プレビューのジョブは、読み取りの成功後に既存の `probeHandler` が積む。

| 状態 | 状況 | `code` |
| --- | --- | --- |
| 404 | 知らない id、または登録フォルダの下に所在が無い | `not_found` |
| 409 | `probeState` が `failed` でない（読み取り中・読み取り済み） | `probe_not_failed` |

画面は 409 を「すでにやり直し中」とみなし、動画を取り直して段階表示へ移る。

## ファイルを開く: `POST /api/videos/{id}/open`

要求の本文は無い。パスは受け取らない。

- 成功は 204 である。代表の所在を、サーバーの PC の既定アプリで開く子プロセスを起動した
  時点で返す。
- アプリが開いたかどうかは確かめない。

判定は表の上から順に行い、最初に当たったものを返す。

| 状態 | 状況 | `code` |
| --- | --- | --- |
| 404 | 知らない id、または登録フォルダの下に所在が無い | `not_found` |
| 409 | サーバーが既定アプリを起動できない環境である（要求元を問わない） | `open_unavailable` |
| 403 | 要求元がループバックでない、または `Host` がループバックの名前（`localhost`・`127.0.0.1`・`[::1]`）でない | `forbidden`（既存） |
| 409 | 代表の所在にファイルが無い（移動・削除された） | `file_missing` |
| 500 | 子プロセスを起動できなかった | `internal` |

既存の POST の同一オリジン確認（`mutationBoundary`）は、この経路にもそのまま掛かる。
`Host` の確認は、それを DNS rebinding で通り抜けられないようにするためである。

403 と 409 `open_unavailable` の判定は、`location.openable` と同じ条件で行う。
画面が `openable: false` でリンクを出さないのは、この 2 つを利用者に見せないためである。
