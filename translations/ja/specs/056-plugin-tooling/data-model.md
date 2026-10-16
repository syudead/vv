---
source: specs/056-plugin-tooling/data-model.md
sourceHash: a18bbfbd98a9e2a2b13fac5f7ad63f04f9480997cb0a9458b04069fbeba887d7
---

# データモデル: 作品情報プラグインの道具 {#data-model-work-info-plugin-tooling}

モデルの残りは変わらない。[internal/store/migrations/](../../internal/store/migrations/) と [049 data-model.md](../049-work-info-plugins/data-model.md) を参照。プロキシは既存の `settings` テーブルの行であり、新しいマニフェストのフィールドは既存の `work_info_plugins.settings` JSON に載るので、どちらもマイグレーションは要らない。

## Migration {#migration}

`00054_work_info_plugin_runs.sql` が `work_info_plugin_runs` を作る。番号は、先に `main` に入ったマイグレーション (`00050` から `00053`) の次を取る。別のマイグレーションがこれより先に `main` に入った場合、番号は次の空きに移る。

## `work_info_plugin_runs` {#work_info_plugin_runs}

プラグインごとの直近の実行の出力 ([R-6](research.md#r-6-the-runtime-returns-each-runs-standard-error-and-the-app-keeps-the-last-one-per-plugin-with-the-secret-values-masked))。

| フィールド | 型 | Null | 意味 |
| --- | --- | --- | --- |
| `plugin_id` | integer | いいえ | 主キー。プラグイン |
| `generation` | integer | いいえ | 実行が始まったときのプラグインの行の `generation`。同じバイト列での置き換えを含め、置き換えのたびに変わる |
| `operation` | text | いいえ | `search` または `fetch` |
| `failure` | text | はい | 実行の失敗コード。成功した実行では null |
| `ran_at` | integer | いいえ | 実行が終わった Unix 秒 |
| `stderr` | text | いいえ | 標準エラーの先頭の 64 KiB。秘密の値は伏せる |
| `truncated` | integer | いいえ | 64 KiB を超える出力を捨てたとき 1 |

**関係**: `plugin_id` は `work_info_plugins.id` を `on delete cascade` で参照する。このテーブルは `work_info_plugins` と同じ所有者のデータであり、[Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery) に挙げる。

## `settings` {#settings}

| キー | 値 | 意味 |
| --- | --- | --- |
| `work_info_plugins.proxy_url` | [contracts/screen-api.md](contracts/screen-api.md#get-and-put-apisettingsplugin-proxy) のプロキシの URL | 行がないか `""` ならプロキシなし ([R-4](research.md#r-4-the-proxy-is-one-url-in-settings-read-on-each-run-and-passed-to-the-runtime-as-a-run-option)) |

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| 実行の行は、`work_info_plugins` が実行した世代の行をまだ持っているときだけ書く。そうでなければ何も書かない | `internal/store`。`id` と `generation` を条件にした 1 つの文 |
| 実行の行は、実行が終わった順に書く。アプリは、実行枠を解放する前に、ランタイムの `record` 呼び出しの中で行を書く | `internal/app`、`internal/wasmplugin` |
| 切り詰められた出力では、値全体のすべてに加えて、末尾にある秘密の値の先頭部分も伏せる | `internal/domain`、伏せ字の関数 |
| 置き換えるアップロードは、アップロードのトランザクションでプラグインの実行の行を削除する | `internal/store` |
| 設定の `default` と `options` は [contracts/plugin-contract.md の Manifest](contracts/plugin-contract.md#manifest) に従う | `internal/domain`。アップロード時と `mdm plugin pack` 時 |
| 設定の実効値は、保存された値が設定されていて選択肢の中にあればそれ、そうでなければ既定値である。選択肢にない保存された値は残し、`staleValue` として報告する | `internal/domain` (`Entry`、要求の値、`required` の確認) |
| 選択肢にない保存された値は、そのキーの次の書き込みで置き換わる | `internal/store`。ほかの値の書き込みと同じ |
| `work_info_plugins.proxy_url` は契約の URL の規則に従う | `internal/domain`。書き込みの前に `internal/app` が確認する |

## 変わらないもの {#what-does-not-change}

`work_info_plugin_settings` の列は変わらない。既定値はそこに書かない ([R-3](research.md#r-3-a-setting-declares-default-and-options-the-stored-value-or-the-default-is-what-the-run-receives))。取得結果はメモリ上に残る。選んだタグの名前は apply のパラメーターであり、保存しない。
