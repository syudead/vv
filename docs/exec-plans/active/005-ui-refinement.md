# 実行計画: 原案デザインに合わせた UI の再構築

- ステータス: 進行中（Phase 4 まで。Phase 5 以降は未着手）
- 最終更新: 2026-09-15
- 対象: [spec](../../../specs/005-ui-refinement/spec.md) の全体（FR-001〜FR-018 / SC-001〜SC-009）。004 で作った 2 画面を、原案の 3 領域（サイドバー・ヘッダー・コンテンツ）の構図に組み替える。変更は **`web/src/` に閉じる**（`api/openapi.yaml`・`internal/`・`cmd/`・`web/src/api/gen/` には触れない）

## 目的

004 の画面は規則（トークン・4 状態・キーボード・読み上げ）を満たしているが、**構図が
原案と違う**。ヘッダーもサイドバーも無く、ツールバーとグリッドがページに直置きされて
いる。本機能はその差分を、**機能を 1 つも増やさずに**埋める。

サーバーの API も、保存するデータも、利用者が実際に行える操作の種類と数も変えない
（FR-013・FR-014）。原案にあって裏側の無い入口（コレクション・タグ・お気に入り・
未整理・画像タブ）は、`<button disabled>` でも `aria-disabled` でもなく
**対話要素にしない**形で置く（[R-503](../../../specs/005-ui-refinement/research.md)）。
依存は実行時・開発時とも 1 つも増やさない — アイコンは外部の集を入れず、インラインの
`<svg>` で持つ（[R-510](../../../specs/005-ui-refinement/research.md)）。

## 一次資料

詳細はすべて Spec Kit の成果物にある。本書は進捗と決定の記録に徹する。

| 文書 | 内容 |
| --- | --- |
| [spec.md](../../../specs/005-ui-refinement/spec.md) | 要件と成功基準（FR-001〜FR-018 / SC-001〜SC-009） |
| [plan.md](../../../specs/005-ui-refinement/plan.md) | 技術的な進め方、ゲート判定、配置 |
| [research.md](../../../specs/005-ui-refinement/research.md) | 未確定事項の解消（R-501〜R-511） |
| [data-model.md](../../../specs/005-ui-refinement/data-model.md) | 画面が持つ状態と、ナビゲーションの静的な表 |
| [contracts/](../../../specs/005-ui-refinement/contracts/) | [layout.md](../../../specs/005-ui-refinement/contracts/layout.md)（3 領域の寸法・境界・スクロールの持ち主）・[design-tokens.md](../../../specs/005-ui-refinement/contracts/design-tokens.md)（004 の契約への差分）・[components.md](../../../specs/005-ui-refinement/contracts/components.md)（C1〜C16 と表示のみの要素の規約） |
| [quickstart.md](../../../specs/005-ui-refinement/quickstart.md) | 受け入れの検証手順（S0〜S9） |
| [tasks.md](../../../specs/005-ui-refinement/tasks.md) | タスク分解（T001〜T046、Phase 1〜8） |

## 検証の方針

[quickstart.md](../../../specs/005-ui-refinement/quickstart.md) の S1〜S9 をもって完了を
判定する。**S2〜S7 は人が確かめる**（レイアウトと画面幅・サイドバーとヘッダー・動画
カード・ツールバーと通知・再生画面・キーボードと読み上げ）。自動化しない。

構図そのもの（SC-001）と画面幅による出し分け（SC-003・SC-007）に機械の検査を用意しない
のは、jsdom が CSS を適用しないためである。`position: fixed`・メディアクエリ・
`grid-template-columns` のどれも確かめられず、擬似的に確かめる仕掛けを作ると「テストは
通るが画面は壊れている」状態を招く（[R-511](../../../specs/005-ui-refinement/research.md)）。

| シナリオ | 手段 |
| --- | --- |
| S0 | 検証用の動画をつくる（[004 の S0](../../../specs/004-library-ui/quickstart.md) をそのまま実行。人の準備作業） |
| S1 | 機械。`make test-web` と `make check` |
| S2〜S7 | **人**。ブラウザで `make up` の画面を原案と並べて確かめる |
| S8 | 機械 + 人。004 の受け入れ基準が引き続き通ること（FR-018 / SC-009）。004 の S4 だけは S2・S6 が期待値を更新して引き取る |
| S9 | 画面を 6 枚記録し、PR に添える（AGENTS.md の取り決め・G8） |

## 進捗

| 区分 | 状態 |
| --- | --- |
| 仕様 | 完了（2026-09-15） |
| 計画・設計成果物 | 完了（2026-09-15） |
| タスク分解 | 完了（2026-09-15）。T001〜T046 |
| Phase 1: Setup（実行計画と着手前の緑） | 完了（2026-09-15）。T001〜T002 |
| Phase 2: Foundational（トークン・表・アイコン） | 完了（2026-09-15）。T003〜T006 |
| Phase 3: US1（レイアウトが原案の 3 領域になる） | 完了（2026-09-15）。T007〜T014。S2 の寸法・境界・固定は実ブラウザで確認済み。原案との照合（SC-001）は S3 で人が行う |
| Phase 4: US2（サイドバーとヘッダーが原案どおりに並ぶ） | 完了（2026-09-15）。T015〜T022。機能する入口が 2 つだけであることは機械で守られている（`layout/placeholders.test.tsx`）。原案との照合（順序・体裁）は S3、読み上げは S7 で人が行う |
| Phase 5: US3（動画カードが原案の形になる） | 完了（2026-09-15）。T023〜T025。サムネイルと題名が 1 つの枠にまとまり、角丸が枠全体に掛かる。尺が未取得のときにバッジを出さないことは機械で守られている（`components/VideoCard.test.tsx`）。原案との照合と明るいサムネイル上での可読性（FR-009）は S4 で人が行う |
| Phase 6: US4（ツールバーと通知を整える） | 未着手。T026〜T032 |
| Phase 7: US5（再生画面が動画と情報パネルの 2 分割になる） | 未着手。T033〜T037 |
| Phase 8: Polish & Cross-Cutting Concerns | 未着手。T038〜T046 |
| 受け入れ検証（S1） | 各フェーズで実施（`make check` が緑） |
| 受け入れ検証（S2） | Phase 3 の範囲（3 領域・640/639 の境界・固定・密度アンカー）は実施済み。原案との照合は S3 で行う |
| 受け入れ検証（S0・S3〜S9） | 未実施 |

## 着手前の検査の結果（Phase 1 / T002）

実行日: 2026-09-15。実行環境は Claude Code on the web のセッション（Linux コンテナ）。
`web/src/` を 1 行も書き換えていない状態、すなわち **004 の完了時点（`main` の
`168dfbd`）** で実行した。

| 検査 | 結果 |
| --- | --- |
| `make test-web` | **成功**。`vite build` が通り、13 ファイル / 114 件のテストが緑 |
| `make check` | **成功**。`fmt-check`（Go の `gofmt` / Web の Prettier）→ `lint`（`golangci-lint` / `tsc --noEmit`）→ `test`（Go 7 パッケージが `ok`・テストを持たないもの 2 つ / Web 114 件 / SDD ハーネス 21 件）→ `generate-check` がすべて成功 |

**この記録が要る理由**（FR-018 / SC-009）: 本機能は `web/src/` のほぼ全域に触れる。以降の
フェーズで検査が落ちたとき、この記録が無いと「本機能が壊した」のか「元から赤かった」の
かを区別できない。004 の受け入れ基準を引き続き満たすこと（FR-018）は成功基準そのもの
（SC-009）なので、起点が緑であることを先に確定させておく。

`generate-check` が成功していることは、`api/openapi.yaml` と生成物（`web/src/api/gen/`・
Go 側）が一致していることを示す。以降のフェーズでも `generate-check` が通り続けることが、
FR-014（契約を変えない）に対する機械的な裏付けになる。

## 決定の記録

<!-- Phase 2 以降で下した判断をここに追記する。書式は 004 の実行計画に合わせる -->

### 実行計画は implement の最初の回で作る（T001）

[plan.md](../../../specs/005-ui-refinement/plan.md) の Structure Decision が定めるとおり、
本書は plan 段階の成果物ではなく **implement の最初の回（Phase 1）** で作る。004 の実行
計画も Phase 1 の回（`e764a2a`）で作られており、plan 段階の成果物は Spec Kit 側
（`plan.md`・`research.md`・`data-model.md`・`contracts/`・`quickstart.md`）に限られる。
plan の PR に本書が無いのは漏れではない。

### 「表示のみ」は型で塞ぐ（T006）

[data-model.md 2.](../../../specs/005-ui-refinement/data-model.md) の不変条件
「`kind: "inert"` の行は `to` を持たない」を、規約ではなく**型**で表した。
`InertNavItem` の `to` を `to?: never` としてあるので、表示のみの行に行き先を書くと
`tsc --noEmit`（`make lint`）が落ちる。確認のため一時ファイルで実際に落ちること
（`TS2322: Type 'string' is not assignable to type 'undefined'`）を見てから消した。

行き先が付く＝押せてしまうということで、それは利用者が行える操作が増えたということ
（FR-013 の違反）である。描画側のレビューで気付く形にすると、行を足すたびに人が見る
必要が残る。

### `--color-inert` は対比表に入れない（T003・T004）

[contracts/design-tokens.md](../../../specs/005-ui-refinement/contracts/design-tokens.md) 4.
のとおり、`--color-inert`（`#6b7482`）は `surface` に対して 4.00、`surface-raised` に
対して 3.58 で、どちらも 4.5:1 を下回る。**これは意図した値である** — FR-011 が
「表示のみの要素は淡く表示するため、この基準の対象外とする」と明示しており、淡く
見えることが要求だからである。したがって `tokens.test.ts` の `pairs` には入れない。

かわりに片側だけを機械で守る。値を濃くしすぎて `muted`（機能する要素の補助文言）と
見分けが付かなくなる方向は、相対輝度の順序（`inert` < `muted`）として検査できる。
淡くしすぎる方向（読めなくなる）は人が [S3](../../../specs/005-ui-refinement/quickstart.md)
で確かめる。

### アイコンは 11 個をインラインで持つ（T005）

[R-510](../../../specs/005-ui-refinement/research.md) のとおり外部のアイコン集を入れず、
`web/src/layout/icons.tsx` に 11 個（`film`・`clock`・`heart`・`folder`・`tag`・
`search`・`filter`・`settings`・`grid`・`list`・`close`）をインラインの `<svg>` で
置いた。**依存は実行時・開発時とも 1 つも増えていない**（`web/package.json` は無変更）。

`aria-hidden="true"` と `focusable="false"` は `Icon` が一律に付け、呼び出し側には
書かせない。呼び出し側に任せると、後から足したアイコンが読み上げに漏れるためである。
色は指定せず `currentColor` に任せるので、囲みの文字色（`text-inert` など）がそのまま
伝わり、`theme/noRawColors.test.ts` の走査も素通りする。

## Phase 2 の検査の結果

実行日: 2026-09-15。

| 検査 | 結果 |
| --- | --- |
| `make test-web` | **成功**。`vite build` が通り、13 ファイル / **119 件**のテストが緑 |
| `make check` | **成功**。`fmt-check` → `lint` → `test` → `generate-check` がすべて成功 |

件数が 114 件から 119 件に増えた内訳は、対比の 3 組（`accent` / `body` / `muted` に
対する `accent-surface`）、`inert` < `muted` の明暗の順序 1 件、そして
`noRawColors.test.ts` の走査対象に `layout/icons.tsx` が 1 つ加わった分である。
`generate-check` が通っているので、`api/openapi.yaml` と生成物は変わっていない
（FR-014）。

## Phase 3 の検査の結果

実行日: 2026-09-15。

| 検査 | 結果 |
| --- | --- |
| `make test-web` | **成功**。`vite build` が通り、13 ファイル / **123 件**のテストが緑 |
| `make check` | **成功**。`fmt-check` → `lint` → `test` → `generate-check` がすべて成功 |

件数が 119 件から 123 件に増えたのは、`noRawColors.test.ts` の走査対象に
`layout/Logo.tsx`・`layout/Sidebar.tsx`・`layout/Header.tsx`・`layout/AppShell.tsx` の
4 ファイルが加わった分である。**Phase 3 で新しい単体テストは足していない** —
[tasks.md](../../../specs/005-ui-refinement/tasks.md) が足すと定めた 4 件はいずれも
Phase 4 以降のもので、構図そのもの（SC-001）と画面幅による出し分け（SC-003・SC-007）には
[R-511](../../../specs/005-ui-refinement/research.md) のとおり機械の検査を用意しない。

`generate-check` が通っているので、`api/openapi.yaml` と生成物は変わっていない
（FR-014）。差分は `web/src/` に閉じている。

### 画面の記録と、そこから読み取れたこと

[docs/how-to/ui-change-screenshots.md](../../how-to/ui-change-screenshots.md) の
「コンテナ内・エージェントの実行環境」の手順で 2 枚を撮った（確認用の動画 6 本は
[S0](../../../specs/005-ui-refinement/quickstart.md) のとおり ffmpeg の testsrc で作った）。

| 画像 | 幅 | 見えること |
| --- | --- | --- |
| `20260915-005-p3-library-1280.png` | 1280 | サイドバー・ヘッダー・コンテンツの 3 領域。帯がヘッダーの下に粘り、件数が帯の右端にある |
| `20260915-005-p3-library-360.png` | 360 | サイドバーが消え、ロゴがヘッダーへ移る。帯は折り返し、横スクロールは出ない |

列数は 1280 の標準で **5 列**、360 で **2 列**であり、
[contracts/layout.md](../../../specs/005-ui-refinement/contracts/layout.md) 1. の表
（1280 標準 = 5、360 = 2）と一致した。T041（004 の表の更新）は Phase 8 の作業なので
ここでは行わない。**この 2 点が一致したことの記録**にとどめる。

### 実ブラウザでの確認（S2 / 004 の S6-2）

レビューの指摘（下記）を受け、確認用の動画を 48 本に増やして
[quickstart.md](../../../specs/005-ui-refinement/quickstart.md) S2 の残りを
実ブラウザ（Chromium）で確かめた。結果は次のとおりで、いずれも期待どおりである。

| 確認 | 対応 | 結果 |
| --- | --- | --- |
| 幅 640 でサイドバーが出る | spec US1-4 / contracts/layout.md 2. | `display: block`、ヘッダーの左端 240px |
| 幅 639 でサイドバーが消える | 同上 | `display: none`、ヘッダーの左端 0px |
| 読み上げに届くロゴ（`h1`）が各幅でちょうど 1 つ | FR-003 / R-502 | 640 はサイドバー側、639 はヘッダー側の 1 つだけ |
| スクロールしても固定領域が動かない | FR-002 | ヘッダー・サイドバーとも `top` が 0 のまま |
| 密度を変えたあと、戻した項目が固定領域の裏に隠れない | 004 の S6-2 / T014 | 控えた項目の上端が固定領域の下端（117px）と一致 |
| 短い一覧で余分な縦スクロールが出ない | 下記の修正 | 文書の高さが画面と同じ（余分 0px） |

**`h1` が各幅で 1 つだけ**であることは、ロゴを 2 か所に置く設計（R-502）の要である
「`display: none` は支援技術からも消える」を実際に裏づけている。

密度アンカーの結果（項目の上端が固定領域の下端に**ちょうど**着く）は、T014 の
「`--size-header` + ツールバーの実測高」がそのまま逃げ幅として効いていることを示す。

なお **SC-001（原案との照合）と 4 状態の見え方は引き続き人が確かめる**（S3）。ここで
機械に寄せたのは真偽がはっきりする項目だけで、[R-511](../../../specs/005-ui-refinement/research.md)
の判断（構図の一致を機械の検査にしない）は変えていない。

## 決定の記録（Phase 3）

### 固定領域の下端は「実測」で取る（T014）

[contracts/layout.md](../../../specs/005-ui-refinement/contracts/layout.md) 1. の

```text
固定領域の下端 = --size-header + ツールバーの実測高
```

のうち、`--size-header` の側も**描かれたヘッダーを測って**得ることにした
（`layout/Header.tsx` の `headerHeight()` が `[data-app-header]` を測る）。56px という値を
JavaScript 側に書き写すと、トークンを変えたときにここだけ古い値が残るためである。
`getComputedStyle` でカスタムプロパティを読んで `rem` を px に直す案も採らなかった —
単位の解釈を自前で持つことになり、jsdom では値が取れないので結局分岐が要る。

測る対象が無いとき（再生画面、骨格を持たずに描くテスト）は 0 を返す。ヘッダーが
無いのだから、その裏に隠れる項目も無く、逃げる高さも 0 でよい。

### 復元と無限スクロールには触っていない（T014 / FR-018）

`window.scrollY` の保存・復元（`api/listSnapshot.ts` と `LibraryPage` の 2 つの
`useLayoutEffect`）と `IntersectionObserver` による続きの読み込みは**差分に現れない**。
スクロールの持ち主を文書のままにした（[R-501](../../../specs/005-ui-refinement/research.md)）
ので、004 が積んだ 3 つの仕掛けのうち固定領域の高さに依存するのは密度アンカーだけであり、
そこだけを直せば足りる。触っていないことが差分から読み取れることが、FR-018 のいちばん
強い根拠である。

### 画面を満たす役目は骨格だけが持つ（レビュー指摘の修正）

PR #36 のレビューで、**件数が少ない一覧に 56px の余分な縦スクロールが出る**ことを
指摘された。`AppShell` が `padding-top: var(--size-header)` を与えているところに、
`LibraryPage` の根も `min-h-dvh` を持っていたため、文書の最低の高さが
`100dvh + --size-header` になっていた。

実ブラウザで再現し（画面 800px に対して文書 856px、余分 56px）、`min-h-dvh` を骨格の
1 か所へ寄せて解消した（修正後は余分 0px）。`box-sizing: border-box` なので、骨格が
`min-h-dvh` と上余白の両方を持てば、余白は 100dvh の内側に収まる。

包む側と包まれる側が同じ「画面を満たす」要求を二重に持つと足し合わさる、という
一般の落とし穴である。骨格を導入した回だからこそ、役目の置き場所を 1 つに決めておく。

## Phase 4 の検査の結果

実行日: 2026-09-15。

| 検査 | 結果 |
| --- | --- |
| `make test-web` | **成功**。`vite build` が通り、14 ファイル / **138 件**のテストが緑 |
| `make check` | **成功**。`fmt-check` → `lint` → `test` → `generate-check` がすべて成功 |

件数が 123 件から 138 件に増えた内訳は、新設の `layout/placeholders.test.tsx` の **7 件**
（[contracts/components.md](../../../specs/005-ui-refinement/contracts/components.md) 5. が
求める 4 つに、「（未実装）」が添えられていることと件数の 2 場面を加えたもの）、
`noRawColors.test.ts` の走査対象に加わった 5 ファイル（`layout/NavItem.tsx`・
`layout/SidebarSection.tsx`・`layout/Tab.tsx`・`layout/IconButton.tsx`・
`layout/placeholders.test.tsx`）、そしてレビュー指摘で足した
`pages/LibraryPage.test.tsx` の **3 件**である。

`generate-check` が通っているので、`api/openapi.yaml` と生成物は変わっていない
（FR-014）。差分は `web/src/` と本書に閉じている。

### 画面の記録と、そこから読み取れたこと

[docs/how-to/ui-change-screenshots.md](../../how-to/ui-change-screenshots.md) の
「コンテナ内・エージェントの実行環境」の手順で 2 枚を撮った（確認用の動画 12 本は
[S0](../../../specs/005-ui-refinement/quickstart.md) のとおり ffmpeg の testsrc で作った。
実データは写していない）。

| 画像 | 幅 | 見えること |
| --- | --- | --- |
| `20260915-005-p4-library-1280.png` | 1280 | サイドバーがロゴ → ライブラリ（見出し無し）→ コレクション → タグの順で並ぶ。「すべての動画」だけが選択中の地と文字色を持ち、右端に件数（12）が出る。残りの 9 行は `--color-inert` で淡い。ヘッダーはタブ 4 つが左、設定が右端 |
| `20260915-005-p4-library-600.png` | 600 | 640px 未満。サイドバーが消え、ロゴがヘッダーへ移る（FR-003）。タブと設定はヘッダーに残る |

**件数が出ているのは 1 行だけ**である。表示のみの 9 行には数字が無く、これは
`layout/placeholders.test.tsx` が機械でも見ている。撮影時点の総件数（12）が
サイドバーと帯の右端の両方で一致しているので、T021 の経路（`LibraryPage` →
`AppShell` の context → `Sidebar`）が実際につながっていることも読み取れる。

原案との照合（順序と体裁が一致していること・4 状態の見え方）は S3、読み上げに
「（未実装）」が届くことは S7 で人が確かめる。画像からは分からない。

## 決定の記録（Phase 4）

### 件数は骨格の中を子から親へ流す（T021）

総件数を持っているのは `useVideos` を呼ぶ `LibraryPage` で、それは `AppShell` の**子**で
ある。出す先の NavItem「すべての動画」は `Sidebar` の中、つまり骨格の別の枝にあるので、
引数では渡せない。`AppShell` の中に件数の context を置き、同じファイルから公開側
（`usePublishVideoCount`）と読み出し側（`useVideoCount`）の hook を export した。

新しいファイルを作らなかったのは、この状態が骨格の外で使われないためである。器と同じ
ファイルに置くほうが、どこまでが骨格の関心かが読み取れる。`web/src/api/useVideos.ts` の
置き場所も戻り値も変えていない（[data-model.md 5.](../../../specs/005-ui-refinement/data-model.md)
の「変わらないもの」）。

値と設定関数は**別の context に分けた**。1 つのオブジェクトにまとめると、件数が変わる
たびに新しいオブジェクトになり、公開側の効果が毎回走り直す。

### 公開するのは `total` ではなく `loading ? undefined : total`（T021）

`useVideos` は `total` を 0 で初期化し、検索語や並び順を変えて取り直すあいだも前の値を
消さない。`total` だけを見ると**初回に「0 本」、絞り込みの最中に前の件数**が出る。どちらも
嘘であり、同じ場面で帯が「読み込み中…」と出しているのとも食い違う（004 の FR-008）。

`undefined` を「まだ分からない」に割り当て、そのあいだは件数を出さないことにした。0 と
未取得が同じ見た目にならない。`layout/placeholders.test.tsx` がこの 2 つ（42 のときは
出る／`undefined` のときは出ない）を両方から見ている。

### 「表示のみ」に件数を渡す口を型で塞いだ（T015 / T021）

`NavItem` の props を判別可能な合併にし、`InertNavItem` を取る側を `count?: never` と
した。表示のみの行へ件数を渡すと `tsc --noEmit` が落ちる。T006 で `to` を `to?: never` に
したのと同じ手口で、**FR-005 の「件数を出さない」を規約ではなく型にした**ものである。

描画側のレビューで気付く形にすると、行を足すたびに人が見る必要が残る。

### hover / active は地ではなく覆いで表す（T015）

contracts/components.md 2. の「背景を 1 段明るく / 暗く」は**地に対する相対**の指示だが、
選択中（`accent-surface`）と通常（サイドバーの `surface-raised`）では地が違う。地の色そのものを
差し替えると、選択中の行にホバーしたときに色みが消える。

`::after` の覆い（`hover:after:bg-body/10` / `active:after:bg-surface-sunken/60`）にすると、
どちらの地の上でも同じ向きに 1 段動き、選択中の色みも残る。`isolate` + `after:-z-10` で
覆いを中身の下に敷いてあるので、文字とアイコンは覆われない。`surface-raised` より明るい
トークンが無いことへの答えでもある（生の色は `theme/noRawColors.test.ts` が禁じている）。

### 検査が本当に効くことを確かめてから残した（T022）

`layout/placeholders.test.tsx` を書いたあと、`InertRow` を一時的に
`<button type="button" disabled>` に変えて実行し、「対話要素になっている」と「tab 順に
現れない」の 2 件が落ちることを見てから戻した。

表示のみの要素の検査は**通ってしまう書き方**になりやすい。最初に書いた
`expect(names).not.toContain(expect.stringContaining(label))` は、`toContain` が
非対称マッチャを解さないため常に通る空の検査だった。壊して落ちることを見なければ、
この種の取り違えは残る。

### レビュー指摘への対応（Phase 4）

PR #37 のレビュー（Devin）が 5 件を挙げ、**4 件を受け入れて直した**。

#### 1. 取得に失敗すると嘘の件数が出る（バグ）

`useVideos` は取得に失敗しても `total` を書き換えない。`loading` だけを見て公開して
いたので、一覧がエラーを出している横でサイドバーが保持された前の件数（初回なら初期値の
0）を名乗っていた。`error !== null && items.length === 0` も `undefined` に倒すことで
直した。

`items.length === 0` が要るのは、**続きのページだけが失敗した場合は、すでに読めている
一覧も総件数も有効**だからである。そこまで隠すと、読めている事実まで取り下げることになる。

これは公開する値を `loading ? undefined : total` にした理由（0 と「まだ分からない」を
同じ見た目にしない）と同じ筋であり、**その理由を最後まで適用していなかった**という
指摘である。`pages/LibraryPage.test.tsx` に 3 件を足し、条件を元に戻すと 2 件が落ちる
ことを見てから残した。

#### 2・3. タブが契約の 4 状態と当たり判定を満たしていない（バグ）

[contracts/components.md](../../../specs/005-ui-refinement/contracts/components.md) 2. は
hover を「背景を 1 段明るく」、active を「1 段暗く」と定め、当たり判定を `--size-tap`
**四方**以上と定めている。タブは下線と文字色だけで状態を示し、幅の下限も持っていなかった
（「動画」は余白込みでも 44px に届かない）。

**契約に寄せた。** 背景の覆いは NavItem と同じ向き・同じトークン由来の色にし、
`min-w-[var(--size-tap)]` を足した。当初は「帯の中の要素に地を敷くと間隔が詰まって
見える」と考えて外していたが、契約は C6 を名指しで 4 状態の対象にしており、見た目の
好みで外してよい規定ではない。四方と書かれているのは、**横に並ぶ要素では幅のほうが先に
足りなくなる**からである。

#### 4. 件数 Context の Provider 欠落を検出できない（受け入れず）

「既定値が何もしない関数なので、骨格の外で `usePublishVideoCount` を呼ぶと黙って
何も起きない」という指摘である。**これは意図した設計なので直さない。**

`pages/LibraryPage.test.tsx`・`LibraryPage.search.test.tsx`・`LibraryPage.restore.test.tsx`
の 3 つは `LibraryPage` を**骨格なしで**描いている。Provider 欠落で投げる作りにすると、
一覧そのものを単体で描けなくなる。件数は一覧の付随情報であって、一覧が成り立つ条件では
ない ── 骨格の外では「件数の出し先が無い」だけで、それは異常ではない。

読み出し側（`useVideoCount`）が骨格の外で `undefined` を返すのも同じ理由である。

#### 5. 機能する入口の検査が自己参照になっていた（受け入れ）

期待値を `navItems` から作っていたため、**別の行を `live` に変えると期待値も一緒に動き、
検査が通ってしまう**状態だった。FR-013 が求めているのは「利用者が行える操作が増えて
いないこと」なので、表とは独立に固定した 2 つ（`all-videos` / `tab-videos`）と突き合わせる
形に直した。

T022 (c) は当初からこの形を求めていた（「`kind: "live"` が **ちょうど 2 つ**である」）ので、
読み違えである。同じ回に「壊して落ちることを確かめる」をやりながら、この 1 件は
`liveItems` を経由していたために壊れても落ちなかった ── 自己参照は、壊し方が
「表を変える」側にあるときだけ効かなくなる。

## Phase 5 の検査の結果

実行日: 2026-09-15。

| 検査 | 結果 |
| --- | --- |
| `make test-web` | **成功**。`vite build` が通り、14 ファイル / **140 件**のテストが緑 |
| `make check` | **成功**。`fmt-check` → `lint` → `test` → `generate-check` がすべて成功 |

138 件から 140 件への 2 件は、T025 で足した時間バッジの 2 場面（尺が取得できているときに
出る／未取得のときに出ない）である。**出る側も一緒に足した** ── 「出ない」だけを検査すると、
探し方（尺の形に一致する文字列）が間違っていても通ってしまう。

`generate-check` が通っているので、`api/openapi.yaml` と生成物は変わっていない
（FR-014）。差分は `web/src/components/VideoCard*`・`docs/` と本書に閉じており、
サーバー側には触れていない。

### 画面の記録と、そこから読み取れたこと

[docs/how-to/ui-change-screenshots.md](../../how-to/ui-change-screenshots.md) の
「コンテナ内・エージェントの実行環境」の手順で 2 枚を撮った（確認用の動画 12 本は
[S0](../../../specs/005-ui-refinement/quickstart.md) のとおり ffmpeg の testsrc で作った。
実データは写していない）。**変更前の 1 枚は、同じ動画のまま変更を戻して撮り直した**もので、
別の回の画像を並べていない（題名が違うと差分が読み取れない）。

| 画像 | 見えること |
| --- | --- |
| `20260915-005-p5-library-1280.png` | 幅 1280 の一覧。12 枚のカードが同じ形に揃い、サムネイルは枠の上端に接して上の 2 隅だけが丸く、題名は枠の中に入って下の 2 隅が丸い。時間バッジは右下 |
| `20260915-005-p5-card-before-after.png` | カード 1 枚の拡大を変更前と並べたもの。変更前は角丸がサムネイルだけに付き題名が枠の外に浮いている。変更後は 1 つの枠にまとまっている |

1280 の画像からは **FR-009 も読み取れる**。`真っ白な雪原`（全面白）と `砂浜の一日`（淡い砂色）
の 2 枚は、サムネイルが明るい端の条件として意図して作ったものである。どちらのバッジも
不透明な `--color-badge` の地のまま読める。

**尺が未取得のカード（バッジが出ない状態）は画像に写せない** ── 取り込みから probe 完了
までの数百ミリ秒しか存在しないためである。そこは T025 の単体テストが機械で見ており、
人の確認としては [S4](../../../specs/005-ui-refinement/quickstart.md) 3 が引き取る。

4 状態（hover / focus / active）の見え方と原案との照合も S4・S3 で人が確かめる。静止画
1 枚からは分からない。

## 決定の記録（Phase 5）

### 枠に `overflow-hidden` を掛けず、角丸を 2 か所で分担する（T023）

「1 つの枠にまとめて枠全体に角丸を掛ける」のを素直に書くと、枠に
`rounded-card overflow-hidden` を置いてサムネイルを流し込む形になる。**これは採らなかった。**

題名の全文への到達手段の 3 つ目（キーボードで狙いを合わせると省略が解け、枠の上へ
**上に伸びて**重なる。004 の R-409）が、枠の高さで切られてしまうからである。長い題名ほど
切られる量が増えるので、いちばん必要な場面で効かなくなる。

かわりに角丸を 2 か所で分担した。

| 場所 | 役目 |
| --- | --- |
| 枠（`Link`）| `rounded-card` + 地（`--color-surface-raised`）。**下の 2 隅**は地が描く |
| サムネイル | `rounded-t-card` + `overflow-hidden`。**上の 2 隅**だけ画像を切る |

見た目は 1 つの枠に角丸が掛かった状態と同じで、枠は何も切らない。

### hover / active は C3 と同じ覆いで表す（T023）

カード全体の 4 状態（[contracts/components.md](../../../specs/005-ui-refinement/contracts/components.md) 2.
の C8）には、Phase 4 で NavItem に入れた `::after` の覆い（`isolate` + `-z-10`）をそのまま
使った。新しい面のトークンを足さずに「1 段明るく / 暗く」が表せる手が、すでに同じ
リポジトリの中にあるためである（「決定の記録（Phase 4）」の「hover / active は地ではなく
覆いで表す」）。

覆いは中身の**下**に敷かれるので、サムネイルの上の小片（時間バッジ・視聴済み・進捗線・
再生できない印）は覆われない。地が明るくなるのは題名の行で、サムネイル側は 004 から
ある `group-hover:opacity-90` が引き続き手応えを返す。

### バッジは等幅をやめ、桁揃えだけを残す（T024）

原案のバッジは地の書体のままである。`font-mono` を外し、`tabular-nums` に替えた。
桁の幅が揃っていればカードごとに数字の位置が動かないので、等幅フォントである必要は
無い。地（`--color-badge`）・位置（右下）・尺が未取得なら出さないことは変えていない。

これは FR-016（動画の情報を等幅の羅列にしない）とは別の判断である。FR-016 の宛先は
再生画面の情報パネル（C16 / T033）で、そちらは Phase 7 で扱う。
