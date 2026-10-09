---
source: specs/043-watch-history/contracts/screen-api.md
sourceHash: cbb7898d05f130b967048fa7b183ebbee3f3aa235e6b539a497fc8c99da3d69f
---

# 契約: 画面の API の変更 {#contract-screen-api-changes}

親 Issue: #792。正本: [api/openapi.yaml](../../../api/openapi.yaml)。このファイルは加えるフィールド、ルート、スキーマだけを挙げる。`task generate` が `internal/httpapi/gen/` と `web/src/api/gen/` を生成し直す。外部 API (`api/external-v1.yaml`) と MCP は変わらない (範囲外)。

3 つのルートはすべて `/api/*` の既定により所有者専用である ([research.md R-7](../research.md#r-7-watch-history-is-one-more-owner-only-screen-and-route-with-the-existing-gate-rules)) ので、`accessRoutes` には何も加わらず、`TestAccessClassificationMatchesOpenAPISecurity` は操作の既定の `security` のまま通り続ける。

## `PUT /api/videos/{id}/progress` の `playbackId` {#playbackid-on-put-apivideosidprogress}

保存は省略できるフィールドを 1 つ得る ([research.md R-2](../research.md#r-2-one-entry-per-playback-identified-by-a-client-generated-playback-id))。

```yaml
ProgressUpdate:
  required: [positionMs]
  properties:
    positionMs: { type: integer, format: int64, minimum: 0 }
    playbackId:
      type: string
      minLength: 36
      maxLength: 36
      description: |
        The client's id for this viewing (RFC 4122 text form). The first save
        with an id adds one watch history entry for the video; later saves with
        the same id add nothing. Absent before playback has started.
```

| 条件 | 応答 |
| --- | --- |
| `playbackId` がない | 今と同じ。エントリはない |
| `playbackId` があり、動画が `content_key` を持つ | 今と同じく `200 Progress`。応答の後にはエントリがあり、動画の実際のタイトルとサーバーの時刻を持つ |
| `playbackId` があるが 36 文字の形式ではない | `400 invalid_request` |
| 動画の `content_key` が空 | `200 Progress`。エントリはない |

`Progress` 応答は変わらない。本文の上限 (1 KiB) は引き続き適用される。

## `GET /api/watch-history` {#get-apiwatch-history}

所有者の視聴履歴を新しい順に返す ([data-model.md、Store operations](../data-model.md#store-operations-playbackstore))。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `cursor` | query | string | いいえ | 前の応答の `nextCursor`。不透明 |
| `limit` | query | integer、1 から 200 | いいえ | ページごとの項目数。既定は 60 |

**応答**: `200 WatchHistoryPage`。

```yaml
WatchHistoryPage:
  required: [items]
  properties:
    items:      { type: array, items: { $ref: WatchHistoryEntry } }
    nextCursor: { type: string }   # present only when more entries follow

WatchHistoryEntry:
  required: [id, playedAt, title]
  properties:
    id:       { type: integer, format: int64 }
    playedAt: { type: string, format: date-time }   # when the viewing started
    title:    { type: string }                       # the title at that time
    video:
      $ref: Video   # the video now in the library with the played content;
                    # absent when there is none (not playable from the history)
```

`video` は一覧の項目と同じ形で、`progress`、`tags`、`favorite` は所有者に対するものと同じく埋まっているので、画面はそれを開き、その状態を示せる。あるとき、`video.title` は今のタイトルであり、画面はそれを表示する。`title` は動画のないエントリのためのものである。

| ステータス | `code` | 条件 |
| --- | --- | --- |
| `400` | `invalid_request` | `limit` が範囲外 |
| `400` | `invalid_cursor` | `cursor` を読めない |

## `DELETE /api/watch-history/{id}` {#delete-apiwatch-historyid}

エントリを 1 つ削除する ([research.md R-6](../research.md#r-6-deleting-entries-is-a-plain-delete-a-vanished-entry-answers-404-and-the-screen-reloads))。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `id` | path | integer、最小 1 | はい | `WatchHistoryEntry.id` |

**応答**: 本文のない `204`。

| ステータス | `code` | 条件 |
| --- | --- | --- |
| `404` | `not_found` | その id のエントリがない (すでに別の場所で削除された) |

## `DELETE /api/watch-history` {#delete-apiwatch-history}

すべてのエントリを削除する。**応答**: 本文のない `204`。履歴がすでに空のときも同じ。確認は画面が受け持つ。

## クライアントでの使い方 {#client-use}

| 変更 | 詳細 |
| --- | --- |
| `web/src/api/client.ts` の `saveProgress(id, positionMs, playbackId?)` と `beaconProgress(id, positionMs, playbackId?)` | 与えられたときに `playbackId` を送る。動画ごとの送信キューと `recordSavedProgress` は変わらない |
| `useProgressSaving` (`web/src/player/useProgressSaving.ts`) | 今の動画 id の再生 id を持つ。`markPlayed()` は最初の `play` でそれを作り、即時の保存を 1 回送る。動画 id が変わると id を捨てる |
| `VideoPlayer` | ページが `markPlayed()` を呼べるように `play` イベントを知らせる (`onPlay`) |
| `web/src/api/history.ts` (新規) | `listWatchHistory({ cursor, limit })`、`deleteWatchHistoryEntry(id)`、`clearWatchHistory()` |
| 履歴の画面 | `204` で行を消す。`404` でメッセージなしに最初のページから読み直す。それ以外の失敗では行を残し、トーストを出す (`errorText`) |
