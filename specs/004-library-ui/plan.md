# Implementation Plan: 現在の機能を前提とした UI の実装

**Branch**: `claude/sdd-004-plan`（機能ディレクトリ: `004-library-ui`） | **Date**: 2026-09-13 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-library-ui/spec.md`

## Summary

002 で揃えた機能（取り込み・一覧・再生・続きから・検索）の上に、**取得できる情報も
保存する情報も1つも増やさずに画面だけを作り込む**。一覧・再生の 2 画面を、1か所に
定義した見た目の規則の上に載せ直し、暗い配色・固定の操作帯・表示密度・小さな画面への
対応・キーボードと読み上げでの到達性を満たす。

技術的な進め方は 3 本である。

1. **規則を CSS の 1 か所に集める**（[R-401](./research.md)）。`web/src/index.css` の
   `@theme` を唯一の真実にし、画面から生の色と生の px を消す。対比（FR-004）はその
   ファイルを読む単体テストで機械的に保証する（[R-405](./research.md)）。
2. **画面の状態と操作の規則を契約にする**
   （[contracts/screen-states.md](./contracts/screen-states.md)）。4 状態・キーボードの
   到達順・読み上げへの代替・44px の当たり判定を表で固定し、実装はそれを満たす。
3. **新しく永続化するのは表示設定 1 個だけ**にする
   （[contracts/view-preferences.md](./contracts/view-preferences.md)）。`localStorage` の
   1 鍵で、壊れていても既定値で画面が出る（FR-019）。

この機能で新しく判断が要ったのは、**戻ったときの一覧の復元**
（[R-403](./research.md) — 絞り込みと並び順は URL、位置と項目はメモリ）、**1000 件規模の
反応**（[R-408](./research.md) — 仮想スクロールを入れず `content-visibility` に任せる）、
**Web の単体テスト基盤**（[R-406](./research.md) — [TD-004](../../docs/exec-plans/tech-debt.md)
が名指しした契機に到達したので Vitest + Testing Library を導入する）の 3 点である。

## Technical Context

**Language/Version**: TypeScript（`web/package.json`、Node 22 LTS）。Go 側の変更は無い

**Primary Dependencies**: React 19 + Vite 8 + Tailwind CSS 4 + `react-router` v7（いずれも
既存）。**新規に増やすのは開発時の依存だけ**で、`vitest`・`@testing-library/react`・
`@testing-library/user-event`・`jsdom` の 4 つ（[R-406](./research.md)）。実行時の依存は
1 つも増やさない

**Storage**: 新しい永続化は `localStorage` の 1 鍵（`vv.view.v1`）だけ。SQLite の表は
変わらず、`api/openapi.yaml` も変わらない（[data-model.md](./data-model.md)）

**Testing**: `vitest run`（`jsdom`）を `make test-web` に追加する。対象は本機能の純粋な
部品 3 つ（対比の検査・表示設定の読み書き・一覧の復元）と、TD-004 が名指しした既存の
3 点（カーソル引き継ぎ・検索の待ち合わせと打ち切り・再生位置の送信）。画面の見た目・
画面幅・読み上げ・反応時間は受け入れ検証（[quickstart.md](./quickstart.md) S3〜S8）で
人が確かめる

**Target Platform**: 現行世代のブラウザ（デスクトップとモバイル）。画面幅 360px〜2560px

**Project Type**: web-service の Web 層のみ（単一 Go バイナリに埋め込まれる React SPA）

**Performance Goals**: 最初の画面 2 秒以内（1万本、SC-002。002 の値を維持する）／1000 件
読み込み後も反応の開始 100ms 以内（SC-008）／サムネイル到着によるレイアウトのずれ 0
（SC-003）

**Constraints**: 実行時の依存を増やさない／`api/openapi.yaml` を変更しない（FR-024）／
002 の受け入れ検証 S1〜S10 が引き続き成功する（FR-025・SC-009）／対比は本文 4.5:1・
境界 3:1（FR-004）／押せる要素は 44px 四方以上（FR-022）

**Scale/Scope**: 画面 2 つ、部品 5〜7 個。変更は `web/src/` に閉じ、TypeScript で
900 行程度（うち新規のテスト 300 行程度）を目安とする

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` は雛形のまま批准されていない。したがってゲートは
001・002 と同じく、実質的な規範として機能している
[`ARCHITECTURE.md`](../../ARCHITECTURE.md)、
[core-beliefs.md](../../docs/design-docs/core-beliefs.md)、
[技術選定文書 2. 選定の判断基準](../../docs/design-docs/tech-stack-selection.md)、
[`AGENTS.md`](../../AGENTS.md) から導出する。

| ゲート | 根拠 | 初期判定 | Phase 1 設計後の再判定 |
| --- | --- | --- | --- |
| G1: 依存方向が一方向で、機械的に強制される | ARCHITECTURE.md | PASS — Go 側に触れない | PASS — 変更は `web/src/` に閉じる。`api/` と `internal/` は変更対象外 |
| G2: 常駐する外部ミドルウェアを増やさない | 判断基準1 | PASS | PASS — 実行時の依存は 0 個増。増えるのは開発時の 4 つだけ（[R-406](./research.md)） |
| G3: DB は再構築可能なインデックスに留める | 判断基準2 | PASS — DB に触れない | PASS — 新しい永続化は端末内の `localStorage` 1 鍵。失われても既定値で成立する（FR-019） |
| G4: 重要な制約は可能な限りテスト可能にする | core-beliefs | 要設計 — 対比・到達性・復元は目視では保証できない | PASS — 対比は CSS を読む単体テスト（[R-405](./research.md)）、復元と表示設定は純粋関数の単体テスト、到達性は Testing Library。残りは quickstart で人が確かめると明示した |
| G5: 文書は変更と同じ変更単位で更新する | AGENTS.md / core-beliefs | 要対応 | PASS — 成果物に `ARCHITECTURE.md`（Web 層の記述）、[TD-004](../../docs/exec-plans/tech-debt.md) の解消、`docs/design-docs/index.md` への追記を含める |
| G6: 生成物は手編集せず、元ファイルから生成する | AGENTS.md | PASS | PASS — `web/src/api/gen/` に触れない。`generate-check` が契約の無変更を機械的に示す（FR-024） |
| G7: 後から重くできる境界を最初に引く | 判断基準4 | 要設計 — 仮想スクロールと明暗の切り替えをどう避けるか | PASS — 密度は CSS の格子だけで実現し、仮想化が要る規模になったら差し替えられる（[R-408](./research.md)）。配色は役割名のトークンなので、明暗の切り替えは値をもう1組足すだけで済む（[R-402](./research.md)） |
| G8: 画面の変更は画像で示す | AGENTS.md | 要対応 — 本機能は画面の変更そのもの | PASS — [quickstart.md](./quickstart.md) S10 で 4 枚を撮り、PR に添えることを手順に組み込んだ |

違反なし。判断の分かれる点は「Complexity Tracking」に記載する。

## Project Structure

### Documentation (this feature)

```text
specs/004-library-ui/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — 未確定事項の解消（R-401〜R-412）
├── data-model.md        # Phase 1 output — 画面が持つ 2 つの状態と読み替えの規則
├── quickstart.md        # Phase 1 output — 受け入れの検証手順（S0〜S10）
├── contracts/           # Phase 1 output
│   ├── design-tokens.md      # 見た目のトークンと対比表（FR-001・FR-004）
│   ├── view-preferences.md   # 表示設定の保存（FR-017〜FR-019）
│   └── screen-states.md      # 4 状態・キーボード・読み上げ・当たり判定
├── checklists/
│   └── requirements.md  # /speckit-specify の品質チェックリスト
```

### Source Code (repository root)

追加・変更する場所だけを示す（`+` が新規、`~` が変更）。

```text
web/
~ package.json               # vitest / @testing-library/* / jsdom（開発時のみ）
~ vite.config.ts             # test の項（環境 jsdom、setupFiles）
+ vitest.setup.ts            # Testing Library の後始末
~ src/index.css              # @theme に見た目のトークン（唯一の真実。R-401）

web/src/
~ App.tsx                    # 変更なしの見込み（経路は 2 つのまま）
├── theme/
│ + tokens.test.ts           # index.css を読み、対比表を検査する（R-405）
│ + noRawColors.test.ts      # *.tsx に生の色・既定パレット名が無いことを検査する
├── preferences/
│ + viewPreferences.ts       # localStorage の読み書き（契約: view-preferences.md）
│ + viewPreferences.test.ts  # 7 つの場合と書き込み失敗
├── pages/
│ ~ LibraryPage.tsx          # 固定の帯・密度・4 状態・復元（FR-002/007/008/009/017）
│ ~ VideoPage.tsx            # 並びの固定・情報欄の言い分け（FR-012〜FR-016）
├── components/
│ ~ VideoCard.tsx            # トークン化・題名の全文への到達手段（R-409）
│ ~ ScanStatus.tsx           # 帯の中へ移す。進捗の見せ方（FR-010）
│ + Toolbar.tsx              # 固定の帯（R-404）
│ + DensitySelect.tsx        # 密度の選択（FR-017）
│ + Skeleton.tsx             # 通信中の骨組み（FR-002）
│ + StateNotice.tsx          # 空・失敗・警告の共通の枠（FR-002）
│ + *.test.tsx               # 到達順・狙いを合わせた印・読み上げ向けの代替
└── api/
  ~ useVideos.ts             # 復元状態の受け渡し口のみ（R-412 が範囲を限定する）
  + listSnapshot.ts          # 一覧の復元状態（メモリ。data-model.md 2.）
  + listSnapshot.test.ts
  + useVideos.test.ts        # TD-004 の 1 点目（カーソル引き継ぎ）
  ~ client.ts                # 変更しない（FR-025 の防波堤）

~ Makefile                   # test-web に vitest run を足す
~ ARCHITECTURE.md            # Web 層の記述（見た目の規則の置き場、テスト基盤）
~ docs/exec-plans/tech-debt.md   # TD-004 を解消済みにする
~ docs/screenshots/          # 新しい 4 枚（quickstart S10）
```

**Structure Decision**: 変更は `web/` に閉じる。Go 側・`api/openapi.yaml`・生成物には
一切触れない。これは方針であると同時に、**FR-024・FR-025 に対する構造上の防波堤**である
（[R-412](./research.md)）。`web/src/` には `theme/` と `preferences/` の 2 つを足す。
どちらも画面に依存しない純粋な部品で、単体テストの最初の対象になる。

`web/src/api/` は「サーバーとのやり取り」の層として既存のまま保ち、本機能が触るのは
`useVideos.ts` の復元の受け渡し口と、新設の `listSnapshot.ts` だけである。`client.ts` を
変更対象から外すことで、契約の解釈が変わっていないことを差分から読み取れるようにする。

## Phase 2 以降へ送る判断

| 論点 | 送り先 | 理由 |
| --- | --- | --- |
| 明暗の切り替え | 別機能 | spec の Assumptions が本仕様では扱わないと決めている。役割名のトークンにしてあるので値を1組足すだけで済む（[R-402](./research.md)） |
| 左の一覧（ライブラリ・コレクション・タグ） | 整理機能と同時 | 区分が「すべての動画」1 つしか無い（spec の Assumptions） |
| タグ・お気に入り・コレクション・未整理と、複数選択 | Phase 2 | spec でスコープ外。裏側が無い入口は「壊れている」ように見える |
| 仮想スクロール | 規模が要求したとき | `content-visibility` で SC-008 を満たせる見込み（[R-408](./research.md)）。満たせなければ TD として記録する |
| 字幕・シークのプレビュー画像・変換による再生 | Phase 2 以降 | 裏側が未実装。技術選定文書 8 のフェーズ分けどおり |
| 再生の E2E（Playwright） | Phase 3 | 技術選定文書のまま。本機能が入れるのは部品単位の検証基盤である（[R-406](./research.md)） |
| 認証・利用者ごとの表示設定 | Phase 3 | 表示設定を端末内に閉じたのはこのため（[R-407](./research.md)） |

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 開発時の依存が 4 つ増える（`vitest`・`@testing-library/react`・`@testing-library/user-event`・`jsdom`） | 本機能は `web/src/` のほぼ全域を書き換えるのに、FR-025／SC-009 は「既存の振る舞いが変わらないこと」を求める。全域を書き換えながら人の目だけで保証はできない。[TD-004](../../docs/exec-plans/tech-debt.md) が「次の機能の着手時に導入する」と契機を明示しており、本機能がそれに当たる | 入れない案は、FR-003・FR-020・FR-021（狙いを合わせた印・キーボードでの到達・読み上げ向けの代替）の退行を検出する手段が無くなる。Playwright を先に入れる案は Phase 3 の範囲で、目的（再生の E2E）も起動する対象も違う |
| 一覧の状態が URL とメモリの 2 か所に分かれる（[R-403](./research.md)） | 絞り込みと並び順は共有・再読み込みで再現したい（URL が適任）。スクロール位置と読み込み済みの項目は URL で表せず、URL に載せると戻るたびに数ページ読み直して SC-008 を損なう | 片方に寄せる案は、URL だけにすると再取得が増え、メモリだけにすると再読み込みで絞り込みが失われて FR-016 の意図に届かない |
| 密度の最小列幅に `min(..., (100% - gap) / 2)` を挟み、狭い画面では 3 つの密度が同じ見た目になる | 幅 360px で 1 列になると、静止画が巨大になり一覧として機能しない。FR-022（横スクロール無し）と両立させる必要がある | 密度ごとに画面幅の断りを書く案は、断りが密度 × 幅の数だけ増え、FR-001（規則は1か所）から遠ざかる |
| 題名の全文への到達手段が 3 つある（アクセシブル名・`title`・狙いを合わせたときの展開。[R-409](./research.md)） | 入力手段ごとに届く手段が違う。`title` はキーボードと読み上げに届かない | 1 つに絞る案は、FR-011・FR-020・FR-021 のいずれかを満たせない |
