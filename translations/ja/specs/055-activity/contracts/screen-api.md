---
source: specs/055-activity/contracts/screen-api.md
sourceHash: e2afa8af79da21bf361b54691e9ff9dff2b3464a8eb081f9b42a0963dc515324
---

# 契約: アクティビティの画面の API {#contract-screen-api-for-activity}

正本: [`api/openapi.yaml`](../../../api/openapi.yaml)。この文書は、この機能が加えるか変えるものだけを持つ。エラーの形は [023 error-api.md](../../023-english-i18n/contracts/error-api.md) である。ここのすべての操作は所有者だけ (既定の `security`) なので、ゲストは `401` を受け取り、`internal/httpapi` の `accessRoutes` はどれも載せない (受け入れ条件 8)。`api/external-v1.yaml` は変わらない。

## `GET /api/activity` {#get-apiactivity}

`operationId: listActivity`。項目の 1 ページを新しい順に返し、タブが示す件数も返す ([R-9](../research.md#r-9-keyset-paging-by-id-newest-first-filtered-by-status))。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `status` | query | `running` \| `attention` \| `done` | いいえ | その状態の項目だけ。ないときはすべて |
| `feature` | query、繰り返し可 | `import` \| `scan` \| `duplicate_check` \| `auto_tagging` \| `work_info` \| `tags` \| `versions` \| `media_folders` \| `organize` | いいえ | それらの機能の項目だけ ([機能](#features))。ないときはすべて |
| `decider` | query | `rule` \| `ai` \| `owner` | いいえ | その決定者の項目だけ |
| `since` | query | date-time | いいえ | その時刻以降に作られた項目だけ |
| `q` | query | string, 1–200 文字 | いいえ | 対象のラベルがそれを含む項目だけ。大文字と小文字を区別しない |
| `cursor` | query | string | いいえ | 前のページの `nextCursor` |
| `limit` | query | integer, 1–100 | いいえ | 既定は 50 |

**応答**: `ActivityPage {items: ActivityEntry[], nextCursor?, counts: {running, attention, done}, features: {<feature>: number}}` とともに `200`。`counts` は `status` 以外のすべての絞り込みを適用するので、Status のメニューは各選択肢が何件を並べるかを示す。`features` は Feature のメニューのために、`feature` 以外のすべての絞り込みを適用する。フッターとサイドバーは、絞り込みのない要求の `counts` を読む。

| `ActivityEntry` のフィールド | 型 | 意味 |
| --- | --- | --- |
| `id` | integer | 項目 |
| `kind`, `status`, `reason?`, `outcome?`, `decider`, `probability?`, `detail?` | [data-model.md](../data-model.md#activity_entries) のとおり | |
| `subject` | `{kind, items: {label, videoId?}[]}` | 鍵の順に鍵ごとに 1 つの要素 (組なら 2 つ、それ以外は 1 つ)。`label` はその鍵について書いたときのまま (`subject_label`、`subject_label_b`)、`videoId` は閲覧者が開ける今の動画で、鍵に動画がないときはない ([R-10](../research.md#r-10-the-subject-is-a-content-key-with-a-label-kept-at-write-time)) |
| `candidate` | boolean | 組について: その候補がまだ Duplicates にある |
| `stage?` | `probe` \| `thumbnail` \| `seek_thumbnail` \| `preview` \| `fingerprint` | 失敗した `import` について: 諦めた段階。そのとき `detail` はそのメッセージを持つ |
| `retryAt?` | date-time | 失敗した `version_judgement` について: そのバックオフが再び試す時刻 |
| `feature` | string | 項目の機能 ([機能](#features)) |
| `progress?` | `{stage, done, total}` | `waiting` を過ぎた `running` の `import` について: 今動いている段階と、`total` の 5 段階のうち終わった数 |
| `undoes?` | `{kind, outcome?, tagName?}` | `undo` の項目について: 取り消された項目の種類と結果、およびタグごとの取り消しならそのタグ |
| `tags?` | `{id, name, suggested, probability, undone}[]` | `auto_tag` について |
| `retryRun`, `undoable` | boolean | 行が再試行と取り消しを示すか ([data-model.md、規則](../data-model.md#rules)) |
| `undone` | boolean | 所有者が取り消した |
| `createdAt`, `updatedAt` | date-time | |

| ステータス | `code` | 条件 |
| --- | --- | --- |
| `400` | `invalid_request` | 未知の `status`、`feature`、`decider`、不正な形のカーソルや `since`、範囲外の `q` や `limit` |

### 機能

各種類は 1 つの機能に属する。機能は Feature の絞り込みが示す単位である。

| `feature` | 種類 |
| --- | --- |
| `import` | `import` |
| `scan` | `scan` |
| `duplicate_check` | `version_judgement` |
| `auto_tagging` | `auto_tag` |
| `work_info` | `work_info_fetch`, `work_info_apply` |
| `tags` | `tag_attach`, `tag_detach`, `tag_keep`, `tag_reject` |
| `versions` | `bundle`, `unbundle`, `make_representative`, `dismiss` |
| `media_folders` | `media_folder_remove` |
| `organize` | `organize_undo` |

`undo` の項目は、それが取り消した項目の機能に属する。

## `POST /api/activity/{id}/retry` {#post-apiactivityidretry}

`operationId: retryActivity`。項目の処理をもう一度始め、`202` を応答する。新しい実行は新しい項目であり、それがこの項目を置き換える。`version_judgement` は自分の項目を保ち、その項目が `running` に戻る ([R-4](../research.md#r-4-three-statuses-with-a-reason-and-a-newer-run-closes-an-older-problem))。`scan` の項目は、`done` で終わる次のスキャンが閉じる。

| 種類 | 始まるもの |
| --- | --- |
| `import` | `POST /api/videos/{id}/probe` と同じように、動画の失敗した段階をもう一度キューに入れる |
| `scan` | `POST /api/scans` が始めるのと同じ、すべてのメディアフォルダの手動のスキャン |
| `auto_tag` | 動画を自動タグ付けのキューに入れる |
| `version_judgement` | 組の `retry_at` を今にし、判定のワーカーを起こす |

| ステータス | `code` | 条件 |
| --- | --- | --- |
| `404` | `not_found` | その項目がない |
| `409` | `activity_not_retryable` | 項目が再試行を示さないか、その対象がなくなった |
| `409` | `scan_in_progress` | スキャンが動いている間の `scan` の項目。`POST /api/scans` の応答と同じ |
| `409` | `auto_tagging_off` | 自動タグ付けがオフの間の `auto_tag` の項目 |

## `POST /api/activity/{id}/undo` {#post-apiactivityidundo}

`operationId: undoActivity`。自動の決定を取り消し、所有者の `undo` の項目とともに `200` を応答する。所有者がすでに状態を変えていたときは、その `outcome` は `already_reverted` である ([R-7](../research.md#r-7-undo-restores-the-state-and-always-records-the-exclusion))。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `tagId` | body | integer | `auto_tag` では必須 | 取り消すタグ |

| ステータス | `code` | 条件 |
| --- | --- | --- |
| `404` | `not_found` | その項目がないか、そのタグがこの項目の加えたものでない |
| `409` | `activity_not_undoable` | 項目かタグが取り消しを示さないか、すでに取り消されている |

## `GET /api/events`: `activity` {#get-apievents-activity}

ペイロードのない新しい通知の種類で、いずれかの項目が書かれるか変わった後に、所有者の流れに送る ([R-8](../research.md#r-8-the-screen-refreshes-on-an-activity-notification))。アクティビティの画面はそれを受けて件数と読み込んだ項目を取得し直し、シェルの入口は件数を取得し直す。
