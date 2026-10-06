---
source: specs/041-tag-overflow-list/plan.md
sourceHash: 3cdfc0937d0ed4b090c461393298068891a81db1cb015f1252eca09e7fd5c65f
---

# 実装計画: カードの `+N` の裏に隠れたタグを、何個でも見えて押せるようにする {#implementation-plan-hidden-tags-behind-a-cards-n-visible-and-pressable-at-any-count}

**ブランチ**: `feature/041-tag-overflow-list` | **親 Issue**: #813

**入力**: 親 Issue。これがこの機能の仕様である。

## 概要 {#summary}

ライブラリとフォルダの画面で、カードの `+N` チップの裏の一覧は、マウスが `+N` の上で止まると開き、ポインタが `+N` か一覧の上にある間は開いたままで、隠れたタグをビューポートの中で折り返すチップとして並べ、収まらない分はスクロールさせ、どの名前も省略せずに示す。`+N` を押すとタッチとキーボードのために今までどおり開く。一覧は両方の画面が描画する `CardTagRow` の既存の `Popover` のままなので、1 つのコンポーネントの変更で、動画のカード、グループのカード、フォルダ画面のカードをまかなう。

| 関心事 | 方針 |
| --- | --- |
| ホバーで開く | ホバーの意図を読むフックが `Popover` の `open` を動かす。マウスのポインタが 400 ms 止まると開き、`+N` と一覧を離れて 200 ms 後に閉じる。押す操作はそのまま動く ([research.md R-1](research.md#r-1-resting-the-mouse-on-n-opens-the-same-popover-that-pressing-opens)) |
| どのタグにも届き、目の動きが少ない | 折り返すチップ。`PopoverContent` の高さを Radix の利用可能な高さに制限する。一覧はその中でスクロールする ([R-2](research.md#r-2-the-list-is-a-wrapping-row-of-chips-that-scrolls-inside-the-viewport)) |
| 長い名前 | 一覧の中では省略しない。一覧より幅の広い名前はチップの中で折り返す ([R-3](research.md#r-3-chips-in-the-list-never-truncate-a-long-name-wraps-inside-its-chip)) |
| 開いている間にタグや隠れた数が変わる | 一覧は描画のたびに行の現在のタグと数から導く ([R-4](research.md#r-4-the-list-is-derived-from-the-current-tags-never-copied-when-it-opens)) |
| 一覧のタグを押す | 変わらない。`onPress` がライブラリを絞り込むか、フォルダからは `/?tag=<id>` を開き、一覧は閉じる |

親 Issue のとおり範囲外のもの: 行がどのタグを示すか、リスト表示のタグ、選択中の隠れたタグ (`+N` は操作できないままで、開かない)、動画ページのタグ。

## 技術的な文脈 {#technical-context}

**正本の定義**:

| 項目 | 出典 |
| --- | --- |
| Web 層の境界とディレクトリ | [ARCHITECTURE.md、Web layer](../../ARCHITECTURE.md#web-layer) |
| タグの行、`+N`、収まる数の測定、押す操作と選択 | [specs/014-video-tags/ui-design.md、Overflow](../014-video-tags/ui-design.md#overflow) と [Card structure and pressing](../014-video-tags/ui-design.md#card-structure-and-pressing)。[web/src/library/CardTagRow.tsx](../../web/src/library/CardTagRow.tsx)、[TagRowMeasure.tsx](../../web/src/library/TagRowMeasure.tsx)、[tagRowOverflow.ts](../../web/src/library/tagRowOverflow.ts)。[web/src/folders/useFolderTagsRow.tsx](../../web/src/folders/useFolderTagsRow.tsx) |
| チップの形 | [017 UI 設計、Folder-derived tag chip](../017-folder-groups/ui-design.md#folder-derived-tag-chip)、[031 UI 設計、Tentative mark](../031-tentative-tags/ui-design.md#tentative-mark)、[components.md、Badge](../../web/registry/rules/components.md#badge) |
| デザインシステム: `Popover`、トークン、検査、例外の一覧 | [docs/design-docs/design-system.md](../../docs/design-docs/design-system.md)、[components.md、Popover](../../web/registry/rules/components.md#popover)、[web/src/ui/shadcn/popover.tsx](../../web/src/ui/shadcn/popover.tsx)、[web/design-exceptions.js](../../web/design-exceptions.js)、[038 contracts/registry.md](../038-design-system/contracts/registry.md) |
| ホバーの慣習: マウスだけの 400 ms の静止、幅は CSS で読む | [010 UI 設計、Interaction](../010-hover-video-preview/ui-design.md#interaction)、[library-ui.md、Width breakpoints in CSS, and the sidebar exception](../../docs/design-docs/library-ui.md#width-breakpoints-in-css-and-the-sidebar-exception) |
| 画面の文言 | [web/src/i18n/en.ts](../../web/src/i18n/en.ts) (`library.tagRow`) |
| ブラウザのテストと検査の入口 | [web/e2e/tags.e2e.ts](../../web/e2e/tags.e2e.ts)、[Taskfile.yml](../../Taskfile.yml) (`task check`、`task check-docs`、`task test-e2e`、`task generate`) |

**この機能に固有の文脈**:

- Web 層だけである。API、ストア、Go の変更はなく、新しい npm の依存もなく (Radix Popover はすでに使っている)、新しい画面の文言もない。`data-model.md` も `contracts/` もない。この機能はエンティティを追加せず、インターフェースも変えない ([P-2](../../docs/design-docs/plan-quality.md#p-2-do-not-create-an-artifact-with-nothing-to-say))。
- `PopoverContent` は、どのポップオーバーにも利用可能な高さの制限を持つようになる (R-2)。内容がビューポートより高いポップオーバーは、以前にビューポートからはみ出していたように、制限された箱からはみ出す。既存のポップオーバーにそれほど高いものは知られていない。`web/design-exceptions.js` の `popover.tsx` の項目にはそのクラスが加わり、`task generate` がレジストリのアイテムを作り直す。
- ホバーは画面の幅ではなく `pointerType` から読む。これで、幅は CSS で読むという library-ui.md の規則を保つ。
- 一覧は `body` にポータルで描画されるので、その上で止まったポインタはカードを離れており、どのポインタが離れたときとも同じくホバーのプレビューは止まる ([010 UI 設計、Interaction](../010-hover-video-preview/ui-design.md#interaction) の規則 4)。そこは何も変わらない。タグを読んでいる間、プレビューは付随的なものである。
- 一覧の見た目、チップの折り返した形、ポップオーバーの幅の段階、`+N` のホバー状態は design 段階 (`ui` ラベル) が決める。次に `ui-design.md` を書き、下の単位はそれに従う。
- [quickstart.md](quickstart.md) は、30 個のタグを持つ動画で 1280×800 と 390×844 のときの受け入れ条件 1 から 5 をたどる。`task check` と `task test-e2e` は動作を確かめ、一覧の見た目は確かめない。

## Constitution Check {#constitution-check}

| 関門 | 判定 |
| --- | --- |
| Radix は `web/src/ui` の下でだけ import する。生の `<button>` を書かず、例外の一覧にない任意値のクラスを書かない (design-system.md "Checks and exceptions") | 合格。`CardTagRow` は引き続き `Popover` と `Button` を使う。Radix の変数を使うクラス 1 つは、その `special` の項目とともに `popover.tsx` に入る |
| 幅は JavaScript ではなく CSS で読む (library-ui.md) | 合格。ホバーのプレビューと同じく、フックは `pointerType` で分岐する |
| どの画面もデザインシステムから作る。足りないコンポーネントは先にそこへ加える (design-system.md) | 合格。新しいコンポーネントはない。`Popover` と `Badge` の形の `Button` で一覧を組み立て、design 段階がその組み立てを記録する |
| 文書は現在を記述する (core-beliefs.md) | 合格。各単位は新しい一覧に合わせて 014 の Overflow 節と library-ui.md の Cards の表を更新する |
| 画面の文言は英語のカタログに置く (i18n.md) | 合格。新しい文言はない。`+N` はアクセシブルな名前を保つ |

この判定は Phase 1 の後も成り立つ。Complexity Tracking に載せる違反はない。

## プロジェクトの構成 {#project-structure}

### 文書 (この機能) {#documentation-this-feature}

```text
specs/041-tag-overflow-list/
├── plan.md          # This file
│                    # No spec.md — the parent Issue is the specification
├── research.md      # R-1 to R-4
├── ui-design.md     # Written by the design stage (ui label)
└── quickstart.md    # Acceptance criteria 1 to 5 with a 30-tag video at two widths
```

`data-model.md` も `contracts/` もない (技術的な文脈を参照)。

### ソースコード {#source-code}

**影響する境界**:

| 境界 | 変わるもの |
| --- | --- |
| `web/src/library` | `CardTagRow.tsx`: 一覧のレイアウト、一覧の中で省略しないチップの形、ホバーのつなぎ込み。新しいホバーの意図を読むフックとそのテスト。`CardTagRow.test.tsx` |
| `web/src/ui/shadcn/popover.tsx`、`web/design-exceptions.js`、`web/registry/r/` (生成物) | 利用可能な高さの制限とその例外の項目。作り直した `popover` アイテム |
| `web/e2e/tags.e2e.ts` | 1280×800 と 390×844 での受け入れ条件 1 から 5 のブラウザのテスト |
| `web/e2e/media-fixtures.mjs` | `generateTagsFixtures` は、1280×800 のグリッドの最後の行がビューポートの下端に来るだけの数の動画を作る。`tags.e2e.ts` は 4 ではなく新しい数を期待する |
| `specs/014-video-tags/ui-design.md`、`docs/design-docs/library-ui.md` | Overflow 節は一覧についてこの機能を指す。Cards の表はホバーで開く一覧を挙げる |

**新しいパス**: `web/src/library/useHoverOpen.ts` と `useHoverOpen.test.ts` (名前は `ui-design.md` に従ってよい)。

**構成の決定**: ホバーの意図を読むフックは、ただ 1 つの利用者の隣の `web/src/library` に置く。`web/src/hooks` (レジストリのコンポーネントが共有するフック) には置かず、`Popover` の prop にもしない。ホバーで開く `Popover` はこの 1 つの操作だけである。共有のコンポーネントにそのためのモードを与えると、どのポップオーバーも使わないタイミングを抱えることになる。2 つ目の利用者が現れたら、フックは `web/src/hooks` へ移す。

## 実装作業 {#implementation-work}

レイアウトを先に入れる。こうするとホバーの単位は最終形の一覧を開き、2 つの単位が `CardTagRow.tsx` の同じ行を同時に編集することはない。

```mermaid
flowchart LR
  layout["隠れたタグを折り返すチップとして並べる"] --> hover["マウスを止めて一覧を開く"]
```

### カードの隠れたタグを、画面の中に収まる折り返すチップとして並べる {#lay-out-a-cards-hidden-tags-as-wrapping-chips-that-stay-inside-the-screen}

**範囲**: 一覧の折り返すレイアウト、`PopoverContent` の高さの制限とその例外の項目と作り直したレジストリのアイテム、一覧の中で省略しないチップ、行の現在のタグから導く一覧 ([research.md R-2](research.md#r-2-the-list-is-a-wrapping-row-of-chips-that-scrolls-inside-the-viewport)、[R-3](research.md#r-3-chips-in-the-list-never-truncate-a-long-name-wraps-inside-its-chip)、[R-4](research.md#r-4-the-list-is-derived-from-the-current-tags-never-copied-when-it-opens))。見た目は `ui-design.md` に従う。受け入れ条件 1 と 5 のブラウザのテストと、下端と右端に近いカードの境界条件のブラウザのテスト。それらの境界条件に必要な `generateTagsFixtures` の追加の動画 (フィクスチャは今は 4 個を作り、下端にカードを置くには少なすぎる)。014 の Overflow 節と library-ui.md の Cards の表の更新。

**依存**: なし

**受け入れ**: `task check` と `task check-docs` が通る。`tags.e2e.ts` に対する `task test-e2e` で、30 個のタグを持ち、そのうち 1 つが 60 文字の動画について、1280×800 と 390×844 で、`+N` を押すと外接矩形がビューポートの中にある一覧が開き、一覧をスクロールすると最後のタグが見え、それを押すとライブラリが絞り込まれて一覧が閉じる (受け入れ条件 1)。60 文字の名前は省略されずに複数の行で描画され、`…` はない (受け入れ条件 5)。1280×800 のグリッドの最後のカードでは、一覧は `+N` の上か横に開き、それでもビューポートの中にある (境界条件)。`CardTagRow.test.tsx` で、一覧が開いている間にタグを減らして再描画すると、なくなったタグが一覧から消え、すべてのタグが収まるように再描画すると `+N` と一覧がアンマウントされる (R-4)。画面が変わるので、実装 PR は `ui-design.md` のレビュー基準に照らした両方の幅の一覧のスクリーンショットを載せる。

### `+N` の上でマウスを止めて、カードの隠れたタグの一覧を開く {#open-a-cards-hidden-tag-list-by-resting-the-mouse-on-n}

**範囲**: ホバーの意図を読むフックと、`CardTagRow` の `Popover` へのそのつなぎ込み。止まったマウスのポインタで 400 ms 後に開き、`+N` と一覧を離れて 200 ms 後に閉じ、ホバーで開いた一覧ではフォーカスを動かさず、押す操作は変わらない ([research.md R-1](research.md#r-1-resting-the-mouse-on-n-opens-the-same-popover-that-pressing-opens))。`ui-design.md` による `+N` のホバー状態。受け入れ条件 2、3、4 のブラウザのテスト。

**依存**: カードの隠れたタグを、画面の中に収まる折り返すチップとして並べる

**受け入れ**: `task check` が通る。偽のタイマーを使うフックの単体テストが次を示す。マウスのポインタが入ると 400 ms 後に開き、399 ms では開かない。その前に離れると取り消される。`+N` から一覧へ移り、200 ms 以内に入り直すと開いたままである。両方を 200 ms 離れると閉じる。`touch` のポインタでは開かない。1280×800 での `tags.e2e.ts` に対する `task test-e2e` で、マウスを `+N` の上に動かして待つとクリックなしで一覧が開き、一覧の中へ動かしてチップを押すと絞り込んで一覧を閉じる (受け入れ条件 2)。100 ms 以内に `+N` を横切って離れると開かない (受け入れ条件 3)。`Escape` と外側を押す操作はホバーで開いた一覧を閉じる。ホバーで開いた後も `document.activeElement` は変わらない。390×844 でタッチをエミュレートすると、`+N` をタップすると一覧が開く。キーボードでは、`Tab` で `+N` へ移って `Enter` を押すと開き、`Tab` で最初のチップに届き、`Enter` で絞り込む (受け入れ条件 4)。画面が変わるので、実装 PR は `ui-design.md` に照らしたホバー状態の `+N` のスクリーンショットを載せる。
