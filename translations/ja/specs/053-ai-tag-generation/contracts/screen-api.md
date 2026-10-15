---
source: specs/053-ai-tag-generation/contracts/screen-api.md
sourceHash: 2a18bd9680d63f191e549d4847da3938380619f4572a84ebe499a2718423aecd
---

# 契約: 画面の API のタグ作成の設定 {#contract-tag-creation-settings-in-the-screen-api}

正本: [`api/openapi.yaml`](../../../api/openapi.yaml) の操作 `getAutoTaggingSettings`、`updateAutoTaggingSettings`、`getOllamaSettings`、`updateOllamaSettings`、`checkOllama`。この文書は、この機能が加える項目だけを記述する。どのルートも所有者だけのままである。外部 API と MCP は変わらない: 作成したタグは、それらがすでに提供している仮のタグである。

## スキーマの変更 {#schema-changes}

| スキーマ | 項目 | 必須 | 意味 |
| --- | --- | --- | --- |
| `AutoTaggingSettings` | `createTags: boolean` | はい | "Create new tags" ([R-8](../research.md#r-8-settings-for-tag-creation)) |
| `AutoTaggingSettings` | `tagLanguage: string` | はい | "Language for new tags" |
| `UpdateAutoTaggingSettingsRequest` | `createTags: boolean` | いいえ | ないときは保存した値を保つ |
| `UpdateAutoTaggingSettingsRequest` | `tagLanguage: string` | いいえ | ないときは保存した値を保つ |
| `OllamaSettings` | `generationModel: string` | 応答でははい、`PUT` ではいいえ | 生成モデル。`PUT` でないときは保存した値を保つ ([R-3](../research.md#r-3-a-separate-generation-model-default-gemma4e4b)) |

要求の項目は省略できるので、この機能より前に作られた画面もほかの設定を保存できる。

## `PUT /api/settings/auto-tagging` {#put-apisettingsauto-tagging}

| ステータス | `code` | 条件 |
| --- | --- | --- |
| 400 | `invalid_request` (今のしきい値と同じ) | `tagLanguage` が前後の空白を除くと空である、40 文字より長い、または制御文字を含む |

## `POST /api/settings/ollama/check` {#post-apisettingsollamacheck}

確認は `GET /api/tags` を 1 回読み、保存した両方のモデルを探す。モデルがないときは `available: false` を答え、`message` はないモデルのそれぞれと、その役割 (判定か生成か) を名指しする。今と同じく、タグのないモデルの名前は `:latest` を意味する ([ai-layer.md](../../../docs/design-docs/ai-layer.md#ollama-provider))。
