---
source: specs/054-auto-dup/contracts/screen-api.md
sourceHash: 1e09d3333ba4451257d98ec63fe50d23cde7b94165ef16435dd795c6f52f782c
---

# 契約: 重複の自動判定の画面の API {#contract-screen-api-for-automatic-duplicate-judging}

正本: [`api/openapi.yaml`](../../../api/openapi.yaml)。この文書は、この機能が加えるか変えるものだけを持つ。バージョンのルートは [030 screen-api.md](../../030-video-versions/contracts/screen-api.md) にある。エラーの形は [023 error-api.md](../../023-english-i18n/contracts/error-api.md) である。

## `Video` の差分 {#video-delta}

| フィールド | 変更前 | 変更後 |
| --- | --- | --- |
| `versions` (`VideoVersionsRef {count, representativeId}`) | `GET /api/videos/{id}` だけ | `Video` を返す画面の API のすべての応答で、一覧の項目も含め、まとまりのメンバーについて入る ([R-9](../research.md#r-9-list-items-carry-the-existing-versions-summary-read-per-page))。`count` は引き続き、閲覧者の見てよい場所を持つバージョンの数である |

ゲストは、今の詳細と同じ規則でそれを受け取る。`api/external-v1.yaml` は変わらない。

## `GET /api/settings/version-judging` {#get-apisettingsversion-judging}

`operationId: getVersionJudgingSettings`。所有者だけ。`VersionJudgingSettings {enabled: boolean}` を返す。`enabled` は所有者が保存するまで `false` である。

## `PUT /api/settings/version-judging` {#put-apisettingsversion-judging}

`operationId: updateVersionJudgingSettings`。所有者だけ。本文は `{enabled: boolean}`。それを保存し、保存した `VersionJudgingSettings` とともに `200` で応答する。オンにすると判定のワーカーを起こし、ワーカーはそれから待っているすべての候補を判定する。

| ステータス | `code` | 条件 |
| --- | --- | --- |
| `400` | `invalid_request` | 本文が `{enabled: boolean}` でない |

Ollama の接続は独自のリソース (`/api/settings/ollama`) のままなので、接続が保存されていない間も機能はオンにできる。そのときはどの判定も失敗し、再試行する ([R-5](../research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends))。

## 振る舞いだけが変わるルート {#routes-that-change-behaviour-only}

| ルート | 変更 |
| --- | --- |
| `POST /api/videos/{id}/unbundle` | まとまりから外した組も記録し、外した動画の候補を作り直すので、指紋で一致するその動画の組はまた一覧に出る ([R-8](../research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates))。応答は変わらない |
| `GET /api/version-candidates` | 形は変わらない。モデルが同じか別と判定した候補はそこからなくなる |

## `GET /api/events`: `candidates` {#get-apievents-candidates}

ペイロードのない新しい通知の種類で、判定の結果かバージョンからの取り外しが候補を変えた後に送る ([R-10](../research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification))。Duplicates のページはそれで取得し直す。自動のまとめは、手作業のまとめと同じように、各メンバーについて `video` の通知も送る (030 R-9)。
