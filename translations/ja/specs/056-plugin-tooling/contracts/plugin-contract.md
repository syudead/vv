---
source: specs/056-plugin-tooling/contracts/plugin-contract.md
sourceHash: fd2935608ba6463b8416101f501280d2316afa01d31efd96dfdbf34e21990979
---

# 契約: プラグインの契約への追加 {#contract-plugin-contract-additions}

正典: 作者向けの手順書 [docs/how-to/work-info-plugins.md](../../../docs/how-to/work-info-plugins.md)。実装がこれを更新する。契約のバージョンは `1` のままである。新しいフィールドのないマニフェストと、新しい指針を無視するプラグインは、従来どおり動く。このファイルは、この機能が加えるものだけを挙げる。

## Manifest {#manifest}

`settings` の各エントリは、2 つのフィールドを追加で持てる ([R-3](../research.md#r-3-a-setting-declares-default-and-options-the-stored-value-or-the-default-is-what-the-run-receives))。

| フィールド | 型 | 必須 | 規則 |
| --- | --- | --- | --- |
| `default` | string | いいえ | 1 から 2000 文字。`options` があるときは、そのうちの 1 つ。`secret` とは併用できない |
| `options` | string array | いいえ | 1 から 200 文字の重複しない値を 2 から 50 個。`secret` とは併用できない |

規則に違反するマニフェストは、アップロード時と `mdm plugin pack` で `manifest_invalid` になる。

## Request {#request}

`settings` は、値を持つ宣言済みのすべてのキーの実効値を載せる。所有者の保存された値が設定されていて、`options` のある設定ではそのうちの 1 つであればその値、そうでなければ `default` である。既定値のある `required` のキーが実行を止めることはない。

`fetch` の要求は `id` を持ち、`url` を持たない ([R-10](../research.md#r-10-a-fetch-takes-a-work-id-only-and-vvmdm-never-fetches-a-url-the-owner-typed)): 候補の `id`、または所有者が入力してトリムした文字列で、プラグインがこれをページの URL に対応づける。`url` を読んでいたプラグインは、代わりに `id` を読む。

## Candidates {#candidates}

`search` の候補は `url` の代わりに `id` を持つ。前後に空白のない 1 から 200 文字で、所有者が候補を選ぶと、VVMDM はこれをそのまま取得の `id` として送る。ダイアログはタイトルの下に ID を示し、ページへのリンクを持たない。

## Standard error {#standard-error}

プラグインが標準エラーに書くものは、プラグインの直近の実行について設定画面で所有者に表示する。先頭の 64 KiB を保持し、各秘密の設定値は `[secret]` に置き換える ([R-6](../research.md#r-6-the-runtime-returns-each-runs-standard-error-and-the-app-keeps-the-last-one-per-plugin-with-the-secret-values-masked))。書く前にエンコードしたり分割したりした秘密は置き換えられない。

## Rate limiting {#rate-limiting}

手順書に、[R-9](../research.md#r-9-on-a-429-a-plugin-waits-for-the-smaller-of-retry-after-and-5-seconds-retries-once-and-otherwise-reports-an-error) の挙動で `429` 応答の節が加わる。実行の待ちの合計が最大 10 秒の範囲で、最大 5 秒後に 1 回再試行し、そのあとは `error` 応答にする。
