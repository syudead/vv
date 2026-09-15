# 契約: 見た目のトークン（004 への差分）

**Feature**: [../spec.md](../spec.md) | **Plan**: [../plan.md](../plan.md) |
**Research**: [R-504](../research.md)

本書は [004 の contracts/design-tokens.md](../../004-library-ui/contracts/design-tokens.md) を
**置き換えない**。その契約（置き場所は `web/src/index.css` の `@theme`、画面は実用クラス
だけを使う、生の色と既定パレット名を書かない）はそのまま有効で、ここには**本機能が
変える分と足す分だけ**を書く。

この表と `web/src/index.css` が食い違ったら、**CSS が正しい**。対比の検査は CSS の側を読む。

## 1. 変えるトークン

| トークン | 004 の値 | 005 の値 | 理由 |
| --- | --- | --- | --- |
| `--color-accent` | `#63a9f2`（青） | `#5fd4e4`（シアン） | FR-010。原案のアクセントに合わせる |

`--color-accent-ink`（`#07131f`）は**変えない**。新しい accent の上で 10.72 あり、替える
理由が無い（[R-504](../research.md)）。

## 2. 足すトークン

| トークン | 値 | 使う場所 |
| --- | --- | --- |
| `--color-accent-surface` | `#12262b` | 選択中の NavItem の地、選択中の Tab の下線の周り |
| `--color-inert` | `#6b7482` | **表示のみの要素**の文字とアイコン（FR-005 の「淡く表示」） |
| `--size-sidebar` | `15rem`（240px） | サイドバー（C1）の幅 |
| `--size-header` | `3.5rem`（56px） | ヘッダー（C5）の高さ |

`--size-sidebar` と `--size-header` をトークンにするのは、**3 か所で使うから**である
（サイドバー自身の幅、コンテンツの左の余地、ツールバーが粘る位置）。値を直に書くと
3 か所がずれる。

## 3. 対比（FR-011 / SC-006）

004 の表に**次の組を足す**。`web/src/theme/tokens.test.ts` の `pairs` にも同じ 6 行を
足すこと。**表に無い組は検査されない。**

| 前景 | 背景 | 必要 | 計算値 | 判定 |
| --- | --- | --- | --- | --- |
| `accent` | `accent-surface` | 4.5 | 8.99 | OK |
| `body` | `accent-surface` | 4.5 | 13.24 | OK |
| `muted` | `accent-surface` | 4.5 | 7.25 | OK |

`--color-accent` の値が変わることで、004 の表のうち accent を含む 4 行の実測値も変わる。
**行は増えないが、値は書き換える。**

| 前景 | 背景 | 必要 | 004 の値 | 005 の値 | 判定 |
| --- | --- | --- | --- | --- | --- |
| `accent` | `surface` | 4.5 | 7.64 | 10.82 | OK |
| `accent` | `surface-raised` | 4.5 | 6.83 | 9.68 | OK |
| `accent` | `surface-sunken` | 3.0 | 8.06 | 11.41 | OK |
| `accent-ink` | `accent` | 4.5 | 7.57 | 10.72 | OK |

「計算値」は sRGB の相対輝度（WCAG 2.1）から求めた値で、単体テストが同じ計算を CSS の
値に対して行う。ここは読み手のための写しである。

## 4. 表示のみの要素の色（FR-011 の対象外）

`--color-inert` は**対比表に入れない**。

| 前景 | 背景 | 対比 |
| --- | --- | --- |
| `inert` | `surface` | 4.00 |
| `inert` | `surface-raised` | 3.58 |

どちらも 4.5:1 を下回る。**これは意図した値である** — spec 4. と FR-011 が
「表示のみの要素は淡く表示するため、この基準の対象外とする」と明示している。淡く見える
ことが要求なので、基準を満たす値にすると要求を満たせない。

ただし**片側だけは機械で守る**。値を濃くしすぎて `muted`（機能する要素の補助文言）と
見分けが付かなくなる方向は、明暗の順序として検査できる。

```text
相対輝度: inert < muted
```

`web/src/theme/tokens.test.ts` にこの 1 件を足す。淡くしすぎる方向（読めなくなる）は
人が [S3](../quickstart.md) で確かめる。

## 5. 変えないもの

| 事項 | 備考 |
| --- | --- |
| `--color-surface` / `-raised` / `-sunken` / `--color-badge` | サイドバーとヘッダーの地は `--color-surface-raised`（「上に乗るもの」）。新しい面のトークンは足さない |
| `--color-border` / `--color-body` / `--color-muted` / `--color-focus` | そのまま |
| `--color-danger*` / `--color-warning*` | そのまま。C13 Notice の 3 段階は `info` = `surface-raised`、`warning`、`danger` の既存の組で足りる |
| `--radius-card` / `--radius-control` / `--size-tap` | そのまま。カード全体の角丸（spec 4.）は `--radius-card` を枠に移すだけで、値は変えない |
| `--size-tile-dense` / `-standard` / `-relaxed` と格子の式 | そのまま。本機能は列の式に触れない |
| 動き（`motion-reduce:` の扱い） | 004 の契約 4. のまま（FR-012） |

## 6. この契約の検査

| 事項 | 検査 |
| --- | --- |
| 3. の表のすべての組が基準を満たす | 単体テスト（`web/src/index.css` を読んで計算） |
| `inert` が `muted` より暗い | 単体テスト（同上） |
| 画面に生の色が書かれていない | 単体テスト（004 の `noRawColors.test.ts` がそのまま効く） |
| 表示のみの要素が「淡いが読める」こと | 受け入れ検証 [S3](../quickstart.md)（人） |
| バッジがサムネイルの明暗によらず読めること（FR-009） | 受け入れ検証 [S4](../quickstart.md)（人）。`--color-badge` は不透明のまま |
