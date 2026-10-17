---
source: specs/056-plugin-tooling/contracts/screen-api.md
sourceHash: dd22175953a55788ba2afd8097309cb94e69495ffb49dacf5f582848987ef0dd
---

# 契約: 画面 API の変更 {#contract-screen-api-changes}

正典: [api/openapi.yaml](../../../api/openapi.yaml)。Go と TypeScript は `task generate` で生成する。以下のすべての操作は、[049 screen-api.md](../../049-work-info-plugins/contracts/screen-api.md) と同じく所有者専用である。外部 API と MCP は変わらない。

## `WorkInfoPluginSetting` {#workinfopluginsetting}

3 つの省略可能なフィールドがエントリに加わる ([R-3](../research.md#r-3-a-setting-declares-default-and-options-the-stored-value-or-the-default-is-what-the-run-receives))。

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `default` | string、省略可能 | マニフェストの既定値 |
| `options` | string array、省略可能 | マニフェストの選択肢。その順序のまま |
| `staleValue` | string、省略可能 | 現在の選択肢のどれでもない保存された値。そのときだけ存在する。実行は代わりに既定値を受け取る |

`set` と `value` は [049](../../049-work-info-plugins/contracts/screen-api.md) の意味のままである。値が保存されているかどうかと、秘密でない設定ではその値である。古い値もまだ保存されているので、その設定は `set: true` で、`value` にその値を、`staleValue` に同じテキストを持つ。秘密の設定は選択肢を持たないので、古くなることはない。

## `WorkInfoPlugin` {#workinfoplugin}

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `lastRunAt` | integer、省略可能 | 出力を保持している直近の実行の Unix 秒 ([R-6](../research.md#r-6-the-runtime-returns-each-runs-standard-error-and-the-app-keeps-the-last-one-per-plugin-with-the-secret-values-masked))。なければ存在しない |

## `GET /api/work-info-plugins/{id}/last-run` {#get-apiwork-info-pluginsidlast-run}

プラグインの直近の実行の出力を返す。

**応答**: `200` と `{"run": WorkInfoPluginRun}`。現在のモジュールの実行が保持されていなければ `{}`。

| `WorkInfoPluginRun` のフィールド | 型 | 意味 |
| --- | --- | --- |
| `ranAt` | integer | Unix 秒 |
| `operation` | `WorkInfoOperation` | 実行した操作 |
| `failure` | string、省略可能 | 実行が失敗したときの [049 Run failures](../../049-work-info-plugins/contracts/screen-api.md#run-failures) の実行の失敗コード。成功した実行では存在しない |
| `stderr` | string | 秘密の値を伏せた、保持した標準エラー。プラグインが何も書かなかったときは空 |
| `truncated` | boolean | 64 KiB を超える出力を捨てたかどうか |

| ステータス | `code` | 条件 |
| --- | --- | --- |
| 404 | `not_found` | そのようなプラグインがない |

## `GET` と `PUT /api/settings/plugin-proxy` {#get-and-put-apisettingsplugin-proxy}

プラグインの実行が使うプロキシを読み書きする ([R-4](../research.md#r-4-the-proxy-is-one-url-in-settings-read-on-each-run-and-passed-to-the-runtime-as-a-run-option))。`/api/settings/network` と違い、このルートはすべてのビルドで応答する。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `url` | body | string | はい | `http`、`https`、`socks5`、ホスト、省略可能なポート、省略可能なユーザー情報。パスもクエリもなし。最大 2048 文字。`""` はプロキシなしを意味する |

**応答**: `200` と `{"url": "<the saved value>"}`。何も保存されていないとき、`GET` は `""` を返す。

| ステータス | `code` | 条件 |
| --- | --- | --- |
| 400 | `invalid_request` | `url` がないか、規則に違反する |

次の実行は保存された値を使う。進行中の実行は、開始したときの値で終わる。

## `POST /api/videos/{id}/work-info-fetches/{fetchId}/apply` {#post-apivideosidwork-info-fetchesfetchidapply}

要求に `tags` が加わる ([R-7](../research.md#r-7-the-apply-takes-the-selected-tag-names-and-a-tag-already-on-the-video-is-not-selectable))。

| フィールド | 場所 | 型 | 必須 | 意味 |
| --- | --- | --- | --- | --- |
| `tags` | body | string array | いいえ | `fields` に `tags` があるときに付ける取得した名前。結果の `tags` の空でない部分集合で、結果が返したとおりの綴り。なければ、従来どおり取得したすべてのタグ |

| ステータス | `code` | 条件 |
| --- | --- | --- |
| 400 | `invalid_request` | `tags` が空、結果にない名前を含む、または `fields` に `tags` がないのに存在する |

`skippedTags` は選んだ名前だけを報告する。

## Run failures {#run-failures}

| ステータス | `code` | `reason` | 変更 |
| --- | --- | --- | --- |
| 502 | `plugin_failed` | `network_error` | プロキシに到達できなかった、接続を拒否した、`407` を返したとき、`Error.host` はプロキシのホストである ([R-5](../research.md#r-5-through-a-proxy-the-destination-is-checked-by-resolving-its-name-before-each-request-and-redirect)) |
| 502 | `plugin_failed` | `blocked_address` | プロキシ経由で、宛先の名前が拒否されるアドレスに解決されるときにも応答する。`Error.host` はその名前である |
