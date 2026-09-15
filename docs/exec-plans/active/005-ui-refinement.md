# 実行計画: 原案デザインに合わせた UI の再構築

- ステータス: 進行中（Phase 3 まで。Phase 4 以降は未着手）
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
| Phase 3: US1（レイアウトが原案の 3 領域になる） | 完了（2026-09-15）。T007〜T014。構図の確認（quickstart S2）は人が行う |
| Phase 4: US2（サイドバーとヘッダーが原案どおりに並ぶ） | 未着手。T015〜T022 |
| Phase 5: US3（動画カードが原案の形になる） | 未着手。T023〜T025 |
| Phase 6: US4（ツールバーと通知を整える） | 未着手。T026〜T032 |
| Phase 7: US5（再生画面が動画と情報パネルの 2 分割になる） | 未着手。T033〜T037 |
| Phase 8: Polish & Cross-Cutting Concerns | 未着手。T038〜T046 |
| 受け入れ検証（S1） | 未実施 |
| 受け入れ検証（S0・S2〜S9） | 未実施 |

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

**構図の確認（[quickstart.md](../../../specs/005-ui-refinement/quickstart.md) S2）は未実施である。**
幅 1280 / 640 / 639 / 360 での 3 領域・固定・ロゴの移動は人がブラウザで確かめるもので、
この回では行っていない。US2 で中身（ナビゲーションとタブ）が入ってから S2・S3 をまとめて
実行するほうが、同じ画面を 2 度見ずに済む。

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
