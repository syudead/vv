# Implementation Plan: 原案デザインに合わせた UI の再構築

**Branch**: `claude/sdd-005-plan`（機能ディレクトリ: `005-ui-refinement`） | **Date**: 2026-09-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-ui-refinement/spec.md`

## Summary

004 で作った画面は、規則（トークン・4 状態・キーボード・読み上げ）は満たしているが、
**構図が原案と違う**。ヘッダーもサイドバーも無く、ツールバーとグリッドがページに直置き
されている。本機能は、その差分を**機能を 1 つも増やさずに**埋める。サーバーの API も、
保存するデータも、利用者が実際に行える操作も変えない（FR-013・FR-014）。

技術的な進め方は 4 本である。

1. **スクロールの持ち主を変えない**（[R-501](./research.md)）。3 領域は
   `position: fixed` のサイドバー・ヘッダーと、いままでどおり文書（ウィンドウ）が
   スクロールするコンテンツで作る。内側にスクロール容器を作る案を採らないのは、
   004 が積んだ復元・密度アンカー・無限スクロールがすべて `window.scrollY` と
   ビューポート基準の `IntersectionObserver` の上に載っているためで、そこを動かすと
   FR-018（004 の受け入れ基準の維持）が丸ごと危険になる。
2. **幅の分岐を CSS だけで行う**（[R-502](./research.md)）。640px の境界は Tailwind の
   `sm:`（`width >= 40rem`）そのものである。ロゴはサイドバーとヘッダーの両方に置き、
   使わない側を `display: none` で消す — JavaScript で幅を監視しない。
3. **表示のみの要素を「押せない構造」にする**（[R-503](./research.md)）。`<button disabled>`
   でも `aria-disabled` でもなく、**対話要素にしない**。裏側の機能が無いのだから、
   条件が揃えば使えるという意味を持つ印を付けるのは誤りである。読み上げには
   補足語（「（未実装）」）で伝える（FR-006）。
4. **アクセントの差し替えを検査つきで行う**（[R-504](./research.md)）。青（`#63a9f2`）を
   原案のシアン（`#5fd4e4`）に替え、新しい組を `web/src/theme/tokens.test.ts` の
   対比表に足す。表に無い組は検査されないので、**トークンを足したら表にも足す**
   （[contracts/design-tokens.md](./contracts/design-tokens.md)）。

新しく判断が要ったのは、上の 4 点に加えて **密度のスライダー化**
（[R-506](./research.md) — 保存の形は `vv.view.v1` のまま変えない）、**再生画面の 2 分割**
（[R-505](./research.md) — 一覧と同じ 640px の境界で縦に畳む）、**見出しの持ち主**
（[R-508](./research.md) — `h1` はロゴへ移し、一覧から見出し文字を消す）である。

## Technical Context

**Language/Version**: TypeScript（`web/package.json`、Node 22 LTS）。Go 側の変更は無い

**Primary Dependencies**: React 19 + Vite 8 + Tailwind CSS 4 + `react-router` v7（いずれも
既存）。**実行時・開発時とも依存を 1 つも増やさない**。アイコンは外部のアイコン集を
足さず、必要な 10 個程度をインライン `<svg>` で持つ（[R-510](./research.md)）

**Storage**: 新しい永続化は**無い**。`localStorage` の鍵は `vv.view.v1` のままで、
値の形も変えない（[data-model.md](./data-model.md) 3.）。SQLite の表も
`api/openapi.yaml` も変わらない（FR-014）

**Testing**: 既存の `vitest run`（`jsdom`）。足すのは (a) 対比表への新しい組と
「表示のみ < 機能する」の明暗の順序（`theme/tokens.test.ts`）、(b) 表示のみの要素が
対話要素になっていないこと（`layout/placeholders.test.tsx`）、(c) 密度スライダーの
値の読み替え、(d) 情報パネルの体裁（等幅でない・ラベルと値が対になる）。
**画面幅による出し分けは jsdom では確かめられない**（CSS メディアクエリが効かない）ので、
[quickstart.md](./quickstart.md) S2 で人が確かめる

**Target Platform**: 現行世代のブラウザ（デスクトップとモバイル）。画面幅 360px〜2560px

**Project Type**: web-service の Web 層のみ（単一 Go バイナリに埋め込まれる React SPA）

**Performance Goals**: 004 の値を維持する — 最初の画面 2 秒以内（1万本）／1000 件読み込み
後も反応の開始 100ms 以内／サムネイル到着によるレイアウトのずれ 0。本機能は
`content-visibility` と格子の式に手を入れない

**Constraints**: 依存を増やさない／`api/openapi.yaml` を変更しない（FR-014）／004 の
受け入れ検証が引き続き成功する（FR-018・SC-009）／対比は本文 4.5:1・大きな文字 3:1
（FR-011。表示のみの要素は対象外）／利用者が実際に行える操作の種類と数を変えない
（FR-013・SC-004）

**Scale/Scope**: 画面 2 つ、新規の部品 16 個（C1〜C16。うち 5 個は既存部品の作り直し）。
変更は `web/src/` に閉じ、TypeScript で 1100 行程度（うち新規のテスト 250 行程度）を
目安とする

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` は雛形のまま批准されていない。したがってゲートは
001・002・004 と同じく、実質的な規範として機能している
[`ARCHITECTURE.md`](../../ARCHITECTURE.md)、
[core-beliefs.md](../../docs/design-docs/core-beliefs.md)、
[技術選定文書 2. 選定の判断基準](../../docs/design-docs/tech-stack-selection.md)、
[`AGENTS.md`](../../AGENTS.md) から導出する。004 と同じ 8 つを引き継ぐ。

| ゲート | 根拠 | 初期判定 | Phase 1 設計後の再判定 |
| --- | --- | --- | --- |
| G1: 依存方向が一方向で、機械的に強制される | ARCHITECTURE.md | PASS — Go 側に触れない | PASS — 変更は `web/src/` に閉じる。`api/` と `internal/` は変更対象外（FR-014） |
| G2: 常駐する外部ミドルウェアを増やさない | 判断基準1 | PASS | PASS — 依存は実行時も開発時も 0 個増。アイコンは外部の集を入れずインライン `<svg>` で持つ（[R-510](./research.md)） |
| G3: DB は再構築可能なインデックスに留める | 判断基準2 | PASS — DB に触れない | PASS — 新しい永続化は無い。`vv.view.v1` の形も変えない（[data-model.md](./data-model.md) 3.） |
| G4: 重要な制約は可能な限りテスト可能にする | core-beliefs | 要設計 — 「表示のみ」「構図」「対比」をどう機械で守るか | PASS — 表示のみは DOM の性質（対話要素でない・tab 順に無い）として検査でき（[R-503](./research.md)）、対比と明暗の順序は CSS を読む検査で守る（[contracts/design-tokens.md](./contracts/design-tokens.md) 4.）。構図と画面幅は人が確かめると明示した（[quickstart.md](./quickstart.md) S2・S3） |
| G5: 文書は変更と同じ変更単位で更新する | AGENTS.md / core-beliefs | 要対応 | PASS — 成果物に `ARCHITECTURE.md`（Web 層に骨格の記述を足す）と `docs/design-docs/library-ui-design-system.md` の追補を含める。実行計画は `docs/exec-plans/active/005-ui-refinement.md` に置く（下記） |
| G6: 生成物は手編集せず、元ファイルから生成する | AGENTS.md | PASS | PASS — `web/src/api/gen/` に触れない。`generate-check` が契約の無変更を機械的に示す（FR-014） |
| G7: 後から重くできる境界を最初に引く | 判断基準4 | 要設計 — 裏側の無い入口を 15 個置くことが、後で機能を足すときの妨げにならないか | PASS — 表示のみの項目は 1 つの表（[data-model.md](./data-model.md) 2.）から描く。機能が付くときは、その行を「機能する」に替えて描画側の分岐を 1 か所通すだけで済む（[contracts/components.md](./contracts/components.md) 3.） |
| G8: 画面の変更は画像で示す | AGENTS.md | 要対応 — 本機能は画面の変更そのもの | PASS — [quickstart.md](./quickstart.md) S9 で 6 枚を撮り、PR に添えることを手順に組み込んだ |

違反なし。判断の分かれる点は「Complexity Tracking」に記載する。

## Project Structure

### Documentation (this feature)

```text
specs/005-ui-refinement/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — 未確定事項の解消（R-501〜R-511）
├── data-model.md        # Phase 1 output — 画面が持つ状態と静的な表
├── quickstart.md        # Phase 1 output — 受け入れの検証手順（S0〜S9）
└── contracts/           # Phase 1 output
│   ├── layout.md             # 3 領域の寸法・境界・スクロールの持ち主（FR-001〜FR-003・FR-015）
│   ├── design-tokens.md      # 004 の契約への差分（FR-010・FR-011）
│   └── components.md         # C1〜C16 の一覧と、表示のみの要素の規約（FR-004〜FR-008）
```

### Source Code (repository root)

追加・変更する場所だけを示す（`+` が新規、`~` が変更）。

```text
web/src/
~ index.css                   # accent をシアンへ。inert・accent-surface・骨格の寸法を足す
~ App.tsx                     # 一覧の経路だけを AppShell で包む（再生画面は包まない。FR-015）
├── layout/                   # + 骨格。画面ではなく「画面の器」を置く新しい区画
│ + AppShell.tsx              # 3 領域（C1 + C5 + コンテンツ）。FR-001・FR-002
│ + Sidebar.tsx               # C1。幅 240px・独立してスクロール
│ + Logo.tsx                  # C2。サイドバーとヘッダーの両方に置き、CSS で出し分ける（R-502）
│ + NavItem.tsx               # C3。機能する / 表示のみ の 2 形（R-503）
│ + SidebarSection.tsx        # C4
│ + Header.tsx                # C5。高さ 56px
│ + Tab.tsx                   # C6
│ + IconButton.tsx            # C7
│ + icons.tsx                 # インラインの <svg>（外部の集を足さない。R-510）
│ + navigation.ts             # 表示のみ / 機能する の表（data-model.md 2.）
│ + placeholders.test.tsx     # 表示のみの要素が対話要素でないことの検査（R-503）
├── components/
│ ~ VideoCard.tsx             # C8 + C9。枠に角丸をまとめ、バッジの体裁を原案に寄せる
│ + SearchInput.tsx           # C10。虫眼鏡つき
│ + Select.tsx                # C11。既定の <select> の見た目を外す
│ + DensitySlider.tsx         # C12。DensitySelect を置き換える（R-506）
│ - DensitySelect.tsx         # C12 に置き換わるので削除
│ ~ StateNotice.tsx           # C13。情報 / 警告 / エラーの 3 段階に整理する
│ + InfoPanel.tsx             # C14 + C15。再生画面の右の縦長のパネル
│ + MetaList.tsx              # C16。等幅の羅列をやめる（FR-016）
│ ~ Toolbar.tsx               # コンテンツ側の帯として残す。件数と表示切替を受ける口を足す
│ ~ ScanStatus.tsx            # 置き場所だけ（帯の中のまま）。振る舞いは変えない
├── pages/
│ ~ LibraryPage.tsx           # 見出し文字を落とし、件数を帯へ移す（R-508）。密度アンカーの逃げ幅にヘッダー高を足す。復元と無限スクロールは触らない
│ ~ VideoPage.tsx             # 2 分割へ。パネルに題名・情報・再開の知らせを収める（FR-015）
└── theme/
  ~ tokens.test.ts            # 対比表に新しい組と明暗の順序を足す

~ ARCHITECTURE.md                                  # Web 層に骨格（layout/）の記述を足す
~ docs/design-docs/library-ui-design-system.md     # 原案に寄せた分の追補
+ docs/exec-plans/active/005-ui-refinement.md      # 実行計画（AGENTS.md）。implement の最初の回で作る
~ docs/screenshots/                                # 新しい 6 枚（quickstart S9）
```

**Structure Decision**: 変更は `web/src/` に閉じる。Go 側・`api/openapi.yaml`・生成物には
一切触れない。これは 004 と同じく、**FR-013・FR-014 に対する構造上の防波堤**である。

新しく `web/src/layout/` を足す。`components/` と分けるのは、骨格（どの画面にも同じ形で
存在し、画面の中身を知らない）と部品（画面が並べるもの）で変更の理由が違うからである。
骨格には「表示のみの要素」が集中しており、その規約（[R-503](./research.md)）を 1 つの
区画に閉じ込められる利点もある。

AGENTS.md が求める実行計画は `docs/exec-plans/active/005-ui-refinement.md` に置き、
完了時に `completed/` へ移す。**作るのは implement の最初の回**である — 004 の実行計画も
Phase 1 の回（`e764a2a`）で作られており、plan 段階の成果物は Spec Kit 側
（`plan.md`・`research.md`・`data-model.md`・`contracts/`・`quickstart.md`）に限られる。
本 PR にその文書が無いのはそのためで、漏れではない。

`pages/LibraryPage.tsx` の**復元と無限スクロールには触らない**。
[R-501](./research.md) がスクロールの持ち主を変えないと決めたのは、この 2 つを差分から
外すためである。触っていないことが差分から読み取れることが、FR-018・SC-009 に対する
いちばん強い根拠になる。

**密度アンカーだけは例外で、逃げ幅にヘッダーの高さを足す必要がある**
（[R-501](./research.md) の「1 つの例外」）。いまの位置合わせはツールバーの実測高だけを
引くが、005 ではツールバーがヘッダーの下に粘るので、固定領域の下端は
`--size-header` + ツールバーの実測高になる。足さないと、戻した項目がヘッダーの裏に
56px ぶん隠れる。`topmostId` の可視判定も同じ下端を基準にする（規則は
[contracts/layout.md](./contracts/layout.md) 1.）。

## Phase 2 以降へ送る判断

| 論点 | 送り先 | 理由 |
| --- | --- | --- |
| 表示のみの要素（コレクション・タグ・お気に入り・未整理・画像タブ）に機能を持たせる | 整理機能として別に | spec のスコープ外。裏側（分類・タグ・お気に入りの保存先）が無い |
| 画質バッジ（4K / HD）・複数選択・カードの「その他の操作」 | 別機能 | spec のスコープ外。今回は原案の構図の再現に絞る |
| 表示切替（グリッド / リスト）の実装 | 別機能 | 表示のみとして置く。リスト表示は密度とは別の描き方で、格子の式ごと増える |
| 明るいテーマ | 別機能 | spec の Assumptions。役割名のトークンなので値を 1 組足すだけで済む |
| 仮想スクロール | 規模が要求したとき | 004 のまま。本機能は格子の式に触れない |
| 再生の E2E（Playwright） | Phase 3 | 技術選定文書のまま。画面幅と構図は quickstart で人が確かめる |

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 裏側の機能が無い入口を 15 個、画面に置く | 原案の構図を再現するのが本機能の目的で（spec 冒頭）、機能の有無で要素を省くと構図そのものが変わる。SC-001 は「原案に写っている要素が同じ領域に同じ順序で存在すること」を判定基準にしている | 省く案は SC-001 を満たせない。押せるようにする案は FR-013（実際に行える操作の種類と数を変えない）に反し、裏側の無い操作を足すことになる |
| 骨格を `position: fixed` で作り、`display: grid` の器にしない（[R-501](./research.md)） | 004 の復元・密度アンカー・無限スクロールが `window.scrollY` とビューポート基準の `IntersectionObserver` に載っている。スクロール容器を内側へ移すと 3 つとも書き換えになり、FR-018・SC-009 の担保が本機能の主目的から外れて膨らむ | grid + 内部スクロールにする案は構図としては素直だが、差分が「見た目の変更」から「一覧の中核の作り直し」へ広がる。持ち主を変えるのは、仮想スクロールのように内側の容器を必要とする変更が来たときでよい |
| ロゴが DOM に 2 か所ある（[R-502](./research.md)） | FR-003 は「ロゴはどの幅でも画面上に存在する」ことを求める一方、640px 未満では置き場所がサイドバーからヘッダーへ移る。CSS だけで満たすには両方に置いて片方を消すほかない | 1 か所に置いて JavaScript で移す案は、幅の監視・初回描画のちらつき・テストのための擬似 `matchMedia` を抱え込む。`display: none` は支援技術からも消えるので、二重に読まれる心配は無い |
| 密度の保存値（`dense` / `standard` / `relaxed`）と、スライダーの位置（0 / 1 / 2）が別物になる（[R-506](./research.md)） | C12 はスライダーだと spec が決めている。一方 `vv.view.v1` の値の形を変えると、いま使っている利用者の設定が失われる（FR-013 の「従来どおり」に反する） | 保存の形をスライダーに合わせて数値にする案は、移行の処理を 1 つ増やし、壊れた値の扱い（004 の FR-019）を書き直すことになる。読み替えは 1 つの配列で足り、往復の検査も 1 つで済む |
