# 契約: 見た目のトークン

**Feature**: [../spec.md](../spec.md) | **Plan**: [../plan.md](../plan.md) |
**Research**: [R-401](../research.md) / [R-402](../research.md) / [R-405](../research.md)

FR-001 は「配色・余白・文字の大きさ・角の丸みの規則を1か所に定義し、画面ごとに別の値を
持たないこと」を求めている。その1か所が `web/src/index.css` の `@theme` であり、本書は
**そこに置く名前と値、および守るべき対比**を定める。

この表と `web/src/index.css` が食い違ったら、**CSS が正しい**。本書は読み手のための写しで
あり、対比の検査（R-405）は CSS の側を読む。

## 1. 置き場所と使い方

```css
/* web/src/index.css */
@import "tailwindcss";
@config "../tailwind.config.ts";

@theme {
  --color-surface: #0f1115;
  /* ... 以下の表のとおり ... */
}
```

- 画面と部品は**トークン名から生成される実用クラス**（`bg-surface`・`text-muted`・
  `border-border`・`rounded-card` など）だけを使う。
- 生の 16 進色、生の `px`、Tailwind 既定のパレット（`neutral-*`・`sky-*`・`red-*`）を
  `web/src/**` に書かない。**この禁止が FR-001 の実体**である。
- 間隔（`gap-4`・`p-6`）と文字の大きさ（`text-sm`）は Tailwind の既定の尺度を使う。
  尺度そのものは規則であり、値を画面ごとに上書きしないことがここでの要求である。

## 2. 色

暗い配色だけを定義する（R-402）。名前は**役割**であって色名ではない。

| トークン | 値 | 使う場所 |
| --- | --- | --- |
| `--color-surface` | `#0f1115` | 画面の地。`body` の背景 |
| `--color-surface-raised` | `#191d24` | 上に乗るもの: 固定の帯、情報欄、空・失敗の枠、入力欄 |
| `--color-surface-sunken` | `#07090c` | 映像の背後（レターボックス）、サムネイル未生成の枠 |
| `--color-badge` | `#12151a` | サムネイルの上に置く不透明の小片（長さ・状態）。**半透明にしない**（R-404 と同じ理由で、背後の画像により対比が変わる） |
| `--color-border` | `#626d7d` | 操作できる要素の境界、区切り線 |
| `--color-body` | `#e8ecf2` | 本文・題名 |
| `--color-muted` | `#a7b1c0` | 補助の文言（件数、長さ、項目名） |
| `--color-accent` | `#63a9f2` | リンク、いま選んでいる状態、進捗の帯 |
| `--color-accent-ink` | `#07131f` | `--color-accent` の上に乗る文字 |
| `--color-danger` | `#ff9b9b` | 失敗の文言 |
| `--color-danger-surface` | `#2c1517` | 失敗の枠の地 |
| `--color-warning` | `#f3c274` | 警告（再生できない形式）の文言 |
| `--color-warning-surface` | `#2b2113` | 警告の枠の地 |
| `--color-focus` | `#8ab4ff` | 狙いを合わせている印（フォーカスリング） |

### 対比（FR-004 / SC-006）

検査する組はこの表がすべてである。**トークンを足したら、この表にも足す。**表に無い組は
検査されない。

| 前景 | 背景 | 必要 | 実測 | 判定 |
| --- | --- | --- | --- | --- |
| `body` | `surface` | 4.5 | 15.94 | OK |
| `body` | `surface-raised` | 4.5 | 14.25 | OK |
| `body` | `badge` | 4.5 | 15.43 | OK |
| `muted` | `surface` | 4.5 | 8.72 | OK |
| `muted` | `surface-raised` | 4.5 | 7.80 | OK |
| `muted` | `surface-sunken` | 4.5 | 9.20 | OK |
| `body` | `surface-sunken` | 4.5 | 16.81 | OK |
| `accent` | `surface` | 4.5 | 7.64 | OK |
| `accent` | `surface-raised` | 4.5 | 6.83 | OK |
| `accent-ink` | `accent` | 4.5 | 7.57 | OK |
| `danger` | `danger-surface` | 4.5 | 8.49 | OK |
| `danger` | `surface` | 4.5 | 9.37 | OK |
| `warning` | `warning-surface` | 4.5 | 9.61 | OK |
| `warning` | `surface` | 4.5 | 11.50 | OK |
| `border` | `surface` | 3.0 | 3.60 | OK |
| `border` | `surface-raised` | 3.0 | 3.22 | OK |
| `focus` | `surface` | 3.0 | 9.05 | OK |
| `focus` | `surface-raised` | 3.0 | 8.09 | OK |
| `accent` | `surface-sunken` | 3.0 | 8.06 | OK |

「必要」は FR-004 による（本文 4.5:1、境界と大きな文字 3:1）。「実測」は sRGB の相対輝度
から求めた値で、単体テストが同じ計算を CSS の値に対して行う。

## 3. 形と大きさ

| トークン | 値 | 使う場所 |
| --- | --- | --- |
| `--radius-card` | `0.75rem` | 一覧の項目、映像、枠 |
| `--radius-control` | `0.5rem` | ボタン、入力欄、選択欄 |
| `--size-tap` | `2.75rem`（44px） | 押せる要素の最小の当たり判定（FR-022） |
| `--size-tile-dense` | `9rem`（144px） | 密度「細かい」の最小列幅 |
| `--size-tile-standard` | `11rem`（176px） | 密度「標準」の最小列幅 |
| `--size-tile-relaxed` | `16rem`（256px） | 密度「ゆったり」の最小列幅 |

### 密度と格子（FR-017 / FR-022 / R-411）

```css
grid-template-columns:
  repeat(auto-fill, minmax(min(var(--tile-min), (100% - var(--tile-gap)) / 2), 1fr));
```

`--tile-min` は密度に応じて上の 3 つのどれかを指す。`min(..., (100% - gap) / 2)` を挟むのは、
**どの画面幅でも列が 1 本にならないことを保証する**ためである。幅 360px では最小列幅が
実質 156px になり、密度によらず 2 列で並ぶ。横方向のスクロールは発生しない（SC-004）。

その結果、**狭い画面では 3 つの密度の見た目が同じになる**。これは意図した動作で、
「狭い画面で 1 列まで大きくする」ことに利用者の利益が無いためである。

| 画面幅 | 細かい | 標準 | ゆったり |
| --- | --- | --- | --- |
| 360px | 2 | 2 | 2 |
| 768px | 4 | 3 | 2 |
| 1280px | 7 | 6 | 4 |
| 2560px | 15 | 12 | 9 |

（`--tile-gap` = `1rem`、左右の余白 = `1rem` としたときの目安。実測値は
[../quickstart.md](../quickstart.md) S4 で確かめる。）

## 4. 動き（FR-023 / R-410）

- 装飾的な遷移（ホバー、骨組み表示の明滅、進捗帯の伸び）は `motion-reduce:` 変種で
  無効にする。
- 無効にするのは**動き**だけである。狙いを合わせた印（`--color-focus`）と、色・不透明度の
  最終状態は常に適用する。
- スクロール位置の復元と密度変更後の位置合わせは、常に瞬時（`behavior: "auto"`）で行う。

## 5. この契約の検査

| 事項 | 検査 |
| --- | --- |
| 対比表のすべての組が基準を満たす | 単体テスト（`web/src/index.css` を読んで計算） |
| 画面に生の色が書かれていない | 単体テスト（`web/src/**/*.tsx` を走査し、`#rrggbb` と Tailwind 既定のパレット名を禁じる） |
| 列数と横スクロールの有無 | 受け入れ検証 [S4](../quickstart.md)（人が幅を変えて見る） |
