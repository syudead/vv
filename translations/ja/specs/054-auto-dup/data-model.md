---
source: specs/054-auto-dup/data-model.md
sourceHash: ce412e90277689b41d1e1516c0c7a60d082cc0950b7e79fb5fb80c166371e67e
---

# データモデル: 重複の候補を自動で判定する {#data-model-judging-possible-duplicates-automatically}

モデルの残りは変わらない: まとまり、候補、退けた組、指紋は [030 データモデル](../030-video-versions/data-model.md) にあり、スキーマは [`internal/store/migrations`](../../internal/store/migrations) のマイグレーションである。

## マイグレーション {#migration}

1 つのマイグレーションが、`video_version_candidates` に判定の列を加え、`video_version_unbundles` を作る。番号は、それを書く時点の `main` の最後のマイグレーションの次である (ほかの feature ブランチもマイグレーションを加える)。Down はテーブルと列を削除する。

| テーブル | 分類 ([Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery)) |
| --- | --- |
| `video_version_candidates` | 今と同じく作り直せるインデックス。その中の判定は、作り直しの後にもう一度問う |
| `video_version_unbundles` | ユーザーデータ: 所有者の操作であり、`video_version_dismissals` と同じように保つ |

## `video_version_candidates` {#video_version_candidates}

判定のために加える列 ([R-4](research.md#r-4-the-judgement-is-columns-on-the-candidate-row))。どの行も、それらを null か 0 にして作り直される。

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `judgement` | text | 可 | 待っている間は null、それ以外は `running`、`undecided`、`failed` のどれか |
| `attempts` | integer | 不可、既定値 0 | これまでの失敗の回数。取得と再起動を越えて保つ |
| `retry_at` | integer | 可 | `failed` の行が取得できるようになる時刻 ([R-5](research.md#r-5-a-failed-judgement-retries-on-a-backoff-that-never-ends))。それ以外は null |
| `judgement_error` | text | 可 | `failed` の行についてのプロバイダのメッセージ |
| `probability` | real | 可 | `undecided` の行についてのモデルの答え。衝突のために判定できないときは null |

## `video_version_unbundles` {#video_version_unbundles}

まとまりから外した組で、モデルは決して判定しない ([R-8](research.md#r-8-removing-a-version-records-the-pairs-and-rebuilds-its-candidates))。

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `key_a`、`key_b` | text | 不可 | 内容の鍵、`key_a < key_b`。主キー |
| `created_at` | integer | 不可 | バージョンを外した時刻 |

**関係**: なし。`video_version_dismissals` と同じく内容の鍵である。

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| 候補を取得できるのは、機能がオンで、その組が `video_version_unbundles` になく、その `judgement` が null か、`retry_at` を過ぎた `failed` であるときである | `domain` (規則)、`store` (取得のクエリ) |
| 同時に `running` の判定は 1 つである | 単一のワーカー。取得は 1 つの文である |
| 起動時に残っている `running` の行は、`attempts` を保ったまま null に戻る | `store`、起動時に `auto_tag_queue` が回復するのと同じように |
| 「同じ動画」は、両側をまたぐ組が退けた組かまとまりから外した組でない限り、両側をまとめる。そうであれば行は `undecided` になる ([R-6](research.md#r-6-a-same-pair-joins-two-sides-unless-a-pair-across-them-is-known-to-differ)) | `domain` (2 つのメンバーの集合に対する確認)、`store` (まとめるトランザクションの中で両方を読み直す) |
| 「同じ動画」は [R-7](research.md#r-7-the-representative-is-the-bundles-else-the-larger-picture) で代表を選び、それから `VersionStore.Bundle` の手順でまとめる | `domain` (選択)、`store` |
| 「別の動画」は、所有者の "Different videos" と同じように、`video_version_dismissals` を書いて候補を削除する | `store` |
| なくなったか、もう `running` でない候補についての結果は捨てる | `store` |
| `Unbundle` は外した鍵をほかのすべてのメンバーと組にして書き、それから外した鍵の候補を作り直す | `store` (`VersionStore.Unbundle`) |
| 同じパスの内容の変化は、`moveDismissals` が退けた組のキーを付け替えるのと同じように、`video_version_unbundles` のキーを付け替える | `store` (引き継ぎ) |
| すべての結果と `Unbundle` は、コミットの後に `VersionCandidatesChanged` を発行する ([R-10](research.md#r-10-the-duplicates-page-refreshes-on-a-candidates-notification)) | `store` |

## 設定 {#settings}

| キー | 値 | ないときの意味 |
| --- | --- | --- |
| `version_judging.enabled` | `true` か `false` | オフ |

## 変わらないもの {#what-does-not-change}

`video_version_dismissals`、`video_bundles`、`video_bundle_members` は列を保つ: 自動のまとめと自動の「別の動画」は、所有者の操作が書くのと同じ行であり、それが `UI品質` の「自動と手動の区別がない」という行の求めることである。
