# Tasks: 原案デザインに合わせた UI の再構築

**Input**: Design documents from `/specs/005-ui-refinement/`

**Prerequisites**: [plan.md](./plan.md)（必須）、[spec.md](./spec.md)（ユーザーストーリー）、[research.md](./research.md)、[data-model.md](./data-model.md)、[contracts/](./contracts/)、[quickstart.md](./quickstart.md)

**Tests**: 本機能はテストを**含む**。ただし増やすのは 4 つだけである
（[R-511](./research.md) / [quickstart.md](./quickstart.md) S1）。

| 足す検査 | 置き場所 | 対応 |
| --- | --- | --- |
| 対比表の新しい組と `inert` < `muted` の明暗の順序 | `web/src/theme/tokens.test.ts` | FR-011 / SC-006 |
| 表示のみの要素が対話要素でなく、tab 順に無く、件数を持たない | `web/src/layout/placeholders.test.tsx` | FR-005 / FR-013 |
| 密度の読み替えが 3 段階すべてで往復して恒等 | `web/src/components/DensitySlider.test.tsx` | FR-013 |
| 情報パネルが 6 項目を順に持ち、値が等幅でない | `web/src/components/MetaList.test.tsx` | FR-016 |

**構図そのもの（SC-001）と画面幅による出し分け（SC-003・SC-007）に機械の検査を用意しない。**
jsdom は CSS を適用しないので、`position: fixed`・メディアクエリ・`grid-template-columns` の
どれも確かめられない。擬似的に確かめる仕掛けを作ると「テストは通るが画面は壊れている」
状態を招く（[R-511](./research.md)）。これらは [quickstart.md](./quickstart.md) S2〜S7 で人が
原案と並べて確かめる。

**Organization**: タスクはユーザーストーリー単位にまとめてある。トークン
（[contracts/design-tokens.md](./contracts/design-tokens.md)）とナビゲーションの表
（[data-model.md 2.](./data-model.md)）とアイコンは複数のストーリーが使うので Foundational に置く。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 並行実行可（別ファイル・依存なし）
- **[Story]**: 対応するユーザーストーリー（US1 / US2 / US3 / US4 / US5）
- 説明には必ず対象ファイルのパスを書く

## Path Conventions

本機能の**コードの変更は `web/src/` に閉じる**。`api/openapi.yaml`・`internal/`・`cmd/`・
`web/src/api/gen/` には触れない。これは方針であると同時に FR-013・FR-014 に対する構造上の
防波堤である（[plan.md](./plan.md) の Structure Decision）。

コード以外では、Phase 8 が `ARCHITECTURE.md`・`docs/design-docs/`・`docs/exec-plans/`・
`docs/screenshots/` と [004 の contracts/design-tokens.md](../004-library-ui/contracts/design-tokens.md) を
更新する（[plan.md](./plan.md) の Source Code の一覧に含まれている）。**この機能の spec.md と
plan.md は書き換えない。**

- 骨格（新設）: `web/src/layout/` — どの画面にも同じ形で存在し、画面の中身を知らない
- 部品: `web/src/components/` — 画面が並べるもの
- 画面: `web/src/pages/`（`LibraryPage.tsx`・`VideoPage.tsx`）
- 見た目の規則の唯一の真実: `web/src/index.css` の `@theme`（004 の R-401 のまま）
- **依存を 1 つも増やさない**（実行時・開発時とも）。アイコンはインライン `<svg>`
  （[R-510](./research.md)）
- `web/src/api/`・`web/src/preferences/viewPreferences.ts` の**値の形は変えない**
  （[data-model.md 5.](./data-model.md)）

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 実行計画を置き、書き換える前の緑を記録する

- [X] T001 実行計画 `docs/exec-plans/active/005-ui-refinement.md` を作成する。書式は [docs/exec-plans/completed/004-library-ui.md](../../docs/exec-plans/completed/004-library-ui.md) に合わせ、ステータス（進行中）・最終更新日・対象・目的・一次資料の表（spec／plan／research／data-model／contracts／quickstart へのリンク）・検証の方針（[quickstart.md](./quickstart.md) S1〜S9 で完了を判定し、S2〜S7 は人が確かめると明記する）・フェーズごとの進捗の空欄・決定の記録を置く。[plan.md](./plan.md) が「作るのは implement の最初の回」と定めているのがこのタスクである
- [X] T002 書き換える前に `make test-web` と `make check` を実行し、結果を `docs/exec-plans/active/005-ui-refinement.md` の進捗に記録する。**004 の検査が緑である状態から始める**ことを記録しておかないと、以降で落ちたときに本機能が壊したのか元から赤かったのかが分からない（FR-018 / SC-009）

**Checkpoint**: 実行計画があり、着手前の `make check` が緑であることが記録されている

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: すべてのストーリーが使うトークン・表・アイコンを置く

**⚠️ CRITICAL**: T003 のトークンが以降すべての画面タスクの前提である。トークンより先に
画面を触ると、生の色を書いて 004 の `noRawColors.test.ts` に落ちる

- [X] T003 `web/src/index.css` の `@theme` を [contracts/design-tokens.md](./contracts/design-tokens.md) 1.・2. のとおりに直す。`--color-accent` を `#63a9f2` から **`#5fd4e4`** に変え（FR-010）、`--color-accent-surface: #12262b`・`--color-inert: #6b7482`・`--size-sidebar: 15rem`・`--size-header: 3.5rem` を足す。`--color-accent-ink`（`#07131f`）は**変えない**。ファイル冒頭のコメントが指している対比表の参照先に 005 の契約を足す（トークンを足したら表にも足す、という規則の宛先が 2 つになるため）
- [X] T004 `web/src/theme/tokens.test.ts` の `pairs` に [contracts/design-tokens.md](./contracts/design-tokens.md) 3. の 3 行（`accent` / `accent-surface`、`body` / `accent-surface`、`muted` / `accent-surface`、いずれも必要 4.5）を足す。あわせて **`inert` の相対輝度が `muted` より小さい**ことを確かめる検査を 1 件足す（同 4.）。`--color-inert` は基準を下回るので **`pairs` には入れない** — FR-011 が明示的に対象外としており、淡く見えることが要求だからである
- [X] T005 [P] `web/src/layout/icons.tsx` を新規作成し、必要な 10 個程度（映画・時計・ハート・フォルダ・タグ・虫眼鏡・漏斗・歯車・格子・一覧・×）をインラインの `<svg>` で置く（[R-510](./research.md)）。**外部のアイコン集を依存に足さない**。塗りと線は `currentColor` にしてトークンに従わせ、`aria-hidden="true"` と `focusable="false"` をこのファイルの中で一律に付ける（呼び出し側に書かせると、足したアイコンが読み上げに漏れる）
- [X] T006 [P] `web/src/layout/navigation.ts` を新規作成し、[data-model.md 2.](./data-model.md) の 14 行をそのまま定数の表にする。項目は `id`・`label`・`icon`・`kind`（`"live"` / `"inert"`）・`to`・`section`（`"library"` / `"collection"` / `"tag"` / `"tab"`）で、**`kind: "inert"` の行が `to` を持てない**ことを型で落ちるようにする（判別可能な合併にする）。`live` は `all-videos` と `tab-videos` の 2 つだけである。**この表が「機能する / 表示のみ」の唯一の真実**であり、描画側は `kind` で分岐するだけにする

**Checkpoint**: `make test-web` が緑。トークンが新しい値で対比を満たし、`inert` が `muted` より暗いことが機械で守られている

---

## Phase 3: User Story 1 - レイアウトが原案の 3 領域になる (Priority: P1) 🎯 MVP

**Goal**: 一覧が「左にサイドバー・上にヘッダー・残りがコンテンツ」の 3 領域になり、
グリッドをスクロールしても固定領域が動かない。幅 640px 未満ではサイドバーが消え、
ロゴがヘッダーへ移る。

**Independent Test**: 幅 1280 で一覧を開き、3 領域が指定のサイズで存在し、グリッドを
スクロールしてもサイドバーとヘッダーが動かない。幅 640 で出て 639 で消える
（[quickstart.md](./quickstart.md) S2）。

- [X] T007 [P] [US1] `web/src/layout/Logo.tsx` を新規作成する（C2）。製品アイコン（T005 の映画）と製品名を**2 段**で並べ、製品名は本リポジトリの名前（`vv`）にする。**`h1` はここが持つ**（[R-508](./research.md)）。**リンクにしない・対話要素にしない** — `Tab` の停止位置が 1 つ増えると FR-013 に反する（[contracts/components.md](./contracts/components.md) 1.）
- [X] T008 [US1] `web/src/layout/Sidebar.tsx` を新規作成する（C1）。`position: fixed` で左に置き、幅 `--size-sidebar`・高さ `100dvh`、中身が溢れたら**独立して**流れる（`overflow-y: auto`）。地は `--color-surface-raised`。**幅 640px 未満では `display: none`**（Tailwind の `hidden sm:block`。`visibility` や `opacity` を使わない。[contracts/layout.md](./contracts/layout.md) 3.）。最上部に T007 の Logo を置く
- [X] T009 [US1] `web/src/layout/Header.tsx` を新規作成する（C5）。`position: fixed` で上に置き、高さ `--size-header`、左端は幅 640px 以上で `--size-sidebar`・未満で画面の左端（`left-0 sm:left-[var(--size-sidebar)]`）。スクロールしない。**ロゴをもう 1 つ置き、640px 以上では `display: none` にする**（`sm:hidden`。DOM に 2 か所ある理由と、支援技術から消えるので製品名が 2 回読まれない理由は [R-502](./research.md)）
- [X] T010 [US1] `web/src/layout/AppShell.tsx` を新規作成する（FR-001 / FR-002）。Sidebar と Header を置き、子（コンテンツ）を `padding-top: var(--size-header)` と幅 640px 以上での `padding-left: var(--size-sidebar)` で避けさせる。**内側にスクロール容器を作らない** — スクロールの持ち主は文書（ウィンドウ）のままである（[R-501](./research.md)）。重なりの順序はサイドバー・ヘッダーがコンテンツより前、ツールバーはヘッダーより後ろ（[contracts/layout.md](./contracts/layout.md) 1.）
- [X] T011 [US1] `web/src/App.tsx` で **`/` の経路だけ**を `AppShell` で包む。`/videos/:id` は包まない（FR-015。再生画面にサイドバーもヘッダーも被せない。[R-505](./research.md)）。骨格が「いまどの画面か」を知らずに済むよう、分岐はこの 1 か所に閉じる
- [X] T012 [US1] `web/src/components/Toolbar.tsx` に件数を受ける口（`count`）を足し、帯の**右端**に置く。`sticky top-0` を **`top: var(--size-header)`** に変え、ヘッダーの裏へ潜らせない（[contracts/layout.md](./contracts/layout.md) 1.）。`z-index` はヘッダーより後ろ、一覧の項目より前
- [X] T013 [US1] `web/src/pages/LibraryPage.tsx` から見出し `<h1>vv</h1>` を消し（`h1` は T007 の Logo が持つ）、件数の段落を T012 の口へ移す（[R-508](./research.md)）。**文言（「N 本」「「語」に一致 N 本」「読み込み中…」）も `role="status"` / `aria-live="polite"` も変えない**（004 の FR-008 / FR-021 を FR-018 が引き継ぐ）。見出しを参照している既存のテスト（`web/src/pages/LibraryPage*.test.tsx`）があれば、参照先を件数か Logo へ付け替える
- [X] T014 [US1] `web/src/pages/LibraryPage.tsx` の密度アンカーの逃げ幅に**ヘッダーの高さを足す**（[R-501](./research.md) の「1 つの例外」）。いまツールバーの実測高だけを引いている位置合わせ（`const offset = bar.current?.getBoundingClientRect().height ?? 0`）を `--size-header` + ツールバーの実測高にし、`topmostId` の可視判定（いまは `bottom > 0`）も同じ固定領域の下端を基準にする。足さないと、戻した項目がヘッダーの裏に 56px ぶん隠れる。**復元（`window.scrollY` の保存と復元）と無限スクロール（`IntersectionObserver`）には触らない** — 触っていないことが差分から読み取れることが FR-018 のいちばん強い根拠である

**Checkpoint**: 一覧が 3 領域になり、`make test-web` が緑。[quickstart.md](./quickstart.md) S2 を人が実行できる

---

## Phase 4: User Story 2 - サイドバーとヘッダーが原案どおりに並ぶ (Priority: P2)

**Goal**: サイドバーにロゴ・ライブラリ・コレクション・タグが原案と同じ順序で並び、
ヘッダーにタブと設定ボタンがある。機能するのは「すべての動画」と「動画」タブの 2 つだけで、
残りは淡く表示されて押せない。

**Independent Test**: 原案と並べて順序と体裁が一致し、[contracts/components.md](./contracts/components.md) 3. の
とおりに機能する / しないが分かれている（[quickstart.md](./quickstart.md) S3）。

- [X] T015 [US2] `web/src/layout/NavItem.tsx` を新規作成する（C3）。`kind` で 2 形に分かれる。`live` は `<Link>` で、通常 / hover（地を 1 段明るく）/ focus（`:focus-visible` で `--color-focus` の輪郭を `outline-offset` つきで外側に）/ active（地を 1 段暗く）の 4 状態を持ち、選択中は `--color-accent-surface` の地と `--color-accent` の文字で示し、右端に件数を出す。`inert` は **`<span>` で描き、`disabled` も `aria-disabled` も `role="button"` も `tabIndex` も付けない**（[R-503](./research.md)）。色は `--color-inert`、**件数は受け取らない**、ラベルの直後に `<span class="sr-only">（未実装）</span>` を添える（FR-006）。hover でも変化しない。当たり判定は `--size-tap` 四方以上
- [X] T016 [P] [US2] `web/src/layout/SidebarSection.tsx` を新規作成する（C4）。見出し（「コレクション」「タグ」など）と項目群を受ける器で、見出しは `<h2>`、項目群は `<ul>` / `<li>` にする。ライブラリの区画は原案に見出し文字が無いので、見出しを省ける形にする
- [X] T017 [P] [US2] `web/src/layout/Tab.tsx` を新規作成する（C6）。`live`（「動画」）は選択中として**下線**と `--color-accent` で示し、4 状態を持つ。`inert` は `<span>` で `--color-inert`、`（未実装）` を添える（T015 と同じ規約）。規約は [contracts/components.md](./contracts/components.md) 3. を共有する
- [X] T018 [P] [US2] `web/src/layout/IconButton.tsx` を新規作成する（C7）。**設定・フィルタ・表示切替の 3 つはすべて表示のみ**なので、この部品は対話要素を作らない — `<span>` にアイコン（T005）と `<span class="sr-only">{label}（未実装）</span>` を置き、`--color-inert` で描く。**`label` は必須の引数にする** — 3 か所で使い回す部品なので、文言を部品の中に固定すると漏斗も表示切替も「設定」と読まれ、要素の区別が読み上げ利用者にだけ失われる（FR-006）。名前が Button であっても `<button>` にしない理由をファイル冒頭のコメントに書く（[R-503](./research.md)）
- [X] T019 [US2] `web/src/layout/Sidebar.tsx` に中身を入れる。`navigation.ts`（T006）の `section` が `library` / `collection` / `tag` の行を、表の順序のまま SidebarSection（T016）+ NavItem（T015）で描く。見出しは「ライブラリ」（原案どおり省いてよい）・「コレクション」・「タグ」。「すべての動画」にだけ件数を渡す
- [X] T020 [US2] `web/src/layout/Header.tsx` に中身を入れる。`navigation.ts` の `section` が `tab` の 4 行を Tab（T017）で左から並べ、右端に設定の IconButton（T018。`label` は「設定」）を置く
- [X] T021 [US2] 一覧の総件数を NavItem「すべての動画」へ届ける。**`total` は `LibraryPage`（AppShell の子）が `useVideos` から得る値**なので、親である Sidebar へは引数で渡せない。`web/src/layout/AppShell.tsx` の中に件数の context（`useState<number | undefined>` と設定用の関数）を置き、同じファイルから提供側と読み出し側の hook を export する。`LibraryPage` は件数を公開するだけにし、**`web/src/api/useVideos.ts` の置き場所も戻り値も変えない**（[data-model.md 5.](./data-model.md) の「変わらないもの」）。新しいファイルを作らないのは、この状態が骨格の外で使われないためである。**公開する値は `total` ではなく `loading ? undefined : total`** にする — `useVideos` は `total` を 0 で初期化し、検索語や並び順を変えて取り直すあいだも前の値を消さないので、`total` だけを見ると初回に「0 本」、絞り込みの最中に前の件数が出る。どちらも嘘であり、帯の件数が同じ場面で「読み込み中…」と出しているのと食い違う（004 の FR-008）。**件数が `undefined` のあいだは NavItem に件数を出さない**。`inert` の行には渡さない（渡す口を作らない。FR-005）
- [X] T022 [US2] `web/src/layout/placeholders.test.tsx` を新規作成する（[contracts/components.md](./contracts/components.md) 5.）。サイドバーとヘッダーを描いて次の 4 つを確かめる。(a) `getAllByRole("button")` / `("link")` に表示のみのラベルが 1 つも現れない、(b) 表示のみの要素が tab 順（`tabIndex >= 0`）に現れない、(c) `navigation.ts` の `kind: "live"` が `all-videos` と `tab-videos` の**ちょうど 2 つ**である（これ以外が `live` になったら利用者が行える操作が増えたということで FR-013 の違反）、(d) 表示のみの要素の近傍に件数が出ていない、(e) 件数が `undefined`（未取得・取り直しの最中。T021）のときは `live` な「すべての動画」にも件数が出ない

**Checkpoint**: サイドバーとヘッダーが原案の順序で並び、`make test-web` が緑。[quickstart.md](./quickstart.md) S3・S7 を人が実行できる

---

## Phase 5: User Story 3 - 動画カードが原案の形になる (Priority: P3)

**Goal**: サムネイルとタイトル行が 1 つの枠にまとまり、枠全体に角丸が付く。時間バッジは
サムネイル右下に乗り、尺が未取得なら出ない。

**Independent Test**: 一覧を表示し、カードの 3 要素が原案どおりに配置される
（[quickstart.md](./quickstart.md) S4）。

- [X] T023 [US3] `web/src/components/VideoCard.tsx` の枠を作り直す（C8）。いまサムネイルとタイトル行が別々に置かれ角丸がサムネイルにだけ付いているのを、**1 つの枠**にまとめて枠全体に `--radius-card` を掛ける（spec 4. / [contracts/components.md](./contracts/components.md) 4.）。サムネイルの 16:9 固定・サムネイルの有無で大きさが変わらないこと（004 の SC-003）・題名の 2 行省略と全文への到達手段 3 つ（004 の R-409）・進捗線・視聴済みの印・再生できない印は**変えない**（spec のスコープ外）。4 状態（hover / focus / active）はカード全体に掛ける
- [X] T024 [US3] 時間バッジ（C9）の体裁を原案に寄せる。位置はサムネイルの**右下**のまま、地は `--color-badge`（**不透明のまま。半透明にしない**。FR-009）。**尺が未取得のときは出さない**（spec US3-3。現行の挙動を変えない）
- [X] T025 [US3] `web/src/components/VideoCard.test.tsx` に、尺が未取得の動画で時間バッジが出ないことの確認が無ければ足す。既存の確認（題名・視聴状況・再生できない印）は**そのまま通ること**を確かめる（FR-018）

**Checkpoint**: 一覧のカードが原案の形になり、`make test-web` が緑。[quickstart.md](./quickstart.md) S4 を人が実行できる

---

## Phase 6: User Story 4 - ツールバーと通知を整える (Priority: P4)

**Goal**: 検索欄・ソート・密度がブラウザ既定の見た目でなくなり、通知が 3 段階で統一される。
**操作の結果は従来と 1 つも変わらない。**

**Independent Test**: 3 つの部品を操作して動作が変更前と同じで、通知が 3 段階のいずれかの
体裁になっている（[quickstart.md](./quickstart.md) S5）。

- [X] T026 [P] [US4] `web/src/components/SearchInput.tsx` を新規作成する（C10）。左端に虫眼鏡アイコン（T005。`aria-hidden`）を置き、入力そのものは `<input type="search">` のままにする。**待ち合わせ 250ms と `maxLength` を変えない**（[data-model.md 5.](./data-model.md)）。4 状態と `--size-tap` の当たり判定を持つ。`web/src/pages/LibraryPage.tsx` の素の入力欄をこれに差し替える
- [X] T027 [P] [US4] `web/src/components/Select.tsx` を新規作成する（C11）。ブラウザ既定の見た目を外し（`appearance: none` + 自前の下向き記号）、`<select>` そのものは保つ。**選択肢は従来どおり 2 つ**（`addedDesc` / `titleAsc`）で、増やさない（FR-013）。`web/src/pages/LibraryPage.tsx` の並べ替えの選択欄をこれに差し替える
- [X] T028 [US4] `web/src/components/DensitySlider.tsx` を新規作成する（C12。[R-506](./research.md)）。`<input type="range" min="0" max="2" step="1">` とし、位置と `Density` の読み替えを **1 つの配列（`["dense","standard","relaxed"]`）**だけで行う（[data-model.md 3.](./data-model.md)）。範囲外・非数値は `standard` に落とす。`aria-valuetext` に段階名（「細かい」「標準」「ゆったり」）を入れる（range は既定で数値を読むため）。**`localStorage` の鍵 `vv.view.v1` と値の形は変えない** — 保存されるのは従来どおり `Density` の文字列である
- [X] T029 [US4] `web/src/components/DensitySlider.test.tsx` を新規作成し、往復が恒等（`toDensity(toIndex(d)) === d`）であることを 3 段階すべてで確かめる。範囲外の位置が `standard` に落ちることも 1 件確かめる
- [X] T030 [US4] `web/src/components/DensitySelect.tsx` を**削除**し、`web/src/pages/LibraryPage.tsx` の差し込み先を DensitySlider（T028）に替える。密度を変えたあとの位置合わせ（T014）は経路を変えない — 変えるのは入口の部品だけである
- [X] T031 [US4] `web/src/components/StateNotice.tsx` を 3 段階に整理する（C13。[contracts/components.md](./contracts/components.md) 4.）。`Tone` から `empty` を落として `info` に寄せ、情報（`surface-raised` / `body`）・警告（`warning-surface` / `warning`）・エラー（`danger-surface` / `danger`）の 3 つにする。**色だけでなく形からも判別できる**よう、左端の色帯かアイコン（T005）を段階ごとに変える（spec US4-5）。`tone="empty"` を渡している既存の呼び出し（`web/src/pages/LibraryPage.tsx`）を `info` に付け替える。**画面全体を置き換えない**という既存の性質は変えない
- [X] T032 [US4] `web/src/components/Toolbar.tsx` に漏斗（フィルタ）と表示切替（グリッド / リスト）の IconButton（T018）を置く。**`label` にはそれぞれ「フィルタ」「表示切替」を渡す**（設定と同じ文言で読まれないこと。FR-006）。**どちらも表示のみ**で、押しても何も起きず、`Tab` でも止まらない（FR-005 / [contracts/components.md](./contracts/components.md) 3.）。帯の中の折り返し（狭い画面で 2 行以上になる）と、帯が操作要素に与える `--size-tap`・`:focus-visible` の規則は**変えない**

**Checkpoint**: ツールバーと通知が原案の体裁になり、`make test-web` が緑。[quickstart.md](./quickstart.md) S5 を人が実行できる

---

## Phase 7: User Story 5 - 再生画面が動画と情報パネルの 2 分割になる (Priority: P5)

**Goal**: 再生画面が左に映像・右に情報パネルの 2 分割になり、サイドバーもヘッダーも
見出し文字も出ない。一覧へ戻る導線はパネル右上の × だけになる。

**Independent Test**: 動画を 1 本開き、2 分割になり、パネル内にタイトルと情報があり、
× で一覧へ戻れる（[quickstart.md](./quickstart.md) S6）。

- [X] T033 [US5] `web/src/components/MetaList.tsx` を新規作成する（C16。[R-507](./research.md)）。`web/src/pages/VideoPage.tsx` の `VideoFacts` を移し、**値から `font-mono` を外す**（FR-016）。`<dl>` / `<dt>` / `<dd>` の構造は保ち、ラベルは `--color-muted`、値は `--color-body`、1 項目 1 行として行間に区切り線を置く。6 項目（長さ・解像度・形式・映像・音声・大きさ）の**順序**と、取れていない値の言い分け（「確認中」「読み取れませんでした」「なし」）は **004 のまま変えない**（[data-model.md 4.](./data-model.md)）
- [X] T034 [P] [US5] `web/src/components/MetaList.test.tsx` を新規作成し、(a) 6 項目が [data-model.md 4.](./data-model.md) の順序で並ぶ、(b) ラベルと値が `<dt>` / `<dd>` の対で出る、(c) 値に等幅フォントの指定が無い、の 3 つを確かめる
- [X] T035 [US5] `web/src/components/InfoPanel.tsx` を新規作成する（C14 + C15）。幅は `sm:` 以上で `minmax(18rem, 24rem)` の範囲に置き、固定幅にしない（[R-505](./research.md)）。中身は **×（右上）→ 題名 → 知らせ → 動画の情報**の順（[contracts/layout.md](./contracts/layout.md) 4.）。**題名は `h1` のままパネルの中に置く** — spec US5-1 が消すと言っているのは「画面の見出し文字」（ロゴや画面名）であって、US5-3 が求めるパネル内の題名ではない。再生画面には Logo が無いので（[R-505](./research.md)）、ここで `h1` を落とすと画面に見出しが 1 つも無くなる。× （C15）は同じファイルの中に置き、4 状態と `--size-tap` を持ち、押すと遷移元の一覧へ戻る（**スクロール位置の復元は 004 の振る舞いのまま**）。**映像に重ねない**（FR-017）
- [X] T036 [US5] `web/src/pages/VideoPage.tsx` を 2 分割にする（FR-015）。幅 640px 以上で左に映像・右に InfoPanel（T035）、**640px 未満ではパネルを映像の下へ回す**（一覧と同じ `sm:` の境界。[R-505](./research.md)）。いまの `<h1>` の題名と「← 一覧へ戻る」のリンクを**画面の直下から外し**、題名（`h1` のまま。T035）と戻る導線（×）をパネルの中へ移す。映像はパネルを除いた領域いっぱいに広がり、比率を保ち（高さを決め打たない）、操作列はブラウザ標準のままにする。再開の知らせ・再生できない形式・再生の失敗の 3 つの通知を**パネルの中**に置き、**映像の大きさを変えない**（spec US5-6 / US4-6）。再生位置の送信（5 秒ごと・離脱時の `sendBeacon`）と再開の下限 5 秒は**触らない**
- [X] T037 [US5] `web/src/pages/VideoPage.test.tsx` と `web/src/pages/VideoPage.progress.test.tsx` を新しい構図に合わせて直す。「← 一覧へ戻る」を指している参照を × （C15）へ付け替え、`VideoFacts` を指している参照を MetaList（T033）へ付け替える。**確かめている事柄そのものは変えない** — 再生位置の記録・再開・戻り先の復元は 004 の受け入れ基準であり、FR-018 が引き続き満たすことを求めている

**Checkpoint**: 再生画面が 2 分割になり、`make test-web` が緑。[quickstart.md](./quickstart.md) S6 を人が実行できる

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: 全画面に効く仕上げと、文書・記録・受け入れ検証

- [ ] T038 [P] 本機能で足した部品（`web/src/layout/` の NavItem・Tab、`web/src/components/` の SearchInput・Select・DensitySlider・InfoPanel・VideoCard）の装飾的な動きをすべて `motion-reduce:` 変種で無効にする（FR-012）。**無効にするのは動きだけ**で、色の最終状態と `--color-focus` の輪郭は常に適用する（操作結果は判別できること）。判定を JavaScript に持たない（`matchMedia` を読まない）
- [ ] T039 [P] `ARCHITECTURE.md` の Web 層の記述に `web/src/layout/`（骨格）を足す。`components/` と分ける理由（骨格はどの画面にも同じ形で存在し画面の中身を知らない／表示のみの要素の規約が 1 区画に閉じる）と、スクロールの持ち主が文書のままであること（[R-501](./research.md)）を書く。依存方向の記述は変えない
- [ ] T040 [P] `docs/design-docs/library-ui-design-system.md` に本機能の分を追補する。「なぜ表示のみの要素を `disabled` でも `aria-disabled` でもなく非対話要素にするのか」「なぜ幅の分岐を CSS だけで行い JavaScript で幅を監視しないのか」「なぜ構図の検証を機械に任せないのか」の 3 点で、詳細は [R-503](./research.md)・[R-502](./research.md)・[R-511](./research.md) を参照する形にする（写さない）。新規の文書ではないので `docs/design-docs/index.md` への追記は要らない
- [ ] T041 [quickstart.md](./quickstart.md) S2 の列数の実測が [contracts/layout.md](./contracts/layout.md) 1. の表（360: 2/2/2、768: 3/2/2、1280: 6/5/3、2560: 7/5/4）と**一致することを確かめてから**、同じ値で [004 の contracts/design-tokens.md](../004-library-ui/contracts/design-tokens.md) 3. の表を更新する。**食い違ったら表を書き換えて済ませない** — 導出の前提（サイドバー 240px、器の上限 1152px、余白 16px、gap 16px）のどれが実装と違うかを突き止める
- [ ] T042 `make check` を通す（[quickstart.md](./quickstart.md) S1）。`fmt-check` → `lint` → `test` → `generate-check` のすべてが成功すること。**`generate-check` が差分なしで通ることが `api/openapi.yaml` と生成物に触れていないことの証明**である（FR-014）。`web/src/api/gen/` に差分が出ていたら、触ってはならないものを触っている
- [ ] T043 [quickstart.md](./quickstart.md) S0 に従って検証用の動画を用意し、**S2〜S7 を人が実行**して結果を `docs/exec-plans/active/005-ui-refinement.md` の進捗に記録する。原案（[002 の assets/ui-mockup.webp](../002-core-video-library/assets/ui-mockup.webp)）を並べて見る。**SC-001 の判定は S3 で行う**（要素の有無と並び順だけで判定し、色・寸法・余白の画素単位の一致は求めない）。実行環境に Docker や読み上げソフトが無くて実施できない項目は、**実施できなかったことを記録する**（できたことにしない）
- [ ] T044 [quickstart.md](./quickstart.md) S8（004 の受け入れ基準）を実行する。`make check` に続き、[004 の quickstart](../004-library-ui/quickstart.md) の **S3・S5〜S9 をそのまま実行する**（FR-018 / SC-009）。**S4（画面幅）だけはそのまま実行しない** — 期待値のうち 2 つ（情報の置き場・列数）が本機能で意図的に変わるためで、その中身は S2 と S6 の「004 の S4 から引き継ぐ確認」が丸ごと引き取っている。特に密度を変えたあとの項目が**ヘッダーの裏に隠れていない**こと（004 の S6-2 / T014）を見る。結果を実行計画に記録する
- [ ] T045 [quickstart.md](./quickstart.md) S9 に従い、[docs/how-to/ui-change-screenshots.md](../../docs/how-to/ui-change-screenshots.md) の手順で 6 枚（一覧 1280 / 一覧 360 / サイドバーの拡大 / 再生 1280 / 再生 360 / 再生の再開の知らせ）を撮り、**新しい日付**の名前で `docs/screenshots/` に置く。既存の `20260914-p7-*` は置き換えず、**変更前として並べて** PR に添える（[AGENTS.md](../../AGENTS.md) の working agreements / plan の G8）
- [ ] T046 `docs/exec-plans/active/005-ui-refinement.md` を完了にし、`docs/exec-plans/completed/005-ui-refinement.md` へ移す（[AGENTS.md](../../AGENTS.md)）。残った妥協（T043・T044 で実施できなかった検証を含む）は [docs/exec-plans/tech-debt.md](../../docs/exec-plans/tech-debt.md) に TD として記録する

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 依存なし。すぐ始められる
- **Foundational (Phase 2)**: Setup の完了に依存し、**すべてのユーザーストーリーを塞ぐ**
- **US1 (Phase 3)**: Foundational の完了後
- **US2 (Phase 4)**: US1 の完了後（Sidebar と Header の器が無いと中身を置けない）
- **US3 (Phase 5)**: Foundational の完了後。US1・US2 とは独立に進められるが、配色が
  決まってから適用するほうが手戻りが無い（spec の Why this priority）
- **US4 (Phase 6)**: Foundational の完了後。T032（帯のアイコンボタン）だけは US2 の
  T018（IconButton）と US1 の T012（帯の口と粘る位置）を前提にする — 同じ
  `web/src/components/Toolbar.tsx` を触るので、T012 → T032 の順に行う
- **US5 (Phase 7)**: Foundational の完了後。骨格を**使わない**画面なので US1・US2 に
  依存しない（[R-505](./research.md)）
- **Polish (Phase 8)**: 必要なストーリーがすべて終わってから。T041 は T043（S2 の実測）の後

### User Story Dependencies

- **US1 (P1)**: 他のストーリーに依存しない。3 領域の枠だけで独立に確かめられる
- **US2 (P2)**: US1 の器（Sidebar / Header）に中身を置く
- **US3 (P3)**: US1・US2 とは独立。トークン（T003）だけに依存する
- **US4 (P4)**: US1 の帯（T012）に差し込む。T032 のみ US2 の T018 に依存
- **US5 (P5)**: 独立。`App.tsx` の経路分岐（T011）だけを前提にする

### Within Each User Story

- 部品 → 骨格 / 画面への差し込み → その検査 の順
- Foundational のトークン（T003）より前に画面を触らない（生の色を書いて 004 の
  `noRawColors.test.ts` に落ちる）
- `navigation.ts`（T006）より前に NavItem / Tab を描かない（「機能する / 表示のみ」の
  真実が 2 か所に分かれる）

### Parallel Opportunities

- Foundational の T005（アイコン）と T006（表）は T003・T004 と並行できる（別ファイル）
- US1 の T007（Logo）は T008〜T010 と並行できる
- US2 の T016・T017・T018 は T015 のあと並行できる（すべて別ファイル）
- US4 の T026・T027 は並行できる
- US5 は T033（MetaList）→ T035（InfoPanel が MetaList を収める）→ T036（画面が InfoPanel を
  収める）が一本の鎖なので、**この 3 つは並行できない**。並行できるのは T034（MetaList の
  検査）だけで、T033 のあと T035・T036 と同時に進められる
- Polish の T038〜T040 は別ファイルで並行できる

---

## Parallel Example: Foundational

```bash
# T003 のあと、次の 3 つは同時に進められる（すべて別ファイル）:
Task: "対比表の 3 組と inert < muted の順序を web/src/theme/tokens.test.ts に足す"
Task: "インラインの <svg> を web/src/layout/icons.tsx に置く"
Task: "ナビゲーションの表を web/src/layout/navigation.ts に置く"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup（実行計画と着手前の緑）
2. Phase 2: Foundational（トークン・表・アイコン。**ここが全ストーリーを塞ぐ**）
3. Phase 3: US1（3 領域）
4. **止めて確かめる**: [quickstart.md](./quickstart.md) S2 を人が実行する
5. この時点で原案の構図の骨格ができている

### Incremental Delivery

1. Setup + Foundational → 土台（トークン・表・アイコン）
2. US1 を足す → S2 で確かめる → ここが MVP
3. US2 を足す → S3・S7 で確かめる
4. US3 を足す → S4 で確かめる
5. US4 を足す → S5 で確かめる
6. US5 を足す → S6 で確かめる
7. Polish → S1・S8・S9

各段で 004 の受け入れ基準（FR-018 / SC-009）は壊れていないこと。壊れたら**その段で直す**
（最後にまとめて確かめるものではない）。

---

## Notes

- [P] のタスク = 別ファイル・依存なし
- `api/openapi.yaml`・`web/src/api/gen/`・`internal/`・`cmd/` には触れない。触れたら
  FR-014 の防波堤が崩れている
- **依存を 1 つも増やさない**（実行時・開発時とも）。アイコン集を入れたくなったら
  [R-510](./research.md) を読み直す
- 生の色・生の px・Tailwind 既定のパレット名を `web/src/**` に書かない。004 の
  `noRawColors.test.ts` が落とす
- トークンを足したら [contracts/design-tokens.md](./contracts/design-tokens.md) 3. の
  対比表にも足す。**表に無い組は検査されない**
- 「機能する」要素を増やさない。`navigation.ts` の `live` は 2 つだけで、T022 がその数を
  数えている。増やしたくなったら FR-013 に反していないかを先に確かめる
- タスクごと、または意味のまとまりごとにコミットする
- 各 Checkpoint で止めてストーリー単位に確かめられる
