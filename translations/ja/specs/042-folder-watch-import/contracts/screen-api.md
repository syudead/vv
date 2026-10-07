---
source: specs/042-folder-watch-import/contracts/screen-api.md
sourceHash: 8f66851781a44ad7402dead8dcfd8a57618d71dc7cddda256d54701403347c1e
---

# 契約: 自動取り込みの設定とスキャンの起点 {#contract-auto-import-setting-and-scan-origin}

正本: [api/openapi.yaml](../../../api/openapi.yaml) (`Scan` と、新しい `getAutoImportSettings` と `updateAutoImportSettings`)。どちらも、`/api/settings` の下のすべてのルートと同じく所有者専用である。外部 API (`api/external-v1.yaml`) は変わらない: 外部のクライアントのスキャンは `manual` である。

## `Scan.origin` {#scanorigin}

`Scan` ([024 の契約、`Scan`](../../024-import-progress/contracts/scan-api.md#scan)) は必須のフィールドを 1 つ得る。

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `origin` | `manual` \| `watch`、必須 | 誰が取り込みを始めたか ([data-model.md、`scans.origin`](../data-model.md#scansorigin-added-column)) |

`/api/events` の `scan` イベントはそれを変えずに運ぶ。クライアントは `watch` のスキャンの進み具合をどこにも示さず、`status` が `partial` か `failed` のときだけ通知する (`要件 7`)。Settings はその問題を今と同じく挙げる。

## `GET /api/settings/auto-import` {#get-apisettingsauto-import}

保存した選択と、監視が今何をしているかを返す。

**応答**: `200` と `AutoImportSettings`。

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `enabled` | boolean、必須 | 保存した選択 ([data-model.md、`settings` のキー](../data-model.md#settings-key-libraryauto_import)) |
| `watch.state` | `off` \| `starting` \| `active` \| `limited`、必須 | 無効のとき、またはメディアフォルダがないときは `off`。監視を加えている間は `starting`。すべてのディレクトリを監視しているときは `active`。監視の問題があるときは `limited` ([R-6](../research.md#r-6-lost-events-and-watch-limits-are-reported-never-repaired-by-a-scan)) |
| `watch.problem` | `watch_limit` \| `events_lost` \| `folder_unreachable` \| `permission_denied` \| 省略 | 最も新しい問題。`state` が `limited` のときだけある |
| `watch.path` | string \| 省略 | 問題が関わるディレクトリ。問題がディレクトリに関わるとき |

問題が消えるのは、所有者が自動取り込みをオフにしてオンにしたとき、メディアフォルダが変わったとき、または手動のスキャンが `done` で終わったとき (`events_lost` だけ: 全体を読めば失われたイベントを覆える) である。

## `PUT /api/settings/auto-import` {#put-apisettingsauto-import}

選択を保存し、監視を張るか外す。スキャンは始めない (`要件 6`)。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `enabled` | body | boolean | はい | 新しい選択 |

**応答**: 変更の後の `200` と `AutoImportSettings`。`watch.state` は `starting` か `off` である。今と同じ値は何も変えない。

| ステータス | `code` | 条件 |
| --- | --- | --- |
| `400` | `invalid_request` | 本文が `{ "enabled": boolean }` ではない |

## クライアントでの使い方 {#client-use}

Settings は、節がマウントされたときと、ウィンドウがフォーカスを取り戻したときに、そして `watch.state` が `starting` の間は 2 秒ごとに `GET /api/settings/auto-import` を読むので、プッシュのイベントやフォーカスの変化がなくても、状態は画面上で落ち着く。
