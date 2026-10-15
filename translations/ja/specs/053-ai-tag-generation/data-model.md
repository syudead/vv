---
source: specs/053-ai-tag-generation/data-model.md
sourceHash: 77b048dd723ce55cd024c544a0daa4d6b75dcdf910eb54b7c7186c715dcda301
---

# データモデル: 自動タグ付けの間に AI がタグを作成する {#data-model-tag-creation-by-the-ai-during-auto-tagging}

モデルの残りは変わらない: タグ、名前、却下した名前、割り当ては [014 データモデル](../014-video-tags/data-model.md) と [031 データモデル](../031-tentative-tags/data-model.md) のまま、種類は [051 データモデル](https://github.com/syudead/vv/blob/feature/051-tag-kinds/specs/051-tag-kinds/data-model.md) のまま、キューは [`00036_auto_tag_queue.sql`](../../internal/store/migrations/00036_auto_tag_queue.sql) のままである。このファイルは、この機能が加えるキューの 2 つの列、設定、ドメインの値を記録する。

## マイグレーション {#migration}

入るときに番号を付ける 1 つのマイグレーションが、`auto_tag_queue` に 2 つの列を加え、その `Down` セクションでそれらを削除する。既存の行は `attempts = 0` と `retry_at = 0` を得るので、今 `failed` である行は、以前と同じく所有者を待つ。

## `auto_tag_queue` {#auto_tag_queue}

| 項目 | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `attempts` | `integer not null default 0` | いいえ | 行がトリガーで最後にキューに入れられてから失敗した実行の数 |
| `retry_at` | `integer not null default 0` | いいえ | `failed` の行を再び取得してよくなる時刻 (Unix ミリ秒)。0 は決して取得しないことを意味する |

テーブルは今と同じく作り直せる: 失うと動画を判定し直し、ユーザーデータは失わない。

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| `Fail` は `attempts` を増やし、`retry_at` を、`attempts` が 1、2、3 のとき現在に 10 分、1 時間、6 時間を加えた時刻に、4 以上のとき 0 にする ([R-9](research.md#r-9-failed-jobs-retry-automatically-three-times)) | `domain` (遅延)、`AutoTagStore.Fail` |
| `Claim` は最も古い `queued` の行を取り、なければ、0 でも未来でもない `retry_at` のうち最も早いものを持つ `failed` の行を取る | `AutoTagStore.Claim` |
| 行をキューに入れるどのトリガーも `attempts = 0` と `retry_at = 0` を設定する | `AutoTagStore.queue` |
| 何もキューにないとき、ワーカーは最も早い `retry_at` まで眠る | `app.AutoTagger.Run` |
| 返された名前は、ジョブを完了として記録するのと同じトランザクションの中で、[R-7](research.md#r-7-how-a-returned-name-is-attached) の表に従って付ける | `AutoTagStore.Finish`、`resolveTagNames` を通じて |
| 作成したタグの `kind_id` は、モデルが選んだ名前の種類の id で、そのトランザクションの中で読む。その種類がもう存在しないときは null | `AutoTagStore.Finish` |

## 設定 {#settings}

`settings` テーブルの、既存の自動タグ付けと Ollama のキーの隣に保存する ([`internal/store/auto_tags.go`](../../internal/store/auto_tags.go))。

| キー | 値 | ないときの既定 |
| --- | --- | --- |
| `auto_tag.create_tags` | 真偽値 | オン ([R-8](research.md#r-8-settings-for-tag-creation)) |
| `auto_tag.tag_language` | 前後の空白を除いて 1 から 40 文字、制御文字なし | `Japanese` |
| `ollama.generation_model` | `ollama.model` と同じく正規化したモデルの名前 | `gemma4:e4b` ([R-3](research.md#r-3-a-separate-generation-model-default-gemma4e4b)) |

## `domain` に加える値 {#values-added-to-domain}

| 値 | 持つもの |
| --- | --- |
| `GeneratorRequest` | `Input ClassifierInput`、`Instructions string`、`Prompt string`、`Schema []byte` ([R-1](research.md#r-1-a-second-task-agnostic-port-for-generation)) |
| `AutoTagSettings.CreateTags`、`AutoTagSettings.TagLanguage` | 上の 2 つの設定 |
| `OllamaSettings.GenerationModel` | 上の設定 |
| `AutoTagVocabulary` | プロンプトが挙げるタグ、同義語、種類、却下した名前 ([R-5](research.md#r-5-reuse-comes-from-the-prompt-and-from-name-lookup)、[R-6](research.md#r-6-kinds-are-described-by-name-examples-and-preset-hints)) |
| `AutoTagProposal` | 返された 1 つの `{name, kind}` |
| `AutoTagSelection.Proposals` | `Finish` が解決する提案。モデルの順 |

## 変わらないもの {#what-does-not-change}

- `tags`、`tag_names`、`video_tags`、`rejected_tag_names`、`auto_tag_rejections`: 作成したタグは通常の仮のタグなので、タグの画面、外部 API、MCP は今と同じようにそれを確定し、却下し、統合する (要件 7)。
- `domain.AutoTagApplied`: `Finish` はすでにその動画についてこれを発行する。どのタグの書き込みもそうであるように、タグの作成は何も発行しない。
