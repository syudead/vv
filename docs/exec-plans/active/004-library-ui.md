# 実行計画: 現在の機能を前提とした UI の実装

- ステータス: 進行中（Phase 4 まで完了）
- 最終更新: 2026-09-14
- 対象: [spec](../../../specs/004-library-ui/spec.md) の全体（FR-001〜FR-025 / SC-001〜SC-009）。一覧と再生の 2 画面を、役割名のトークン 1 か所から導いた見た目に作り直し、表示設定・画面幅・キーボードと読み上げまで含めて整える。変更は **`web/` に閉じる**（`api/openapi.yaml`・`internal/`・`cmd/`・`web/src/api/gen/` には触れない）

## 目的

002 で入った機能（一覧・検索・並べ替え・取り込み・再生・視聴状況）はそのままに、
**見た目の規則を 1 か所に集める**。色と形と大きさは `web/src/index.css` の `@theme` を
唯一の真実とし、生の色と Tailwind 既定のパレット名を `web/src/**/*.tsx` から締め出す
（FR-001）。あわせて、対比・表示設定・一覧の復元・視聴状況の読み替え・既存の 3 点を
機械で確かめられる状態にする（SC-006・SC-007・SC-009）。

既存の振る舞いは変えない（FR-025）。`web/src/` のほぼ全域を書き換えるため、これを人の目
だけで保証することはできない。そこで本機能で Vitest + Testing Library を導入し
（[TD-004](../tech-debt.md) が名指しした契機がこの機能である）、書き換えの**前に**回帰の
網を張る。

## 一次資料

詳細はすべて Spec Kit の成果物にある。本書は進捗と決定の記録に徹する。

| 文書 | 内容 |
| --- | --- |
| [spec.md](../../../specs/004-library-ui/spec.md) | 要件と成功基準（FR-001〜FR-025 / SC-001〜SC-009） |
| [plan.md](../../../specs/004-library-ui/plan.md) | 技術的な進め方、ゲート判定、配置 |
| [research.md](../../../specs/004-library-ui/research.md) | 未確定事項の解消（R-401〜R-412） |
| [data-model.md](../../../specs/004-library-ui/data-model.md) | 画面が持つ 2 つの状態と読み替えの規則 |
| [contracts/](../../../specs/004-library-ui/contracts/) | [design-tokens.md](../../../specs/004-library-ui/contracts/design-tokens.md)（トークンと対比表）・[view-preferences.md](../../../specs/004-library-ui/contracts/view-preferences.md)（表示設定）・[screen-states.md](../../../specs/004-library-ui/contracts/screen-states.md)（4 状態・キーボード・読み上げ） |
| [quickstart.md](../../../specs/004-library-ui/quickstart.md) | 受け入れの検証手順（S0〜S10） |
| [tasks.md](../../../specs/004-library-ui/tasks.md) | タスク分解（T001〜T051、Phase 1〜7） |

## 検証の方針

[quickstart.md](../../../specs/004-library-ui/quickstart.md) の S1〜S10 をもって完了を
判定する。**S3〜S8 は人が確かめる**（見た目・画面幅・キーボードと読み上げ・表示設定・
反応時間・「動きを減らす」）。自動化しない。

| シナリオ | 手段 |
| --- | --- |
| S0 | 検証用の動画をつくる（`ffmpeg`。人の準備作業） |
| S1・S2 | 機械。`make test-web` と `make check` |
| S3〜S8 | **人**。ブラウザで `make up` の画面を見て確かめる |
| S9 | 機械 + 人。002 の S1〜S10 が引き続き成功すること（FR-025 / SC-009） |
| S10 | 画面を 4 枚記録し、PR に添える（AGENTS.md の取り決め・G8） |

## 進捗

| 区分 | 状態 |
| --- | --- |
| 仕様 | 完了（2026-09-13） |
| 計画・設計成果物 | 完了（2026-09-13） |
| タスク分解 | 完了（2026-09-13）。T001〜T051 |
| Phase 1: Setup（単体テストの実行基盤） | 完了（2026-09-13）。T001〜T004 |
| Phase 2: Foundational（トークンと回帰の網） | 完了（2026-09-13）。T005〜T013 |
| Phase 3: US1（一覧が「自分のライブラリ」に見える） | 完了（2026-09-14）。T014〜T023。S3 は人の確認待ち |
| Phase 4: US2（再生画面が同じ規則で整う） | 完了（2026-09-14）。T024〜T031。S8・S5 は人の確認待ち |
| Phase 5: US3（表示の仕方を自分で選べる） | 未着手 |
| Phase 6: US4（小さな画面でも破綻しない） | 未着手 |
| Phase 7: Polish | 未着手 |
| 受け入れ検証（S1・S2） | 未着手 |
| 受け入れ検証（S3〜S8・S10） | 未着手（人の実機作業） |

## 決定の記録

### Vitest は 5.x、`defineConfig` は `vitest/config` から取る（T003）

`web/vite.config.ts` の `defineConfig` を `vite` ではなく `vitest/config` から取るように
変えた。Vite 側の `defineConfig` は `test` の項を型として知らず、`make lint-web`
（`tsc --noEmit`）が落ちるためである。実行時の振る舞いは変わらない。

あわせて `web/tsconfig.json` に 2 つ足した。`types` の `vitest/globals`（T003 が
`globals: true` を指定しているので、Phase 2 以降のテストが `describe`／`it`／`expect` を
import せずに書ける）と、`include` の `vitest.setup.ts`（型検査の対象に入れる）である。

### `passWithNoTests: true` を置いた（T003）

tasks.md Phase 1 の Checkpoint が「テストが 0 件でも成功で終わる」ことを求めているため。
テストが揃う Phase 2 以降は外してよい。`web/vite.config.ts` の当該行にその旨を書いてある。

### 増えた依存は開発時の 4 つだけ（T002・plan の G2）

`vitest`・`@testing-library/react`・`@testing-library/user-event`・`jsdom` を
`devDependencies` に足した。`dependencies`（実行時の依存）は 1 つも増えていない。

### 対比表に `surface-sunken` の 2 組を足した（T017）

サムネイル未生成の枠（`bg-surface-sunken`）に理由の文言を置くので、`muted` と `body` を
その地の上に描くことになった。[contracts/design-tokens.md](../../../specs/004-library-ui/contracts/design-tokens.md)
2. の対比表は「表に無い組は検査されない」と定めているため、`muted` / `surface-sunken`
（9.20）と `body` / `surface-sunken`（16.81）を表と `web/src/theme/tokens.test.ts` の
両方に足した。トークンの値は変えていない。

### `takeListSnapshot` は控えを消さない（T027）

名前は「取り出す」だが、読んでも控えは残す実装にした。React は開発時に効果と
`useState` の初期化を 2 回走らせるので、1 回目で消すと 2 回目で復元が消える。
要らなくなった控えは次の `saveListSnapshot` が上書きするか、取り込み完了時の
`clearListSnapshot` が捨てるので、残しても溜まらない（持つのは常に 1 件）。

### 鍵の正規化は「並び順の既定値」も吸収する（T027）

`/` と `/?sort=addedDesc` は利用者から見て同じ一覧なので、鍵も同じにした。
`defaultSort`（`addedDesc`）を `web/src/api/listSnapshot.ts` にも置いてあり、
`web/src/pages/LibraryPage.tsx` の同名の定数と値が揃っていることは
`web/src/api/listSnapshot.test.ts` が確かめている。

### 復元の判定は「鍵ごと」覚える（T029）

`web/src/api/useVideos.ts` に足したのは初期状態の受け渡し口だけである
（[R-412](../../../specs/004-library-ui/research.md) が範囲を限定している）。
「復元で埋めたかどうか」を 1 回で消える印にせず、並び順・検索語・読み直しの世代を
そのまま覚える形にした。効果が何度走っても同じ判断になり、開発時の 2 回実行で
復元が捨てられない。カーソルの引き継ぎ・打ち切り・`reload` は変えていない
（`web/src/api/useVideos.test.ts` が変更前と同じ内容で通っている）。

### `noRawColors.test.ts` の猶予の一覧を畳んだ（T024〜T026）

`pages/VideoPage.tsx` が唯一の猶予だった。書き換えが済んで一覧が空になったので、
「一覧が空になったらこの仕組みごと消してよい」という当該ファイルの取り決めに従い、
`pendingRewrite` と対になる検査を消した。以後は走査対象のすべてが同じ規則で検査される。

### 取り込みの「新しさ」は id で見分ける（T030 の付随）

`GET /api/scans/current` は実行中のものが無ければ**最後に終わったもの**を返す。したがって
`ScanStatus` は「終わっている取り込みがある」としか言えず、最初の巡回で `done` を読んだ
だけで `onFinished` を呼んでいた。一覧を開くたびに起きるので、T030 の復元は毎回その場で
捨てられていた（控えを消し、`reload()` が項目を空にする）。

**役割を分けて直した。** `ScanStatus` は終わっている取り込みを観測するたび、その `Scan` を
そのまま渡す（同じものを何度渡してもよい）。それが新しいかどうかは、いまの一覧がどの
取り込みまでを映しているかを知っている `LibraryPage` が id で判断する。控えにも
`scanId` を持たせるので、再生画面から戻ったときに「見ていない間に別の取り込みが終わって
いた」ことも分かる。

この形にすると、画面をまたいで生きる印を持たずに済む。一覧を開くたびの 1 ページ目の
二重取得も無くなる。判断の材料が `Scan.id` ひとつなので、実行中を一度も観測できなかった
場合（開始要求の応答だけが失われ、次の巡回までに終わっていた場合）にも気付ける。

T016 は `ScanStatus` の振る舞いを変えないと定めているが、
[contracts/screen-states.md](../../../specs/004-library-ui/contracts/screen-states.md) 4.
が求めているのは「終わったら一覧を読み直す」ことであって、開くたびに読み直すことではない。

あわせて「取り込みを始められなかった」ことを状態の取得の失敗とは別に持つようにした ──
同じ入れ物だと、押した直後の巡回が成功しただけで消え、押した利用者には始まったように
見える。文言も「取り込みを始められません: …」に分けた。ただし**消えなさすぎてもいけない**:
要求の失敗は「始まらなかった」ことを保証しないので、そのあと実行中が見えたり別の取り込みが
見えたりしたら言い分のほうが誤りである。失敗した時点で見えていた取り込みの id を添えておき、
それと違うものが見えたら引っ込める。

`web/src/components/ScanStatus.test.tsx`（観測を取りこぼさないこと）と
`web/src/pages/LibraryPage.restore.test.tsx`（新しいかどうかの判断）が固定している。

> **経緯**: ここは PR #27 のレビューで 5 回直している。最初は「実行中を見たか」という印を
> `ScanStatus` に持たせる形にし、画面をまたぐために印をモジュール変数へ移し、印を立てる
> 場所と降ろす場所をそのつど直した。印を増やすほど取りこぼす経路が増えたので、
> 最終的に印を捨てて id の比較に置き換えた。**状態を足して場合分けするより、判断に必要な
> 事実（どの取り込みまで映しているか）を持つほうが小さく収まる。**

### 「一覧へ戻る」は遷移元の URL へ帰る（T024 の付随）

`<Link to="/">` のままでは、`/?q=ねこ&sort=titleAsc` から開いた再生画面から戻ったときに
絞り込みも並び順も消え、控えの鍵（`q` と `sort` でできている）とも一致しないので復元も
効かなかった。項目のリンクに遷移元の URL を `state` で持たせ、再生画面はそれを戻り先に
する。直接開いた場合（`state` が無い）と、`/` で始まらない値は `/` に落とす。

往復そのものは `web/src/pages/LibraryPage.restore.test.tsx` が確かめている。

## 保守者に残る作業

| 作業 | 参照 |
| --- | --- |
| S0（検証用の動画をつくる） | [quickstart.md](../../../specs/004-library-ui/quickstart.md) S0。S3 以降の前提 |
| S3〜S8 の実機確認 | 同 S3〜S8 |
| S10（画面の記録）と PR への添付 | 同 S10、[docs/how-to/ui-change-screenshots.md](../../how-to/ui-change-screenshots.md) |
