---
source: docs/design-docs/writing-quality.md
sourceHash: fbce8f7c0b6c3481696156ac81578e250d755bce4812ada1fd332da748070736
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

数値、上限、ステータスコード、設定を示す。現在の挙動は現在形と能動態で書く。

| 書く例 | 書かない例 |
| --- | --- |
| 4 MiB を超える字幕ファイルは一覧に載らない。 | 大きなファイルは適切に処理される。 |

### W-7: まず図、次に表、文章は最後 {#w-7-draw-first-tabulate-second-write-prose-last}

この表で、内容に合う最初の形式を選ぶ。文章は理由にだけ使う。

| 内容 | 形式 |
| --- | --- |
| 何が何とやり取りするか: コンポーネント、ファイル、要求の経路 | Mermaid `flowchart` |
| 時間に沿った呼び出しやメッセージの順序 | Mermaid `sequenceDiagram` |
| 状態と、各遷移のきっかけ | Mermaid `stateDiagram-v2` |
| 分岐のある規則（これならあれ） | 判断ノード付きの Mermaid `flowchart` |
| 同じ属性で比較する選択肢 | 表 |
| 互いに順序のない場合と、それぞれでの挙動 | 表 |
| 対応関係（フィールド → 意味、コード → メッセージ、パス → 所有者） | 表 |
| 読者が順に実行する手順 | 番号付きリスト |
| ある選択をした理由 | 文章 |

設計文書は、扱う部分の図で始まり、流れ、順序、状態、分岐のある規則を主題とする節にはそれぞれ専用の図がある。文章と表だけの節は、図が欠けている兆候である。表のセルには一つの記述だけを入れる。どの図にも、何を示すかを述べる文が図の前にあり、図のラベルには文書の用語を使う（W-8）。ラベルは 4 語程度に収め、詳細は図の下の文に書く。分岐のある規則は左から右へ読む（`flowchart LR`）ので、判断ノードが小さく保たれる。

### W-8: 一つの概念に一つの用語 {#w-8-one-term-per-concept}

一つの概念には、すべての文書を通じて一つの英語の用語を使う。製品の用語は、[web/src/i18n/en.ts](../../web/src/i18n/en.ts) で定義された、画面に表示される語を使う。用語を導入するときは一度だけ定義し、他の箇所からはその定義にリンクする。[翻訳の用語](japanese-translation.md#terms)は、各用語を一つの日本語訳に対応づける。

### W-9: 安定した見出し {#w-9-stable-headings}

見出しは名詞句か決定事項（`R-3: Glossary and product terms`）であり、疑問文や文の断片ではない。見出しはリンク先になる: 見出しを変えるとそのアンカーが変わり、その変更では参照元のリンクをすべて更新しなければならない（アンカーが壊れていると `task check-docs` が失敗する）。

見出しには節番号を付けない（`## 3. Rules` ではなく `## Rules`）。番号はアンカーに入るので、節を追加または削除するとそれより後のすべての節の名前が変わり、それらへのリンクと `§3` 形式の参照が壊れる。節は見出しで参照し、そのアンカーにリンクする（`data-model.md, Rules`）。番号付きの見出しがあると `task check-docs` が失敗する。読み手が順に実行する手順は番号付きの見出しではなく、番号付きリスト（W-7）にする。

### W-10: コードが言えないことを書く {#w-10-write-what-the-code-cannot-say}

文書は、読者がコードから得られないことを記録する: 規則、理由、退けた代替案、利用者や呼び出し側が目にする挙動。文書はコードをなぞらない。ファイルや関数を名指しするのは規則がどこにあるかを言うためだけで、節ごとに一度とし、どの関数がどの関数を呼ぶかを語るためには使わない。

| 書く例 | 書かない例 |
| --- | --- |
| 再生に使う場所の隣にあるファイルだけが一覧に載る（[`mediafs`](../../internal/mediafs)）。 | `internal/httpapi/subtitles.go` は `openMediaFile` と同じ順序で場所を試す。`internal/mediafs` の `ListSidecarFiles` は、その場所のフォルダにある通常ファイルの名前とサイズを返す。ただし、その場所が `OpenMediaFile` と同じ規則で開ける場合に限る。 |

### W-11: 長さの上限 {#w-11-length-budget}

| 単位 | 上限 |
| --- | --- |
| 決定（設計文書の `##` 節、調査の `R-N`） | 規則を 1 文、理由を最大 3 文、場合があるときは表を 1 つ |
| 表のセル | 一つの記述 |
| 設計文書 | 2 画面に収まる。収まらないときは分割する |
| 手順書の手順 | 一つの操作 |

上限を超えた節には、コードをなぞる記述（W-10）か二度述べた要点（W-4）がある。分割する前にそれらを削る。

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
