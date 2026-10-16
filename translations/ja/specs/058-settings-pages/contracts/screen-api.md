---
source: specs/058-settings-pages/contracts/screen-api.md
sourceHash: 79e23541ddd8c7dc79d7d7df5d8c226a505305d765deb3bee72fc738e870afb6
---

# 契約: 設定ページの画面 API {#contract-screen-api-for-the-settings-pages}

正本: [`api/openapi.yaml`](../../../api/openapi.yaml)。この文書はこの機能が変えるものだけを持つ。誤りの形は [023 error-api.md](../../023-english-i18n/contracts/error-api.md) にある。`api/external-v1.yaml` は変わらない。

## `OllamaSettings` の差分 {#ollamasettings-delta}

`getOllamaSettings` と `updateOllamaSettings` は同じスキーマを返す。それに 1 つのフィールドを足す ([R-4](../research.md#r-4-ollamasettings-reports-whether-the-connection-was-saved))。

| フィールド | 型 | 必須 | 意味 |
| --- | --- | --- | --- |
| `saved` | boolean | 必須 | 所有者が接続先を保存した (重複の判定が読むとおり、行がある)。`endpoint` と `model` が、未保存の接続に対してサーバーが返す既定値の間は `false` |

`updateOllamaSettings` は `saved: true` を返す。エンドポイント、本文、ステータス、誤りは変わらない。

## クライアントの使い方 {#client-use}

自動タグ付けとライブラリのページは、開いたときに `GET /api/settings/ollama` を 1 度読み、`saved` を接続の状態とする ([R-5](../research.md#r-5-the-auto-tagging-page-is-gated-by-the-connection-with-three-other-states))。失敗したリクエストは状態を不明のままにする。接続のページは今までどおり、フォームのためにこの資源を読み続ける。
