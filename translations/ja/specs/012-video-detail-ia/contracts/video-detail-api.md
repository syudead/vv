---
source: specs/012-video-detail-ia/contracts/video-detail-api.md
sourceHash: bf7e252111c58e0a65f1c36f877594536cd2f0fec0e5da49c76a8220d32714f1
---

# 契約: 動画詳細画面のための API の変更 {#contract-api-changes-for-the-video-detail-screen}

正: [api/openapi.yaml](../../../api/openapi.yaml)。この文書は、この機能が加えるフィールド、エンドポイント、エラーの意味だけを扱う。各実装単位は、自分の部分をスキーマへ移す。

`Error` の形 (`code`、`message`) と既存のエンドポイントの意味は変わらない。`Error.code` の enum に 3 つの値を加える: `probe_not_failed`、`open_unavailable`、`file_missing`。403 は既存の `forbidden` を使う。

## `Video` に加えるフィールド {#added-video-fields}

どちらのフィールドも省略可能で、`GET /api/videos/{id}` の応答にだけ現れる。次の応答にはない:

- `GET /api/videos`
- `GET /api/folders/{rootId}/videos`
- 関連動画

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `location` | object | 代表の場所。登録したフォルダの下に場所がない動画はいずれにせよ 404 なので、この応答には必ずある |
| `location.path` | string | 代表の場所の絶対パス。サーバーから見たもの (コンテナの中ではコンテナの中のパス) |
| `location.openable` | boolean | 次のすべてを満たすとき `true`: 要求がループバックから来ている、`Host` がループバックの名前である、サーバーが既定のアプリケーションを起動できる場所で動いている (「ファイルを開く」の 403 と 409 `open_unavailable` と同じ条件) |
| `folder` | object | 代表の場所を持つフォルダ (`rootId`、`path`、`rootName`)。プレーヤー画面の見出しのパンくずに使う。`rootName` は登録したフォルダの表示名 (`FolderSummary.name` と同じ規則) で、この応答にだけ現れる。場所が登録したどのフォルダにもないときは省く。`folder` 自体は一覧の応答にも現れる (013 の list-api.md) |
| `seekThumbnailState` | `pending` \| `done` \| `failed` | シーク用プレビューの状態。既存の `seekThumbnailUrl` と同じ条件 (プローブ済み、正の長さ、映像コーデックがある、内容の鍵がある) のときだけある |

代表の場所は既存の `GetVideo` と同じく選ぶ: 登録したフォルダの下で、パスが最も小さい場所である。

`seekThumbnailState` は次のように導く:

| 値 | 条件 |
| --- | --- |
| `done` | 保存先のディレクトリ (`thumbnails/seek/<prefix>/<contentKey>/`) がある |
| `pending` | 保存先のディレクトリがなく、`thumbnail_state` が `pending` か、動画の `thumbnail` ジョブが `queued` または `running` である |
| `failed` | それ以外 (ジョブが `failed`、または保持期間の後にジョブの行が消えた) |

サムネイルのジョブが代表のサムネイルより前の段階で最終的に失敗すると、同じトランザクションで `thumbnail_state` も `failed` になる (plan の Structural Decisions 13)。そのため、`failed` のジョブが `thumbnail_state` を `pending` のまま残すことはない。

## 関連動画: `GET /api/videos/{id}/related` {#related-videos-get-apivideosidrelated}

応答は 200 で `RelatedVideos` を返す。閲覧者が見てよい他のすべての動画を、この動画に近い順に並べたものの 1 ページである。順序、ページ送り、前後の動画は [関連動画と前後の動画](../../../docs/design-docs/related-videos.md) にあり、スキーマは [api/openapi.yaml](../../../api/openapi.yaml) の `RelatedVideos` である。

`items` の各 `Video` は、一覧と同じく `progress` を持つ。

## プローブし直す: `POST /api/videos/{id}/probe` {#probe-again-post-apivideosidprobe}

要求に本文はない。

- 成功は 202 で、更新した `Video` (`probeState: pending`) を返す。
- サーバーは 1 つのトランザクションで次をする (`RequeuePreviewRepair` と同じ形):
  - `where probe_state='failed'` で、`probe_state` を `pending` に戻し、`probe_error` を消す。更新した行が 0 なら 409 を返す。
  - `thumbnail_state` が `done` でなければ、`pending` に戻す。
  - `thumbnail_state` が `done` でも、シーク用プレビューの保存先のディレクトリがなければ、状態はそのままにして `thumbnail` ジョブを積む。既存の `thumbnailHandler` は、代表のサムネイルがあればそれを作り直さず、シーク用プレビューだけを作る。
  - `preview_state` が `failed` なら、`pending` に戻す。
  - `probe` ジョブを積む。`thumbnail_state` を戻したとき、またはシーク用プレビューの保存先のディレクトリがないときは、`thumbnail` ジョブも積む。どちらも、先にその種類の終わった行を削除し、それから `on conflict … do nothing` で挿入する。
- プレビューのジョブは、プローブが成功した後に既存の `probeHandler` が積む。
- `probe_state='failed'` は、その動画のプローブのジョブが終わったことを意味する。最終的な失敗は、ジョブを `failed` にするのと同じトランザクションで記録するからである (plan の Structural Decisions 13)。そのため、ここで積む新しいジョブが、まだ動いている古いジョブとの重複防止で飛ばされることはない。

| ステータス | 状況 | `code` |
| --- | --- | --- |
| 404 | 不明な id、または登録したフォルダの下に場所がない | `not_found` |
| 409 | `probeState` が `failed` でない (プローブ中またはプローブ済み) | `probe_not_failed` |

画面は 409 を「すでにやり直し中」として扱い、動画を取得し直して段階の表示へ移る。

## ファイルを開く: `POST /api/videos/{id}/open` {#open-file-post-apivideosidopen}

要求に本文はなく、パスも受け付けない。

- 成功は 204 で、サーバーの PC の既定のアプリケーションで代表の場所を開く子プロセスが起動した時点で返す。
- アプリケーションが実際に開いたかは確かめない。

確認は表の上から順に進め、最初に当てはまったものを返す。

| ステータス | 状況 | `code` |
| --- | --- | --- |
| 404 | 不明な id、または登録したフォルダの下に場所がない | `not_found` |
| 409 | サーバーが既定のアプリケーションを起動できない場所で動いている (コマンドがない、ディスプレイがない。要求元にかかわらない) | `open_unavailable` |
| 403 | 要求がループバックから来ていない、または `Host` がループバックの名前 (`localhost`、`127.0.0.1`、`[::1]`) でない | `forbidden` (既存) |
| 409 | 代表の場所にファイルがない (移動または削除された) | `file_missing` |
| 500 | 子プロセスを起動できなかった | `internal` |

POST に対する既存の同一オリジンの確認 (`mutationBoundary`) は、このエンドポイントにもそのまま適用する。`Host` の確認は、DNS リバインディングでそれをすり抜けられないようにする。

403 と 409 `open_unavailable` は、`location.openable` と同じ条件で決める。画面は `openable: false` のときリンクを示さないので、利用者がこの 2 つを見ることはない。
