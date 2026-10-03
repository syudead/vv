---
source: docs/design-docs/writing-quality.md
sourceHash: 8f76b3d2ba6606c18d20161d25352bc721374eca56ccc2e7a08129b826d36f0f
---

# 文書の品質: 型に沿った技術英語 {#writing-quality-typed-technical-english}

この規則はリポジトリのすべての文書に適用する: `docs/`、`specs/`、ルートの文書、テンプレート、スキル。文書の種類ごとに型（見出しが固定された雛形）もあり、[文書の型](#document-types) に一覧がある。Plan には [plan-quality.md](plan-quality.md) の P-1..P-7 が加わり、Issue の仕様は [spec-quality.md](../product-specs/spec-quality.md) に従う。

日本語の読者は、文書サイトの `/ja/` 以下に公開される翻訳を使う（[docs-site.md](../how-to/docs-site.md)）。翻訳は `doc-translator` サブエージェントが、完成した英語から同じプルリクエストの中で書く（[japanese-translation.md](japanese-translation.md)）。翻訳は誰も手で編集しない。英語の原文を直し、もう一度翻訳する。

## 規則 {#rules}

### W-1: 文書は英語、Issue と PR 本文は日本語 {#w-1-documents-are-in-english-issues-and-pr-bodies-are-in-japanese}

リポジトリの文書はすべて技術英語で書く。親 Issue、子 Issue、プルリクエストの本文は日本語で書く。

文書内の日本語はインラインコードかコードブロックに入れる: 英語 UI 以前の画面文言の引用、例の中のユーザーデータ、`要件` のような Issue の節名。それ以外の場所に日本語があると `task check-docs` が失敗する。

### W-2: 要点から始める {#w-2-start-with-the-point}

文書や節の最初の文は、それが何を決めるか、または読者が何をできるかを述べる。前置きは削る。

| 書く例 | 書かない例 |
| --- | --- |
| 字幕は、要求のたびに動画のフォルダを読んで見つける。 | この節では字幕の見つけ方を説明する。 |
| `task docs` を実行し、表示された URL を開く。 | サイトをローカルで閲覧するには、いくつかの手順に従う必要がある。 |

### W-3: 水増ししない {#w-3-no-padding}

読み飛ばしても読者が何も失わない文は削る:

- 見出しの言い直し
- 上の節を締めくくるまとめ
- 重要性の表明（"It is important to note that"）
- 文書で扱う内容の一覧

| 書く例 | 書かない例 |
| --- | --- |
| ジョブは 5 時間で停止する。 | 制限内に収めるために、ジョブは 5 時間で停止するよう設計されている点に留意すべきである。 |

### W-4: 一つのことは一度だけ言う {#w-4-say-each-thing-once}

同じ節でも別の節でも、要点を別の言葉で言い直さない。それを述べている箇所にリンクする。

| 書く例 | 書かない例 |
| --- | --- |
| 翻訳は `translations/ja/` に置く（保存先の節を参照）。 | 翻訳は `translations/ja/` に置く。言い換えると、翻訳したファイルは `translations/ja/` という別のディレクトリに保持される。 |

### W-5: 根拠のない強調をしない {#w-5-no-emphasis-without-evidence}

強意語や宣伝文句（"critical"、"robust"、"seamless"、"comprehensive"、"powerful"、"significantly"、"simply"、"just"）は、同じ文に数値か理由が続く場合を除いて使わない。太字は読者が見落としてはならない用語を示すもので、感情を示すものではない。

| 書く例 | 書かない例 |
| --- | --- |
| 10B のモデルは 4 vCPU のランナーで 3–5 トークン/秒を生成するため、初回の実行に 40 時間かかる。 | 大きなモデルは著しく遅く、致命的なボトルネックになる。 |

### W-6: 抽象より具体 {#w-6-concrete-over-abstract}

ファイル、関数、エンドポイント、数値を名指しする。現在の挙動は現在形と能動態で書く。

| 書く例 | 書かない例 |
| --- | --- |
| `seekSpriteCell` は位置を `frameCount - 1` に収める。 | 位置は境界で適切に処理される。 |

### W-7: 構造には表と図、理由には文章 {#w-7-tables-and-diagrams-for-structure-prose-for-reasons}

| 内容 | 形式 |
| --- | --- |
| 同じ属性で比較する選択肢 | 表 |
| 状態、場合、状況と、それぞれでの挙動 | 表 |
| 対応関係（フィールド → 意味、コード → メッセージ、パス → 所有者） | 表 |
| コンポーネントをまたぐ流れ、呼び出しの順序、依存グラフ、状態機械 | Mermaid 図 |
| 読者が順に実行する手順 | 番号付きリスト |
| ある選択をした理由 | 文章 |

表のセルには一つの記述だけを入れる。セルに段落が必要なら、その内容は文章にあたる。どの図にも、何を示すかを述べる文が図の前にある。

### W-8: 一つの概念に一つの用語 {#w-8-one-term-per-concept}

一つの概念には、すべての文書を通じて一つの英語の用語を使う。製品の用語は、[web/src/i18n/en.ts](../../web/src/i18n/en.ts) で定義された、画面に表示される語を使う。用語を導入するときは一度だけ定義し、他の箇所からはその定義にリンクする。[翻訳の用語](japanese-translation.md#terms)は、各用語を一つの日本語訳に対応づける。

### W-9: 安定した見出し {#w-9-stable-headings}

見出しは名詞句か決定事項（`R-3: Glossary and product terms`）であり、疑問文や文の断片ではない。見出しはリンク先になる: 見出しを変えるとそのアンカーが変わり、その変更では参照元のリンクをすべて更新しなければならない（アンカーが壊れていると `task check-docs` が失敗する）。

## 文書の型 {#document-types}

種類ごとに雛形がある。文書を書くスキルは雛形をコピーし、当てはまる節を残し、残りを削除する（[plan-quality.md](plan-quality.md) の P-6）。

| 種類 | 雛形 | 作成者 |
| --- | --- | --- |
| Plan | [plan-template.md](../../.agents/skills/sdd-plan/assets/plan-template.md) | `sdd-plan` |
| 調査 | [research-template.md](../../.agents/skills/sdd-plan/assets/research-template.md) | `sdd-plan` |
| データモデル | [data-model-template.md](../../.agents/skills/sdd-plan/assets/data-model-template.md) | `sdd-plan` |
| 契約 | [contract-template.md](../../.agents/skills/sdd-plan/assets/contract-template.md) | `sdd-plan` |
| クイックスタート | [quickstart-template.md](../../.agents/skills/sdd-plan/assets/quickstart-template.md) | `sdd-plan` |
| UI 設計 | [ui-design-template.md](../../.agents/skills/sdd-design/assets/ui-design-template.md) | `sdd-design` |
| 設計文書 | [design-doc-template.md](../../.agents/skills/sdd-implement/assets/design-doc-template.md) | `sdd-implement`、または設計文書を追加する任意の変更 |
| 手順書 | [how-to-template.md](../../.agents/skills/sdd-implement/assets/how-to-template.md) | `sdd-implement`、または手順書を追加する任意の変更 |
