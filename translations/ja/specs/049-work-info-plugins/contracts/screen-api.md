---
source: specs/049-work-info-plugins/contracts/screen-api.md
sourceHash: 4de537eb9de52dfa838c8f608795016d906b5e97e5efafd93177b769cfdc1e2d
---

# 契約: 作品情報プラグインの画面の API {#contract-work-info-plugin-screen-api}

正本: [api/openapi.yaml](../../../api/openapi.yaml)。Go と TypeScript は `task generate` で生成する。以下の操作はすべて所有者だけのもの (`security` キーなし) なので、他の所有者のルートと同じく、ゲストは `403` を受け取る。エラーは `Error` とその `code` を使う ([023 error-api](../../023-english-i18n/contracts/error-api.md))。外部 API と MCP は変わらない。

## `WorkInfoPlugin` {#workinfoplugin}

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `id` | integer | 行の id |
| `name` | string、省略可 | マニフェストの名前。マニフェストを読めなかったときはない |
| `fileName` | string | アップロードしたファイルの名前 |
| `version`、`description` | string、省略可 | マニフェストから |
| `operations` | `search` / `fetch` の配列 | マニフェストから。無効なプラグインでは空 |
| `digest` | string | モジュールの SHA-256。承認するときに送り返す ([R-7](../research.md#r-7-approving-names-the-digest-the-owner-saw)) |
| `state` | `approved` / `unapproved` / `invalid` | [data-model.md、状態の遷移](../data-model.md#state-transitions) |
| `problem` | string の列挙、省略可 | `invalid` のときにある ([data-model.md](../data-model.md#work_info_plugins)) |
| `network` | boolean | 契約 1 では常に `true`: プラグインはインターネットのサイトに到達できる (要件 2) |
| `updatedAt` | integer | Unix 秒 |

## `GET /api/work-info-plugins` {#get-apiwork-info-plugins}

すべてのプラグインを、名前、次にファイル名の順に一覧する。設定画面と動画のページがこれを読む ([R-11](../research.md#r-11-the-video-page-reads-the-plugin-list-to-decide-whether-to-show-the-fetch-action))。

**応答**: `{"plugins": [WorkInfoPlugin]}` を伴う `200`。

## `POST /api/work-info-plugins` {#post-apiwork-info-plugins}

プラグインを加えるか、同じマニフェストの名前を持つものを置き換える。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `module` | マルチパートのパート | file | はい | `.wasm` ファイル。そのファイル名を保つ |

**応答**: 新しい `WorkInfoPlugin` を伴う `201`、または置き換えたものを伴う `200`。使えないファイルも保存し、`state: invalid` で返す。

| ステータス | `code` | いつ |
| --- | --- | --- |
| 400 | `invalid_request` | `module` パートがないか、2 つ以上ある |
| 413 | `plugin_too_large` (`limit`) | パートが 16 MiB を超える |

## `PUT /api/work-info-plugins/{id}/approval` {#put-apiwork-info-pluginsidapproval}

所有者が見たモジュールを承認する。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `digest` | 本文 | string | はい | 一覧の `digest` |

**応答**: `WorkInfoPlugin` を伴う `200`。

| ステータス | `code` | いつ |
| --- | --- | --- |
| 404 | `not_found` | そのプラグインがない |
| 409 | `plugin_changed` | 保存されたダイジェストが異なる |
| 409 | `plugin_unusable` | プラグインが `invalid` である |

## `DELETE /api/work-info-plugins/{id}/approval` {#delete-apiwork-info-pluginsidapproval}

承認を取り消す。**応答**: `WorkInfoPlugin` を伴う `200`。`404` `not_found`。

## `DELETE /api/work-info-plugins/{id}` {#delete-apiwork-info-pluginsid}

プラグインを削除する。**応答**: `204`。`404` `not_found`。

## `POST /api/work-info-plugins/{id}/search` {#post-apiwork-info-pluginsidsearch}

プラグインの `search` を実行し、その候補を返す。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `query` | 本文 | string | はい | 前後の空白を除いた後で 1 から 200 文字 |

**応答**: `{"candidates": [{"url", "title", "detail"?}]}` を伴う `200` ([R-10](../research.md#r-10-search-candidates-are-text-only))。

エラー: [実行の失敗](#run-failures)、加えて、マニフェストに `search` がないときは `409` `operation_unsupported`。

## `POST /api/videos/{id}/work-info-fetches` {#post-apivideosidwork-info-fetches}

この動画のためにプラグインの `fetch` を実行し、結果を 10 分間保つ ([R-8](../research.md#r-8-a-fetched-result-lives-in-memory-for-10-minutes-bound-to-the-video-and-the-plugins-digest))。動画は変えない。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `pluginId` | 本文 | integer | はい | 承認済みのプラグイン |
| `url` | 本文 | string | はい | http か https、最大 2048 文字 |

**応答**: 次を伴う `201`:

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `fetchId` | string | 結果の id |
| `expiresAt` | integer | Unix 秒 |
| `title` | string、省略可 | 表示名の規則を通した後の取得したタイトル。プラグインが返さなかったか、それが規則を破るときはない |
| `tags` | string の配列 | 取得したタグ名。空のこともある |

画面はこれらを、すでに持っている動画と比べる。値のないフィールドは選べない。

エラー: [実行の失敗](#run-failures)、動画に対する `404` `not_found`、マニフェストに `fetch` がないときの `409` `operation_unsupported`。

## `POST /api/videos/{id}/work-info-fetches/{fetchId}/apply` {#post-apivideosidwork-info-fetchesfetchidapply}

取得した結果の選んだフィールドを適用する ([R-9](../research.md#r-9-the-title-and-the-tags-are-applied-in-one-transaction-and-skipped-names-are-reported))。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `fields` | 本文 | `title` / `tags` の配列 | はい | 空でない。それぞれが結果の中に値を持たなければならない |

**応答**: `{"video": Video, "skippedTags": [{"name", "reason": "rejected" | "invalid"}]}` を伴う `200`。結果は消費される。

| ステータス | `code` | いつ |
| --- | --- | --- |
| 400 | `invalid_request` | `fields` が空か、値のないフィールドを挙げる |
| 404 | `not_found` | その動画がないか、結果が別の動画のものである |
| 409 | `work_info_fetch_stale` | 結果の期限が切れたか未知であるか、取得の後にプラグインが置き換えられた、承認を取り消された、または削除された |

## 実行の失敗 {#run-failures}

検索と取得は、失敗した実行にこれらで答える。動画は決して変えない。`Error.message` は vv 自身の英語の文のままである ([023 error-api](../../023-english-i18n/contracts/error-api.md#every-message-in-english)) ので、`Error` は vv の外から来たもののために省略できる 2 つのフィールドを得る: `pluginMessage` (プラグイン自身のエラーメッセージの最初の 300 文字) と `host` (拒否された、または到達できなかったホスト)。

| ステータス | `code` | `reason` | いつ |
| --- | --- | --- | --- |
| 404 | `not_found` | — | そのプラグインがない |
| 409 | `plugin_not_approved` | — | プラグインが承認されていない |
| 502 | `plugin_failed` | — | プラグインがエラーを報告した (`pluginMessage`)、0 以外で終了した、または無効な応答を書いた |
| 502 | `plugin_failed` | `blocked_address` | プラグインが vv、LAN、または他の公開されていないアドレスに到達しようとした (`host`) |
| 502 | `plugin_failed` | `network_error` | 要求が接続に失敗し、プラグインが失敗した (`host`) |
| 504 | `plugin_timed_out` | — | 実行が 30 秒を超えた |
| 502 | `plugin_failed` | `memory_exceeded` | 実行が 128 MiB を超えた |
| 502 | `plugin_failed` | `output_too_large` | 出力が 1 MiB を超えた |
| 503 | `plugin_busy` | — | 別の実行が 10 秒間枠を占めていた |
