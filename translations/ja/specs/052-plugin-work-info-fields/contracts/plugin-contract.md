---
source: specs/052-plugin-work-info-fields/contracts/plugin-contract.md
sourceHash: 8d2e37c224423b9a1a7ecc62d83b3629f676f933112f9b050e8044c86a58663b
---

# 契約: プラグインの取得の応答の作品の詳細 {#contract-work-details-in-the-plugins-fetch-response}

正本: [作品情報プラグインを書く、Response](../../../docs/how-to/work-info-plugins.md#response)。「プラグインの取得の応答で作品の詳細を受け取る」単位がこれを更新する。契約のバージョン 1 はそれ以外は変わらない。要求、ホスト関数、上限、アドレスは公開したとおりである ([R-1](../research.md#r-1-the-fetch-response-gains-optional-work-detail-fields-in-contract-1))。

## `fetch` の結果 {#fetch-result}

`{"workInfo": {...}}` は `title` と `tags` の横に、以下の任意のプロパティを得る。VVMDM はこれらのプロパティのすべての文字列の前後の空白を除き、除いた後に空になる項目や、`kind`、`role`、`source`、`value` が無いか未知の項目を落とし、落とした後に数えて各上限までの先頭の項目を残す。`title` と `tags` はこれまでとまったく同じに読む。この空白の除去はそれらに及ばない (要件 8)。

| プロパティ | 形 | 残すもの |
| --- | --- | --- |
| `code` | 文字列 | — |
| `description` | 文字列。改行を許す | — |
| `dates` | `[{"kind": "release" / "recorded" / "published", "date": "YYYY", "YYYY-MM" or "YYYY-MM-DD"}]` | 種類ごとに最初の項目 |
| `people` | `[{"role": "performer" / "director" / "photographer" / "streamer", "name": "...", "externalIds": [{"source": "...", "value": "..."}]}]`。`externalIds` は任意 | 役割ごとに先頭 100 件、項目ごとに先頭 20 の外部 ID |
| `organizations` | `[{"role": "maker" / "label", "name": "...", "externalIds": [...]}]` | 役割ごとに先頭 20 件、項目ごとに先頭 20 の外部 ID |
| `urls` | `["https://..."]` | 重複を除いた先頭 20 件 |
| `externalIds` | `[{"source": "...", "value": "..."}]` | 重複を除いた先頭 20 の `(source, value)` |

[作品情報を設定する](../../../docs/how-to/external-api.md#set-work-information) の規則を破る値 (100 文字を超える `code`、別の形の `date`、絶対でない URL) は結果に残して所有者に見せる。適用がそれを飛ばして所有者に伝える ([R-4](../research.md#r-4-the-apply-merges-the-selected-fields-into-the-stored-work-information-in-one-transaction-and-skips-each-value-that-breaks-a-rule))。1 つの役割に 2 回ある同じ人物は 1 回だけ残す。

## クライアントの使い方 {#client-use}

`title` と `tags` だけを返すプラグインは変更を要しない。プラグインは、人物の外部サイト自身の識別子を `externalIds: [{"source": "<site>", "value": "<id>"}]` として返す。そうすると VVMDM は、取得をまたいで、また同じ `source` を使うプラグインをまたいで同じ出演者を結び付ける ([R-3](../research.md#r-3-people-and-organizations-are-matched-by-external-id-then-by-folded-name-or-alias-and-an-ambiguous-match-links-nothing))。
