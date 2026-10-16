---
source: specs/055-activity/data-model.md
sourceHash: 6aaf5c4f6f8394722ae3cec4f164ccdb61d6deec32d3799a71b6d2591acf14a2
---

# データモデル: Activity {#data-model-activity}

モデルの残りは変わらない: スキーマは [`internal/store/migrations`](../../internal/store/migrations) のマイグレーションであり、この機能が書き込むテーブルは [030 データモデル](../030-video-versions/data-model.md)、[054 データモデル](../054-auto-dup/data-model.md)、[auto-tagging.md](../../docs/design-docs/auto-tagging.md) にある。

## マイグレーション {#migration}

1 つのマイグレーションが、`activity_entries`、`activity_entry_tags`、`video_version_judging_holds` を作り、`jobs`、`auto_tag_queue`、`video_version_candidates` に null を許す `activity_id` を加える。番号は、それを書く時点の `main` の最後のマイグレーションの次 (今は `00050`) である。Down はそれらを削除する。それより前に起きたことについては項目を書かない (親 Issue の最後のエッジケース)。

| テーブル | 分類 ([Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)) |
| --- | --- |
| `activity_entries`、`activity_entry_tags` | ユーザーデータ: 作り直さず、消去しない ([R-1](research.md#r-1-a-persistent-activity-log-named-apart-from-the-scan-activity)) |
| `video_version_judging_holds` | ユーザーデータ: 所有者の取り消しであり、`video_version_unbundles` と同じように保つ |

## `activity_entries` {#activity_entries}

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `id` | integer | 不可 | 主キー。ページの順序 ([R-9](research.md#r-9-keyset-paging-by-id-newest-first-filtered-by-status)) |
| `kind` | text | 不可 | 下の種類のどれか |
| `subject_kind` | text | 不可 | `video`、`pair`、`scan`、`folder` のどれか |
| `subject_key` | text | 不可 | 内容の鍵、組の小さい方の鍵、スキャンの id、または削除したメディアフォルダのパス ([R-10](research.md#r-10-the-subject-is-a-content-key-with-a-label-kept-at-write-time)) |
| `subject_key_b` | text | 可 | 組の大きい方の鍵 |
| `subject_label` | text | 不可 | 表示名 (組では両方)、スキャンの起点、またはフォルダのパス。書いたとおり |
| `status` | text | 不可 | `running`、`attention`、`done` のどれか ([R-4](research.md#r-4-three-statuses-with-a-reason-and-a-newer-run-closes-an-older-problem)) |
| `reason` | text | 可 | `running` と `attention` について: 理由を参照 |
| `outcome` | text | 可 | `done` について: 結果を参照 |
| `detail` | text | 可 | 失敗についてのプロバイダか段階のメッセージ、所有者の操作が変えたタグ、メディアフォルダの削除が取り除いた動画の数 |
| `stage` | text | 可 | 失敗した `import` について、諦めた取り込みの段階 (`jobs.kind` の値) |
| `undoes_id` | integer | 可 | `undo` の項目について、それが取り消した項目 |
| `decider` | text | 不可 | `rule`、`ai`、`owner` のどれか ([R-5](research.md#r-5-the-decider-is-who-made-the-recorded-decision-else-who-started-the-run)) |
| `probability` | real | 可 | モデルが決めたか未決定のまま残した重複の組についての、モデルの答え |
| `undo_keys` | text | 可 | まとめた組について、加わった側のメンバーの鍵と、その側の以前の代表の鍵を持つ JSON オブジェクト ([R-7](research.md#r-7-undo-restores-the-state-and-always-records-the-exclusion)) |
| `undone_by` | integer | 可 | この項目を取り消した所有者の項目 |
| `created_at`、`updated_at` | integer | 不可 | Unix ミリ秒 |

**関係**: `videos` へはなし。`undone_by` はほかの項目を指す。`(status, id)` と `(kind, subject_key, subject_key_b, id)` にインデックスを置く。

### 種類 {#kinds}

| `kind` | 対象 | 書くもの |
| --- | --- | --- |
| `import` | video | 未完了のジョブがない動画の取り込みのジョブをキューに入れること ([R-3](research.md#r-3-one-entry-per-run-and-subject-an-import-is-one-entry-for-all-its-stages)) |
| `scan` | scan | `interrupted` 以外の理由で `failed` で終わったスキャン |
| `auto_tag` | video | 動画を自動タグ付けのキューに入れること |
| `version_judgement` | pair | 判定がオンの間に判定できる候補が現れること、または判定がオンにされること |
| `work_info_fetch` | video | プラグインでの取得 ([R-11](research.md#r-11-a-failed-work-information-fetch-is-a-finished-entry-not-a-problem)) |
| `bundle`、`unbundle`、`make_representative`、`dismiss` | video か pair | 所有者のバージョンの操作 ([R-6](research.md#r-6-the-owner-operations-recorded)) |
| `tag_attach`、`tag_detach`、`tag_keep`、`tag_reject` | video | 所有者のタグの操作。動画とタグごとに 1 つの項目 |
| `work_info_apply` | video | 取得した作品情報の適用 |
| `media_folder_remove` | folder | メディアフォルダの削除。`detail` は、それが取り除いた動画の数を持つ |
| `organize_undo` | video | ドラッグでの整理の取り消し。変わった動画ごとに 1 つの項目 |
| `undo` | 取り消した項目と同じ | 自動の決定の取り消し ([R-7](research.md#r-7-undo-restores-the-state-and-always-records-the-exclusion)) |

### 理由 {#reasons}

| `reason` | 状態 | 種類 |
| --- | --- | --- |
| `waiting` | running | ワーカーが取得する前の `import`、`auto_tag`、`version_judgement` |
| `processing` | running | ワーカーが取得した |
| `failed` | attention | `import` (ある段階が `MaxJobAttempts` の後に諦めた。`stage` がそれを示し、`detail` がそのメッセージを持つ)、`scan`、`auto_tag`、`version_judgement` (その再試行の時刻とともに) |
| `interrupted` | attention | 起動時に理由が `processing` の `running` だった項目 |
| `no_connection` | attention | Ollama の接続が保存されていないのにモデルが要る `version_judgement` |
| `undecided` | attention | 所有者に委ねた `version_judgement` |

### 結果 {#outcomes}

| `outcome` | 意味 |
| --- | --- |
| `completed` | 実行が終わったか、操作が適用された |
| `same`、`different` | `version_judgement` が組をまとめたか退けた |
| `failed` | エラーを返した `work_info_fetch` |
| `superseded` | この項目が `attention` の間に、同じ対象に対する同じ種類の新しい項目が始まった |
| `decided_by_owner` | 所有者が、開いている `version_judgement` の組をまとめたか退けた |
| `gone` | 開いている `version_judgement` の候補が、動画とともになくなった |
| `already_reverted` | 対象の状態を所有者がすでに変えていた `undo` |

## `activity_entry_tags` {#activity_entry_tags}

`auto_tag` の項目が加えたタグ。取り消しはタグごとである。

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `entry_id` | integer | 不可 | `auto_tag` の項目。`tag_id` とともに主キー |
| `tag_id` | integer | 不可 | タグ |
| `tag_name` | text | 不可 | 書いたとおりの名前 |
| `suggested` | integer | 不可 | 提案したタグとして加えたときは 1 |
| `user_key` | text | 不可 | 付与を書いたユーザーキー (内容の鍵かまとまりの鍵) |
| `attached_at` | integer | 不可 | 書いた `video_tags.created_at`。取り消しは、まだその値を持つ行だけを削除する |
| `probability` | real | 不可 | モデルの答え |
| `undone_by` | integer | 可 | `undo` の項目 |

## `video_version_judging_holds` {#video_version_judging_holds}

自動の "Different videos" を所有者が取り消した組。判定のワーカーはそれを決して取得しない。

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `key_a`、`key_b` | text | 不可 | 内容の鍵、`key_a < key_b`。主キー |
| `created_at` | integer | 不可 | 取り消しが起きた時刻 |

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| 項目は、それが記録する変更のトランザクションの中で書くか更新し、`ActivityChanged` はコミットの後に発行する ([R-2](research.md#r-2-an-entry-is-written-in-the-transaction-of-the-change-it-records)) | `store` (1 つのパッケージ内のヘルパー) |
| ジョブと自動タグ付けのキューの行は、自分が属する項目を指す。項目が `running` でない行を取得すると、新しい項目を開く | `store` |
| 組について挿入した候補は、作り直しによるものも含め、`(kind, subject_key, subject_key_b)` で見つけたその組の開いている `version_judgement` の項目に付く。`interrupted` の項目は、次の取得で `running` に戻る | `store` |
| 開いている `version_judgement` は、その鍵の 1 つがライブラリから離れたときだけ `gone` として閉じる | `store` |
| 判定をオフにしても、開いている `version_judgement` の項目は `waiting` のまま残る。オンにされるともう一度取得される | `store` |
| `import` の項目は、そのジョブのどれかが未完了の間は `running`、1 つが諦めたときは `attention`、それ以外は `done` である | `domain` (段階の状態についての規則)、`store` |
| ある対象に対するある種類の新しい項目は、その種類と対象の `attention` の項目を `superseded` として閉じる。`done` で終わったスキャンは、開いているすべての `scan` の項目を閉じる。`version_judgement` は再試行を通して組ごとに 1 つの項目を保つ | `domain`、`store` |
| 起動時、ワーカーが始まる前に、理由が `processing` の `running` の項目をすべて `attention`/`interrupted` にする。`waiting` の項目は残る | `store`。`cmd/mdm` から `Scans.RecoverInterrupted` と並べて呼ぶ |
| 再試行を示すのは、理由が `failed` か `interrupted` の `attention` で、種類が `import`、`scan`、`auto_tag`、`version_judgement` のどれかのときである | `domain` |
| 取り消しを示すのは、`rule` か `ai` が決めてまだ取り消していない項目である: 結果が `same` か `different` の `version_judgement`、または `auto_tag` の項目のタグ | `domain` |
| 判定の取得は、`video_version_unbundles` にある組を飛ばすのと同じように、`video_version_judging_holds` にある組を飛ばす | `store` (`judgeablePairs`) |
| 同じパスでの内容の変更は、`moveKeyPairs` が退けた組のキーを付け替えるのと同じように、`video_version_judging_holds` と開いている項目の対象の鍵を付け替える | `store` (引き継ぎ) |

## 変わらないもの {#what-does-not-change}

`video_version_dismissals`、`video_bundles`、`video_tags` とキューは意味を保つ: 決定者は決定が書いた行ではなく項目にあるので、054 の「自動のまとめは所有者のまとめと同じ行である」は引き続き成り立つ。
