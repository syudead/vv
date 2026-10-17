---
source: specs/058-settings-pages/contracts/screen-api.md
sourceHash: 8e4590639ec8cea19619e4e317fd7df735e0e7e68bb2f58e3af169b52f505626
---

# 契約: 設定ページの画面 API {#contract-screen-api-for-the-settings-pages}

正本: [`api/openapi.yaml`](../../../api/openapi.yaml)。この文書はこの機能が変えるものだけを持つ。誤りの形は [023 error-api.md](../../023-english-i18n/contracts/error-api.md) にある。`api/external-v1.yaml` は変わらない。

## `OllamaSettings` の差分 {#ollamasettings-delta}

`OllamaSettings` は `getOllamaSettings` と `updateOllamaSettings` が返す本文である。それは 1 つのフィールドを得る ([R-4](../research.md#r-4-ollamasettings-reports-whether-the-connection-was-saved))。

| フィールド | 型 | 必須 | 意味 |
| --- | --- | --- | --- |
| `saved` | boolean | 必須 | 所有者が接続先を保存した (重複の判定が読むとおり、行がある)。`endpoint` と `model` が、未保存の接続に対してサーバーが返す既定値の間は `false` |

`updateOllamaSettings` は `saved: true` を返す。

## `UpdateOllamaSettingsRequest` {#updateollamasettingsrequest}

`PUT /api/settings/ollama` のリクエストの本文は `OllamaSettings` を参照するのをやめ、今受け付ける 2 つのフィールドだけを持つ独自のスキーマを得る。このため `saved` はリクエストの一部にならない。

| フィールド | 型 | 必須 | 意味 |
| --- | --- | --- | --- |
| `endpoint` | string | 必須 | `OllamaSettings.endpoint` と同じ |
| `model` | string | 必須 | `OllamaSettings.model` と同じ |

`additionalProperties: false`。ハンドラのデコーダがほかのフィールドを `400` で拒むからである。SPA の `updateOllamaSettings` は `endpoint` と `model` だけを送り、読んだ本文をそのまま送ることはない。エンドポイント、ステータス、誤りは変わらない。

## クライアントの使い方 {#client-use}

自動タグ付けとライブラリのページは、開いたときに `GET /api/settings/ollama` を 1 度読み、`saved` を接続の状態とする ([R-5](../research.md#r-5-the-auto-tagging-page-is-gated-by-the-connection-with-three-other-states))。失敗したリクエストは状態を不明のままにする。接続のページは今までどおり、フォームのためにこの資源を読み続ける。
