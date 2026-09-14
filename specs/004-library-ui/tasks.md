# Tasks: 現在の機能を前提とした UI の実装

**Input**: Design documents from `/specs/004-library-ui/`

**Prerequisites**: [plan.md](./plan.md)（必須）、[spec.md](./spec.md)（ユーザーストーリー）、[research.md](./research.md)、[data-model.md](./data-model.md)、[contracts/](./contracts/)、[quickstart.md](./quickstart.md)

**Tests**: 本機能はテストを**含む**。spec が SC-006（対比 100%）・SC-007（表示設定が 100%
保たれる）を機械で確かめられる形で求めており、FR-025／SC-009 が「既存の振る舞いが
変わらないこと」を求めているためである。`web/src/` のほぼ全域を書き換えながら人の目だけで
これを保証することはできない（[plan.md](./plan.md) Complexity Tracking）。実行基盤は
[R-406](./research.md) の Vitest + Testing Library を本機能で導入する
（[TD-004](../../docs/exec-plans/tech-debt.md) が名指しした契機がこの機能である）。

機械で確かめるのは [quickstart.md](./quickstart.md) S1・S2 の範囲（対比・生の色の禁止・
表示設定・一覧の復元・視聴状況の読み替え・既存の 3 点）で、見た目・画面幅・読み上げ・
反応時間は S3〜S8 で人が確かめる。

**Organization**: タスクはユーザーストーリー単位にまとめてある。見た目のトークン
（[contracts/design-tokens.md](./contracts/design-tokens.md)）は一覧・再生の両方が使うので
Foundational に置く。既存の振る舞いを守る 3 つの回帰テスト（TD-004）も、書き換えの前に
用意しておかないと退行を検出できないので Foundational に置く。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 並行実行可（別ファイル・依存なし）
- **[Story]**: 対応するユーザーストーリー（US1 / US2 / US3 / US4）
- 説明には必ず対象ファイルのパスを書く

## Path Conventions

本機能の変更は **`web/` に閉じる**。`api/openapi.yaml`・`internal/`・`cmd/`・
`web/src/api/gen/` には触れない。これは方針であると同時に FR-024・FR-025 に対する構造上の
防波堤である（[R-412](./research.md)）。

- 画面: `web/src/pages/`（`LibraryPage.tsx`・`VideoPage.tsx`）
- 部品: `web/src/components/`
- 新設: `web/src/theme/`（検査のみ）・`web/src/preferences/`（表示設定）
- 見た目の規則の唯一の真実: `web/src/index.css` の `@theme`（[R-401](./research.md)）
- `web/src/api/client.ts` は**変更しない**。`useVideos.ts` の変更は復元の受け渡し口だけに限る

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 単体テストの実行基盤を用意する。ここが無いと以降のタスクの検証が書けない

- [X] T001 実行計画 `docs/exec-plans/active/004-library-ui.md` を作成する。書式は [docs/exec-plans/active/003-sdd-loop-harness.md](../../docs/exec-plans/active/003-sdd-loop-harness.md) に合わせ、ステータス（進行中）・最終更新日・対象・目的・一次資料の表（spec／plan／research／data-model／contracts／quickstart へのリンク）・検証の方針（[quickstart.md](./quickstart.md) S1〜S10 で完了を判定し、S3〜S8 は人が確かめると明記する）・フェーズごとの進捗の空欄・決定の記録を置く
- [X] T002 `web/package.json` の `devDependencies` に `vitest`・`@testing-library/react`・`@testing-library/user-event`・`jsdom` の 4 つを足す（[R-406](./research.md)）。**`dependencies`（実行時の依存）は 1 つも増やさない**（plan の Constitution Check G2）。`scripts.test` を `vite build --outDir .vite-build-check --emptyOutDir && vitest run` に変える（既存のビルド検証は残す。[quickstart.md](./quickstart.md) S1 の期待がビルドと単体テストの両方であるため）。`npm install` を実行して `web/package-lock.json` を更新する
- [X] T003 `web/vite.config.ts` に `test` の項を足す（`environment: "jsdom"`、`globals: true`、`setupFiles: ["./vitest.setup.ts"]`、`css: false`）。あわせて `web/vitest.setup.ts` を新規作成し、`@testing-library/jest-dom` を入れずに `afterEach(() => cleanup())` だけを置く（実行時にも開発時にも依存を増やさないため、表明は Vitest の `expect` を使う）
- [X] T004 `Makefile` の `test-web` のコメントを実態に合わせて直す（現在の「Phase 0 は E2E を入れない」は、単体テストが無いことの説明として読まれている）。`$(NPM) run test` がビルド検証と `vitest run` の両方を走らせることを 1 行で書く。ターゲットの依存（`web/node_modules`）と `check` からの呼ばれ方は変えない

**Checkpoint**: `make test-web` がビルド検証を通り、テストが 0 件でも成功で終わる

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 見た目の規則（唯一の真実）と、書き換えの前に張っておく回帰の網

**⚠️ CRITICAL**: T005 のトークンがすべての画面タスクの前提である。T010〜T013 は**既存の
振る舞いに対して**書くもので、US1・US2 の書き換えより**先に**緑にしておかないと、
FR-025／SC-009 の退行を検出できない

### 見た目のトークン（FR-001 / FR-004）

- [X] T005 `web/src/index.css` の `@theme` に [contracts/design-tokens.md](./contracts/design-tokens.md) 2.・3. の表をそのまま定義する。色 14 個（`--color-surface: #0f1115`、`--color-surface-raised: #191d24`、`--color-surface-sunken: #07090c`、`--color-badge: #12151a`、`--color-border: #626d7d`、`--color-body: #e8ecf2`、`--color-muted: #a7b1c0`、`--color-accent: #63a9f2`、`--color-accent-ink: #07131f`、`--color-danger: #ff9b9b`、`--color-danger-surface: #2c1517`、`--color-warning: #f3c274`、`--color-warning-surface: #2b2113`、`--color-focus: #8ab4ff`）と、形と大きさ 6 個（`--radius-card: 0.75rem`、`--radius-control: 0.5rem`、`--size-tap: 2.75rem`、`--size-tile-dense: 9rem`、`--size-tile-standard: 11rem`、`--size-tile-relaxed: 16rem`）。あわせて `:root` に `color-scheme: dark` を宣言し、`body` を `bg-surface text-body` にする。**明暗の切り替え機構は入れない**（[R-402](./research.md)）
- [X] T006 [P] `web/src/theme/tokens.test.ts` を書く。`web/src/index.css` をファイルとして読み、`@theme` ブロックから `--color-*` の 16 進値を取り出し、sRGB → 相対輝度 → 対比を WCAG の式で計算する（外部の検査ライブラリは使わない。[R-405](./research.md)）。検査する組は [contracts/design-tokens.md](./contracts/design-tokens.md) 2. の対比表 17 行をそのまま表としてテスト側に持ち、各組が「必要」（`body`・`muted`・`accent`・`accent-ink`・`danger`・`warning` の組は 4.5、`border`・`focus`・`accent`/`surface-sunken` の組は 3.0）以上であることを表明する。表に載っているトークンが CSS に無い場合も失敗にする（写しと実体のずれを検出する）
- [X] T007 [P] `web/src/theme/noRawColors.test.ts` を書く。`web/src/**/*.tsx` を走査し、(1) `#rgb`／`#rrggbb` の直書き、(2) Tailwind 既定のパレット名（`neutral-`・`sky-`・`red-`・`amber-`・`slate-`・`gray-`・`zinc-`・`stone-` などに続く数値）を含む実用クラス、(3) `rgb(`／`rgba(`／`hsl(` の直書き が 1 件も無いことを表明する。見つかったときは**ファイル名と行番号と該当文字列**を出す（直す場所が分かるようにする）。`web/src/api/gen/` は除外する（生成物。[AGENTS.md](../../AGENTS.md)）。この禁止が FR-001 の実体である（[contracts/design-tokens.md](./contracts/design-tokens.md) 1.）

### 両画面が使う部品（FR-002）

- [X] T008 [P] `web/src/components/Skeleton.tsx` を新規作成する。通信中の骨組みを描く部品で、`aspect-video`（一覧の項目用）と任意の高さ（行用）の 2 通りを引数で選べるようにする。地は `bg-surface-raised`、角は `rounded-card`。明滅の動きは `motion-reduce:` 変種で止める（FR-023 / [R-410](./research.md)）。個々の骨組みは読み上げに渡さない（`aria-hidden`）。領域全体へ「読み込み中」を伝えるのは呼び出し側の責任であることを doc コメントに書く（[contracts/screen-states.md](./contracts/screen-states.md) 3. 読み上げ）
- [X] T009 [P] `web/src/components/StateNotice.tsx` を新規作成する。空・失敗・警告・お知らせの共通の枠で、`tone`（`"empty"` / `"danger"` / `"warning"` / `"info"`）で地と文字のトークンを切り替える（`danger` は `bg-danger-surface text-danger`、`warning` は `bg-warning-surface text-warning`、それ以外は `bg-surface-raised text-body`）。見出し・本文・任意の操作（子要素）を受け取る。枠は `rounded-card border border-border`。**この部品は画面全体を置き換えない**（FR-002 が求めるのは、失敗しても画面のほかの部分が使えることである）

### 既存の振る舞いを守る網（FR-025 / SC-009 / TD-004）

- [X] T010 [P] `web/src/api/useVideos.test.ts` を書く。`web/src/api/client.ts` の `listVideos` を `vi.mock` で差し替え、(1) 1 ページ目のあと `loadMore()` が**直前の応答の `nextCursor` をそのまま**次の要求に渡すこと、(2) `nextCursor` が未設定なら `hasMore` が偽になり `loadMore()` が要求を出さないこと、(3) `sort` または `query` が変わったら**カーソルを引き継がずに**先頭から読み直すこと、(4) 連続して変えたときに古い応答が新しい一覧を上書きしないこと（`AbortController` による打ち切り）を表明する。TD-004 が名指しした 1 点目である。**このテストは US1・US2 の書き換えのあとも同じ内容で通ること**
- [X] T011 [P] `web/src/pages/LibraryPage.search.test.tsx` を書く。`client.ts` を差し替え、`user-event` で検索欄に連続して打鍵し、(1) 打鍵のたびに要求が出ず、入力が止まってから **250ms**（`searchDebounceMs`）後に 1 回だけ出ること、(2) 打ち終えた値で必ず 1 回引くこと（取りこぼしが無い）、(3) 待ち合わせ中に次の打鍵が来たら前の待ちが破棄されること を表明する。時間は `vi.useFakeTimers()` で進める。描画は `MemoryRouter` の中で行う（`VideoCard` が `Link` を使うため。T015 で `useSearchParams` へ移したあともこの書き方のまま通ること）。TD-004 が名指しした 2 点目である
- [X] T012 [P] `web/src/pages/VideoPage.progress.test.tsx` を書く。`client.ts` の `saveProgress`・`beaconProgress` を差し替え、(1) 再生中は **5 秒ごと**（`saveIntervalMs`）に位置が送られること、(2) 同じ位置を繰り返し送らないこと（前回との差が 1 秒未満なら送らない）、(3) 画面を離れるとき（`visibilitychange` で hidden、および unmount）に `beaconProgress` で最後の位置が送られること、(4) `saveProgress` が失敗しても例外が外へ出ないこと を表明する。TD-004 が名指しした 3 点目である
- [X] T013 [P] `web/src/components/VideoCard.watchState.test.ts` を書く。`partialRatio` と `unplayableText` に対して [data-model.md 3.](./data-model.md) の読み替えの表を検査する。とくに **`durationMs` が取れていない（`undefined` または 0 以下）動画は、`positionMs` が正でも `null` を返す**こと（帯を描かず、割合を偽らない）、`progress.completed === true` なら `null` を返すこと、`ratio` が 1 を超えないこと を表明する。`unplayableText` は `probeState` が `"pending"`／`"failed"` のときに `unplayableReason` より優先されることを表明する

**Checkpoint**: `make test-web` が緑。この時点で `web/src/index.css` にトークンがあり、
既存の 3 点の回帰テストが**書き換え前の実装に対して**通っている

---

## Phase 3: User Story 1 - 一覧が「自分のライブラリ」に見える (Priority: P1) 🎯 MVP

**Goal**: 一覧を開いた瞬間に自分の動画置き場だと分かる。静止画が主役で並び、題名・長さ・
視聴状況が決まった位置にあり、探す・並べ替える・取り込むの 3 つがスクロール位置によらず
常に画面上にある。

**Independent Test**: 動画を数十本置いた状態で一覧を開き、[quickstart.md](./quickstart.md)
S3 の 1〜5 を確かめる。密度の選択（US3）と復元（US2）が無くても、この 5 つは単独で成立する。

- [X] T014 [US1] `web/src/components/Toolbar.tsx` を新規作成する。`position: sticky; top: 0` の帯で、背景は `bg-surface`（**透過させない**。背後の静止画で対比が変わると SC-006 を静的に保証できない。[R-404](./research.md)）、下端に `border-b border-border`。子として「探す」「並べ替える」「密度」「取り込む」を**この順**で受け取る（[contracts/screen-states.md](./contracts/screen-states.md) 3. の到達順がそのまま DOM の順である）。帯の中の操作できる要素（検索欄・選択欄・ボタン）すべてに `focus-visible:outline-2 focus-visible:outline-focus outline-offset-2` を与え、狙いを合わせている状態が見た目で分かるようにする（FR-003。輪郭は要素の**外側**に描き、隣に隠れない）。`:focus-visible` を使うのは、ポインタで押しただけでは印を出さないためである。密度はこの段階では受け取り口だけ用意し、US3 で中身を入れる
- [X] T015 [US1] `web/src/pages/LibraryPage.tsx` の検索語と並び順を、コンポーネントの `useState` から `react-router` の `useSearchParams`（`/?q=...&sort=...`）へ移す（[R-403](./research.md) 1.）。入力欄の値（`input`）は打鍵の受け皿として `useState` のまま残し、250ms の待ち合わせのあと**クエリを書き換える**形にする（`replace: true` で履歴を汚さない）。`sort` の値は `web/src/api/gen/openapi.ts` 由来の `VideoSort` で検証し、想定外なら既定値に落とす。待ち合わせ時間（`searchDebounceMs = 250`）と `MAX_QUERY_LENGTH` は変えない（T011 が守る）
- [X] T016 [US1] `web/src/components/ScanStatus.tsx` を帯の中で成立する形に直す。振る舞い（2 秒ごとの巡回、実行中だけ見に行く、`onFinished`）は**変えない**（[contracts/screen-states.md](./contracts/screen-states.md) 4.）。変えるのは見た目だけで、生の色を `bg-surface-raised`・`text-muted`・`text-danger` などのトークンへ置き換え、進行中は「済んだ数 / 総数」を `text-muted` の 1 行で示し、ボタンを `rounded-control border border-border` かつ `min-h-[--size-tap] min-w-[--size-tap]` にする（FR-010 / FR-022）。進捗の帯を描く場合は `aria-hidden` にし、意味は状態の文言が持つ
- [X] T017 [US1] `web/src/components/VideoCard.tsx` の見た目をトークンへ移し、[contracts/screen-states.md](./contracts/screen-states.md) 1. 「一覧の 1 項目」の表を満たす。枠は `aspect-video rounded-card bg-surface-sunken` 固定（サムネイルの有無で大きさが変わらない。SC-003）、サムネイル未生成は枠の中に「画像を準備中」／「画像を作れませんでした」、長さは枠の右下に `bg-badge`（**半透明にしない**）、視聴済みは `bg-accent text-accent-ink` の印、途中は `bg-accent` の帯、再生できない理由は枠の左上に `bg-warning-surface text-warning`。狙いを合わせた印は `focus-visible:outline-2 focus-visible:outline-focus outline-offset-2`（要素の外側に描き、隣に隠れない）。`img` に `decoding="async"` を足す（`loading="lazy"` は残す）
- [X] T018 [US1] `web/src/components/VideoCard.tsx` に題名の全文への到達手段を 3 つ入れる（[R-409](./research.md) / FR-011）: (1) リンクのアクセシブル名は**常に全文**（見た目の省略に引きずられない）、(2) ポインタ向けに `title` 属性で全文、(3) `group-focus-visible:` で省略（`line-clamp-2`）を解く。解けた題名は**枠の上に重ねて描き、項目の高さを変えない**（FR-005 / SC-003）。サムネイルの `img` は `alt=""` のままにする（題名はリンク名が持つ）
- [X] T019 [US1] `web/src/components/VideoCard.test.tsx` を書く。(1) 長い題名でもリンクのアクセシブル名が全文であること、(2) `img` のアクセシブル名が空であること、(3) 視聴済みの項目で「視聴済み」が読めること、(4) 途中の項目で「N% まで再生済み」が読めること（`aria-label`）、(5) サムネイル未生成の項目で理由が読めること を表明する（FR-021 / [contracts/screen-states.md](./contracts/screen-states.md) 3.）
- [X] T020 [US1] `web/src/pages/LibraryPage.tsx` に 4 状態を入れる（[contracts/screen-states.md](./contracts/screen-states.md) 1. の表）。**通信中**は帯を先に出し（探す・並べ替える・取り込むが押せる）、項目の場所に `Skeleton` を並べ、領域に「読み込み中」を伝える。**成功**は格子。**空（蔵書 0）**は置き場所と「取り込む」への案内、**空（該当 0）**は検索語と「検索語を消す」（2 つは別の文言であること。FR-009）。**失敗**は `StateNotice` の `danger` と再試行の入口を出し、**帯は残して操作できる状態に保つ**。続きのページの読み込み中は、既に読めている項目を骨組みに置き換えず、末尾に細い知らせを出す
- [X] T021 [US1] `web/src/pages/LibraryPage.tsx` の件数の文言を [contracts/screen-states.md](./contracts/screen-states.md) 1.「件数の文言」の 3 行にそろえ（検索していない = 「N 本」、検索している = 「「<検索語>」に一致 N 本」、読み込み中 = 「読み込み中…」）、その要素を変化を知らせる領域（`role="status"` / `aria-live="polite"`）にする（FR-008 / FR-021）
- [X] T022 [US1] `web/src/pages/LibraryPage.tsx` の一覧の項目に `content-visibility: auto` と `contain-intrinsic-size`（枠の見込みの大きさ）を与える（[R-408](./research.md) / SC-008）。仮想スクロールのライブラリは**入れない**。`contain-intrinsic-size` を必ず与えること（省いた項目の高さを 0 と見積もらせるとスクロールバーが暴れる）。あわせて `web/src/pages/LibraryPage.tsx` の格子を `grid-template-columns: repeat(auto-fill, minmax(min(var(--tile-min), (100% - var(--tile-gap)) / 2), 1fr))` へ移す（列数を JavaScript で計算しない）。この段階では `--tile-min` を `var(--size-tile-standard)`（11rem）に、`--tile-gap` を `1rem` に固定して置く。**US3 が入らなくても US1 だけで格子が成立していること**が要件で、US3（T035）は `--tile-min` の指す先を密度に連動させるだけである
- [X] T023 [US1] `web/src/pages/LibraryPage.test.tsx` を書く。`client.ts` を差し替え、(1) 4 状態それぞれで期待するものが出ること（通信中に帯が押せる／失敗しても帯が残る）、(2) 空（蔵書 0）と空（該当 0）の文言が**互いに異なる**こと、(3) Tab の到達順が「探す → 並べ替える → 取り込む → 一覧の項目（並んでいる順）」であること（[contracts/screen-states.md](./contracts/screen-states.md) 3.）、(4) 件数の文言が検索の前後で変わること を表明する

**Checkpoint**: 一覧が単独で成立する。[quickstart.md](./quickstart.md) S3 の 1〜5 を人が
確かめられる

---

## Phase 4: User Story 2 - 再生画面が同じ規則で整い、見ることに集中できる (Priority: P2)

**Goal**: 映像が主役になり、情報は映像の外にまとまる。続きから始まったこと・再生できない
形式であることは映像を隠さない決まった位置に出る。一覧へ戻る道は常に見えていて、戻ると
離れたときの一覧に帰る。

**Independent Test**: 一覧から 1 本選んで再生し、[quickstart.md](./quickstart.md) S8 の
1〜5 を確かめる。戻ったときに位置・検索語・並び順が保たれる（S5 の最後）。

- [ ] T024 [US2] `web/src/pages/VideoPage.tsx` の並びを [contracts/screen-states.md](./contracts/screen-states.md) 2.「並び」の 5 段に固定する（上から: 一覧へ戻る → 題名 → 知らせの置き場 → 映像 → 情報欄）。**知らせが映像より上にある**ことが要点で、映像の上に重ねてはならない（FR-014）し、情報欄の下に置いてもならない（気付かれない）。映像は `bg-surface-sunken rounded-card w-full` で、比率を保ったまま画面幅に収まること（FR-012）。「一覧へ戻る」は最初の到達先である（FR-016 / [contracts/screen-states.md](./contracts/screen-states.md) 3.）
- [ ] T025 [US2] `web/src/pages/VideoPage.tsx` の `VideoFacts` を [data-model.md 3.](./data-model.md)「取れていない値の扱い」に従わせる。**空欄にしない**: `probeState` が `"pending"` なら「確認中」、`"failed"` なら「読み取れませんでした」（`probeError` があれば併記）、`"done"` で値が無いなら「なし」。対象は長さ・解像度・形式・映像・音声で、`sizeBytes` と `addedAt` は必須項目なのでこの分岐に入らない（FR-013）。見た目はトークンへ移し、項目名を `text-muted`、値を `text-body` にする
- [ ] T026 [US2] `web/src/pages/VideoPage.tsx` に 4 状態を入れる（[contracts/screen-states.md](./contracts/screen-states.md) 2. の表）。**通信中**は「一覧へ戻る」を先に出し、映像の場所に 16:9 の `Skeleton`。**失敗**（取得の失敗・id が不正）は `StateNotice` の `danger` と「一覧へ戻る」を出し、映像は出さない。再生できない形式の警告は `warning`、再生時の失敗は `danger` で、いずれも 3 の位置に出す（FR-015）。この画面に「空」は無い
- [ ] T027 [P] [US2] `web/src/api/listSnapshot.ts` を新規作成する（[data-model.md 2.](./data-model.md)）。モジュール変数に**直近の 1 件だけ**を持ち、`key`（`q` と `sort` を正規化して連結した文字列）・`items`・`total`・`cursor`・`hasMore`・`scrollY` を保持する。公開するのは `saveListSnapshot`・`takeListSnapshot(key)`・`clearListSnapshot` の 3 つ。`takeListSnapshot` は**鍵が一致しなければ `undefined` を返す**（復元できないことは異常ではない）。`localStorage`・`sessionStorage` には**書かない**（メモリだけ。[R-403](./research.md)）
- [ ] T028 [P] [US2] `web/src/api/listSnapshot.test.ts` を書く。(1) 保存した鍵と同じ鍵で取れること、(2) **鍵が違えば取れないこと**、(3) `clearListSnapshot` のあとは取れないこと、(4) 2 件目を保存すると 1 件目が捨てられること（数千件の `Video` を鍵ごとに溜めない）、(5) 鍵の正規化が `q` の前後の空白と `sort` の既定値を吸収すること を表明する
- [ ] T029 [US2] `web/src/api/useVideos.ts` に復元状態の受け渡し口だけを足す（[R-412](./research.md) が範囲を限定している）。初期状態を外から与えられるようにし（与えられたら 1 ページ目の取得を**行わない**）、現在の `items`・`total`・`cursor`・`hasMore` を読み出せるようにする。**それ以外の振る舞い（カーソルの引き継ぎ、打ち切り、`reload`）は変えない** — T010 が変わっていないことを守る。`web/src/api/client.ts` には触れない
- [ ] T030 [US2] `web/src/pages/LibraryPage.tsx` に復元を入れる（FR-016 / [data-model.md 2.](./data-model.md)）。(1) 一覧を離れる瞬間（項目のリンクを踏んだとき）に `saveListSnapshot` へ現在の状態と `window.scrollY` を書く、(2) 一覧を開く瞬間に `takeListSnapshot` で読み、あれば `useVideos` の初期状態として渡す、(3) **項目を描いたあと**に `useLayoutEffect` で `scrollTo({ behavior: "auto" })` して位置を戻す（中身が無いうちに呼ぶと文書の高さが足りず途中で止まる）、(4) `history.scrollRestoration = "manual"` にしてブラウザ自身の復元を止める、(5) 取り込みが終わって読み直すとき（`ScanStatus` の `onFinished`）は `clearListSnapshot` する（古い一覧に戻してはならない）
- [ ] T031 [US2] `web/src/pages/VideoPage.test.tsx` を書く。(1) 通信中・失敗のそれぞれで「一覧へ戻る」が先に出ること、(2) 失敗のとき映像が出ないこと、(3) `probeState` の 3 通りで情報欄の言い分けが [data-model.md 3.](./data-model.md) のとおりであること（空欄にならない）、(4) 再生できない形式で**再生を試みる前に**警告が出ること、(5) 続きから始まった知らせと「先頭から見直す」が**映像より前**の DOM 位置に出ること を表明する

**Checkpoint**: 一覧と再生の 2 画面が同じ規則で整い、往復して位置が保たれる

---

## Phase 5: User Story 3 - 表示の仕方を自分で選べる (Priority: P3)

**Goal**: 一覧の密度を選べ、選んだ設定はその端末で保たれる。壊れていても既定値で画面が出る。

**Independent Test**: [quickstart.md](./quickstart.md) S6 の 1〜6 と「壊れた値の確認」。

- [ ] T032 [P] [US3] `web/src/preferences/viewPreferences.ts` を新規作成する（[contracts/view-preferences.md](./contracts/view-preferences.md)）。公開するのは `readViewPreferences(storage?: Storage): ViewPreferences` と `writeViewPreferences(value: ViewPreferences, storage?: Storage): void` の**2 つだけ**で、どちらも**決して投げない**。鍵は `vv.view.v1`、値は `{ "density": "standard", "sort": "addedDesc" }`。`density` の取りうる値は `"dense"` / `"standard"` / `"relaxed"`（既定 `"standard"`）、`sort` は `web/src/api/gen/openapi.ts` の `VideoSort` をそのまま使う（既定 `"addedDesc"`。**ここで列挙を書き写さない** — 契約に値が増えたときに型検査で気付けるようにするため。[data-model.md 1.](./data-model.md)）。読み出しは契約 3. の 7 段を上から判定し、**5・6 は項目ごと**に既定値へ落とす（片方が壊れただけで利用者の選択を丸ごと捨てない）。書き込みは上の 2 項目だけを書き、読んだときに残っていた未知の項目は引き継がない。書き込みの失敗は黙って捨てる
- [ ] T033 [P] [US3] `web/src/preferences/viewPreferences.test.ts` を書く。偽の `Storage`（`getItem` が投げる／`null` を返す／壊れた JSON を返す／配列を返す／片方だけ壊れた値を返す／正しい値を返す／`setItem` が投げる）を渡し、[contracts/view-preferences.md](./contracts/view-preferences.md) 3. の 7 つの場合**すべて**で完全な値が返り例外が外へ出ないこと、書き込みの失敗を握りつぶすこと、未知の項目を引き継がないこと を表明する（FR-019 / SC-007）
- [ ] T034 [US3] `web/src/components/DensitySelect.tsx` を新規作成する。「細かい」「標準」「ゆったり」の 3 つから選ぶ選択欄で、`rounded-control border border-border bg-surface-raised text-body` かつ当たり判定が `--size-tap`（44px）四方以上（FR-017 / FR-022）。ラベルは読み上げに渡す（見た目のラベルが無い場合は `sr-only`）。`web/src/components/Toolbar.tsx` の 3 番目の口（T014 で用意した）に差し込む
- [ ] T035 [US3] `web/src/pages/LibraryPage.tsx` の格子の最小列幅を密度に連動させる（[contracts/design-tokens.md](./contracts/design-tokens.md) 3.）。T022 で固定値にしてある `--tile-min` を、密度に応じて `--size-tile-dense`（9rem）／`--size-tile-standard`（11rem）／`--size-tile-relaxed`（16rem）のどれかへ指し替える（`style` で要素に与える）。列の式（`repeat(auto-fill, minmax(min(var(--tile-min), (100% - var(--tile-gap)) / 2), 1fr))`）と `--tile-gap`（`1rem`）は T022 のまま変えない。**列数を JavaScript で計算しない**。初期値は `readViewPreferences()` の `density`
- [ ] T036 [US3] `web/src/pages/LibraryPage.tsx` に密度変更時の位置合わせを入れる（[R-411](./research.md) / US3 受け入れ 1）。密度を変える**直前**に画面上端に最も近い項目の id を覚え、変更後にその項目を `scrollIntoView({ block: "start", behavior: "auto" })` で画面上端へ戻す。座標ではなく**項目**を基準にすること（列幅が変われば同じ座標は別の項目を指す）。あわせて密度を変えたら `writeViewPreferences` で `density` を書く（`sort` は現在値のまま）
- [ ] T037 [US3] `web/src/pages/LibraryPage.tsx` の並び順に表示設定を効かせる（[data-model.md 1.](./data-model.md)「並び順の 2 つの役割」）。`/` をクエリ無しで開いたときは表示設定の `sort` を初期値にし、`/?sort=titleAsc` で開いたときは **URL の値が優先**される。画面で並び順を変えたら URL を書き換え、同時に `writeViewPreferences` で `sort` も書く（次回の初期値になる）。**検索語は保存しない**（次に開いたとき前回の検索で絞られていると、動画が消えたように見える。[contracts/view-preferences.md](./contracts/view-preferences.md) 4.）

**Checkpoint**: 密度と並び順が選べ、再読み込みと画面遷移のあとも保たれる

---

## Phase 6: User Story 4 - 手元の小さな画面でも破綻しない (Priority: P4)

**Goal**: 幅 360px でも一覧は読める形で並び、操作は指で押せる大きさがあり、再生画面は
映像が主役のまま収まる。

**Independent Test**: [quickstart.md](./quickstart.md) S4（幅 360px・768px・1280px・2560px）。

- [ ] T038 [US4] `web/src/components/Toolbar.tsx` を狭い画面で**2 行に折り返す**形にする（重ねない。行が増えた分だけ帯が高くなる。[R-404](./research.md)）。帯の中の 4 つ（探す・並べ替える・密度・取り込む）が幅 360px・768px・1280px・2560px のいずれでも重ならないこと。検索欄は残りの幅へ伸び、固定幅（現在の `w-48`）をやめる
- [ ] T039 [US4] `web/src/components/Toolbar.tsx`・`web/src/components/DensitySelect.tsx`・`web/src/components/ScanStatus.tsx`・`web/src/pages/VideoPage.tsx` の押せる要素すべてに `--size-tap`（44px）四方以上の当たり判定を与える（FR-022 / [contracts/screen-states.md](./contracts/screen-states.md) 3. 指）。一覧の項目そのものは 44px より大きいので対象外である
- [ ] T040 [US4] `web/src/pages/LibraryPage.tsx` の格子に `min(var(--tile-min), (100% - var(--tile-gap)) / 2)` の下限が効いていることを確かめ、**幅 360px でも 1 列にならない**こと・横方向のスクロールが出ないことを満たす（[contracts/design-tokens.md](./contracts/design-tokens.md) 3.）。狭い画面では 3 つの密度の見た目が同じになるが、これは意図した動作である（「狭い画面で 1 列まで大きくする」ことに利用者の利益が無い）。一覧の左右の余白を `--tile-gap` と同じ `1rem` にそろえる（[contracts/design-tokens.md](./contracts/design-tokens.md) 3. の列数の目安がこの前提で計算されている）
- [ ] T041 [US4] `web/src/pages/VideoPage.tsx` を狭い画面で成立させる。映像は画面幅に収まり比率を保つ（`w-full` + `aspect-video` ではなく、映像自身の比率を保ったまま `max-w-full` にする）。情報欄は狭いときに縦 1 列へ落ちる（現在の `grid-cols-[auto_1fr]` を折り返す形にする）。横方向のスクロールを発生させない（FR-022 / SC-004）

**Checkpoint**: 4 つの幅で横スクロールが無く、操作要素が重ならない

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 全画面に効く仕上げと、文書・記録・受け入れ検証

- [ ] T042 [P] `web/src/components/VideoCard.tsx`・`web/src/components/Skeleton.tsx`・`web/src/components/ScanStatus.tsx`・`web/src/components/Toolbar.tsx` の装飾的な動き（ホバーの遷移、骨組みの明滅、進捗帯の伸び）をすべて `motion-reduce:` 変種で無効にする（FR-023 / [R-410](./research.md)）。**無効にするのは動きだけ**で、色・不透明度の最終状態と狙いを合わせた印（`--color-focus`）は常に適用する。判定を JavaScript に持たない（`matchMedia` を読まない）
- [ ] T043 [P] `ARCHITECTURE.md` の Web 層の記述を更新する。見た目の規則の置き場が `web/src/index.css` の `@theme` 1 か所であること、`web/src/theme/` と `web/src/preferences/` の役割、単体テストの基盤が Vitest + Testing Library（`jsdom`）で `make test-web` から走ること、`web/src/api/` が「サーバーとのやり取り」の層として保たれていること を書く。依存方向の記述は変えない
- [ ] T044 [P] `docs/design-docs/library-ui-design-system.md` を新規作成し、`docs/design-docs/index.md` の Documents に追記する（[AGENTS.md](../../AGENTS.md) の working agreements）。内容は「なぜ見た目の規則を CSS の 1 か所に置き、対比をテストで保証するのか」「なぜ暗い配色だけを実装し、明暗の切り替えを入れないのか」「なぜ仮想スクロールを入れずに `content-visibility` に任せるのか」の 3 点で、詳細は [research.md](./research.md) の R-401・R-402・R-405・R-408 を参照する形にする（写さない）
- [ ] T045 [P] `docs/exec-plans/tech-debt.md` の [TD-004](../../docs/exec-plans/tech-debt.md)（Web の自動テストはビルド検証のみ）を解消済みにする。解消した変更（本機能）と、名指しされていた 3 点がどのファイルのテストになったか（`web/src/api/useVideos.test.ts`・`web/src/pages/LibraryPage.search.test.tsx`・`web/src/pages/VideoPage.progress.test.tsx`）を書く。**TD-006 と TD-007 には触れない**（本機能の範囲外）
- [ ] T046 `make check` を通す（[quickstart.md](./quickstart.md) S2）。`fmt-check` → `lint` → `test` → `generate-check` のすべてが成功すること。**`generate-check` が通ることは `api/openapi.yaml` を変えていないことの証明**である（FR-024）。`web/src/api/gen/` に差分が出ていたら、それは触ってはならないものを触っている
- [ ] T047 [quickstart.md](./quickstart.md) S0 に従い検証用の動画 3 本（長い題名・音の無い動画・長い動画）を用意し、S3〜S6・S8 を人が実行して結果を `docs/exec-plans/active/004-library-ui.md` の進捗に記録する。SC-001（3 つの入口を 10 秒以内に指し示せる）だけは 5 人の協力が要るため、実施できない場合は**実施できなかったことを記録する**（できたことにしない）
- [ ] T048 [quickstart.md](./quickstart.md) S7（規模と反応）を実行する。1万本で最初の画面が 2 秒以内（SC-002）、1000 件まで読み込んだ状態で反応の開始が 100ms 以内（SC-008）。**SC-008 を満たせなかった場合は仮想スクロールへ進まず、[docs/exec-plans/tech-debt.md](../../docs/exec-plans/tech-debt.md) に TD として記録する**（[plan.md](./plan.md)「Phase 2 以降へ送る判断」）
- [ ] T049 [quickstart.md](./quickstart.md) S9 を実行する。`make check` と `go test ./...` に続き、[002 の quickstart](../002-core-video-library/quickstart.md) の **S1〜S10 をそのまま実行する**（SC-009 / FR-025）。S5 は [TD-006](../../docs/exec-plans/tech-debt.md) のとおり S0 で足した「長い動画.mp4」を対象にする。結果を `docs/exec-plans/active/004-library-ui.md` に記録する
- [ ] T050 [quickstart.md](./quickstart.md) S10 に従い、[docs/how-to/ui-change-screenshots.md](../../docs/how-to/ui-change-screenshots.md) の手順で 4 枚（一覧 1280px 密度「標準」／一覧 360px ／再生（途中から再開）／検索して該当なし）を撮り、**新しい日付**の名前で `docs/screenshots/` に置く。既存の 4 枚（`20260913-*`）は置き換えない。PR に画像を添える（[AGENTS.md](../../AGENTS.md) の working agreements）
- [ ] T051 `docs/exec-plans/active/004-library-ui.md` を完了にし、`docs/exec-plans/completed/004-library-ui.md` へ移す（[AGENTS.md](../../AGENTS.md)）。残った妥協は [docs/exec-plans/tech-debt.md](../../docs/exec-plans/tech-debt.md) に TD として記録する

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: 依存なし。すぐ始められる
- **Foundational (Phase 2)**: Setup の完了に依存し、**すべてのユーザーストーリーを塞ぐ**
- **US1 (Phase 3)**: Foundational の完了後
- **US2 (Phase 4)**: Foundational の完了後。T030（一覧の復元）だけは US1 の T015（URL の
  クエリ）を前提にする — 鍵が `q` と `sort` でできているため
- **US3 (Phase 5)**: Foundational の完了後。T034 は US1 の T014（帯の 3 番目の口）を、
  T035・T036 は US1 の T022（格子）を前提にする
- **US4 (Phase 6)**: US1 と US2 の完了後（両画面の形が決まっていないと幅を詰められない）
- **Polish (Phase 7)**: 必要なストーリーがすべて終わってから

### User Story Dependencies

- **US1 (P1)**: 他のストーリーに依存しない。単独で「フォルダを開く代わり」として成立する
- **US2 (P2)**: 画面としては US1 に依存しないが、T030 の復元は US1 の URL の形に乗る
- **US3 (P3)**: US1 の帯と格子に差し込む。US2 とは独立
- **US4 (P4)**: US1・US2 が済んでいれば追加の機能なしに満たせる（spec の Why this priority）

### Within Each User Story

- 部品 → 画面 → その画面のテスト の順
- Foundational のトークン（T005）より前に画面を触らない（生の色を書いて T007 に落ちる）
- 既存の振る舞いの回帰テスト（T010〜T012）は**書き換えより前**に緑にする

### Parallel Opportunities

- Setup の T002 → T003 → T004 は同じ設定群なので順に行う（T001 は独立で [P]）
- Foundational の T006・T007（検査）、T008・T009（部品）、T010〜T013（回帰）は
  それぞれ別ファイルで並行できる（T005 のあと）
- US1 の T014（帯）と T017・T018（項目）は別ファイルで並行できる
- US2 の T027・T028（`listSnapshot`）は T024〜T026（画面）と並行できる
- US3 の T032・T033（表示設定）は T034（選択欄）と並行できる
- Polish の T042〜T045 は別ファイルで並行できる

---

## Parallel Example: Foundational

```bash
# T005 のあと、次の 4 つは同時に進められる（すべて別ファイル）:
Task: "対比の検査を web/src/theme/tokens.test.ts に書く"
Task: "生の色の禁止を web/src/theme/noRawColors.test.ts に書く"
Task: "骨組みを web/src/components/Skeleton.tsx に作る"
Task: "空・失敗・警告の枠を web/src/components/StateNotice.tsx に作る"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup（テストの実行基盤）
2. Phase 2: Foundational（トークンと回帰の網。**ここが全ストーリーを塞ぐ**）
3. Phase 3: US1（一覧）
4. **止めて確かめる**: [quickstart.md](./quickstart.md) S3 を人が実行する
5. この時点で「フォルダを開く代わりに開く場所」として成立している

### Incremental Delivery

1. Setup + Foundational → 土台（トークン・検査・回帰の網）
2. US1 を足す → S3 で確かめる → ここが MVP
3. US2 を足す → S8 と S5 の往復で確かめる
4. US3 を足す → S6 で確かめる
5. US4 を足す → S4 で確かめる
6. Polish → S1・S2・S7・S9・S10

各段で 002 の受け入れ検証（S1〜S10）は壊れていないこと。壊れたら**その段で直す**
（FR-025 は最後にまとめて確かめるものではない）。

---

## Notes

- [P] のタスク = 別ファイル・依存なし
- `web/src/api/client.ts`・`api/openapi.yaml`・`web/src/api/gen/`・`internal/`・`cmd/` には
  触れない。触れたら FR-024 の防波堤が崩れている（[R-412](./research.md)）
- 生の色・生の px・Tailwind 既定のパレット名を `web/src/**` に書かない。T007 が落とす
- 対比表にトークンを足したら [contracts/design-tokens.md](./contracts/design-tokens.md) 2. の
  表にも足す。表に無い組は検査されない
- タスクごと、または意味のまとまりごとにコミットする
- 各 Checkpoint で止めてストーリー単位に確かめられる
