# Implementation Plan: 取り込みの進捗と結果を、利用者が知りたいことに答える形に作り直す

**Branch**: `feature/024-import-progress` | **Parent Issue**: #444

**Input**: The parent Issue. It is this feature's specification.

## Summary

今の画面は、走査の割合（`Scan` の total / completed）と、仕事の種類ごとの残り（`Processing`）を
そのまま出している。これを、「直近の取り込みの対象の動画が何本のうち何本済んだか」という1つの
進み具合と、今の処理の1行と、動画ごとの問題の一覧に置き換える。

- **対象と済みの本数**: サーバーが直近の走査に属する動画の集合を持つ。動画は、仕事が積まれた
  時点でその集合に加わる。「済み」は、その動画に残りの仕事が無いことから読み出しのたびに決める
  （[research.md R-1](research.md#r-1-取り込みの対象を走査の記録に紐づく動画の集合として保存する)〜
  [R-5](research.md#r-5-走査中の分母は集合にまだ登録していない対象のファイルの数を足す)）。
- **完了**: 走査が閉じて、対象の動画がすべて済んだときにだけ「完了」または「一部失敗」にする。
  その時刻を1つ保存する（R-4）。
- **問題**: 次の出来事を直近の取り込みの問題として保存し、動画ごとに1件にまとめて返す
  （[R-6](research.md#r-6-問題は出来事ごとの行で保存し読み出しで動画ごとの1件にまとめる)、
  [R-7](research.md#r-7-代用は生成の関数が結果の値として返す)）。
  - 走査で読めなかったファイル、登録できなかったファイル
  - やり直しの上限まで失敗した仕事
  - 代表サムネイルとシーク用サムネイルの代用
- **今の処理**: `internal/app` がメモリに持ち、走査の1ファイルごと、仕事の開始と終了ごとに知らせる
  （[R-8](research.md#r-8-今の処理は保存せずinternalapp-がメモリに持つ)）。
- **API**: `Scan` の形を作り直し、`/api/processing` と SSE の `processing` をなくす。右下の表示と
  設定画面は同じ `Scan` を読み、設定画面だけが問題の一覧を別の経路で読む
  （[contracts/scan-api.md](contracts/scan-api.md)）。
- **画面の形**: 見た目・言葉・配置は、この Plan のあとの design 段階が `ui-design.md` で決める
  （親 Issue に `ui` ラベルがある）。この Plan は、画面が読む値と、その値の意味までを決める。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、ドメインイベント、`/api/events`、作り直せるデータの分類:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)（「Intended dependency direction」
  「Rebuildable and user data」、イベントと SSE の段落）
- 今の走査: [internal/scanner/scanner.go](../../internal/scanner/scanner.go)
  （`Scan`、`progressInterval`、ファイルごとの失敗のログ）、
  [internal/app/scans.go](../../internal/app/scans.go)（`StartScan`・`run`・`RecoverInterrupted`）、
  [internal/store/scans.go](../../internal/store/scans.go)
- 今の仕事と失敗: [internal/domain/job.go](../../internal/domain/job.go)（`MaxJobAttempts`・
  `JobStateAfterFailure`・`Processing`）、[internal/store/jobs.go](../../internal/store/jobs.go)
  （`EnqueueJob`・`EnsureJob`・`FailClaimedJob`・`recordTerminalFailure`・`Processing`）、
  [internal/store/ingest_results.go](../../internal/store/ingest_results.go)
  （結果の書き込み、作り直しの積み直し、`RetryProbe`）、
  [internal/app/ingest.go](../../internal/app/ingest.go)（`JobFinished`、解析のあとのプレビューの積み込み）、
  [internal/jobs/worker.go](../../internal/jobs/worker.go)
- 今の代用: [internal/media/thumbnail.go](../../internal/media/thumbnail.go)（先頭のコマでの再試行）、
  [internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go)（3段の生成）、
  [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)
- 今の HTTP と SSE: [api/openapi.yaml](../../api/openapi.yaml)（`startScan`・`getCurrentScan`・
  `getProcessing`・`streamEvents`、`Scan`・`Processing`・`VideoFolder`）、
  [internal/httpapi/scans.go](../../internal/httpapi/scans.go)、
  [internal/httpapi/events.go](../../internal/httpapi/events.go)、
  [cmd/mdm/events.go](../../cmd/mdm/events.go)
- 今の画面: [web/src/shell/ScanProvider.tsx](../../web/src/shell/ScanProvider.tsx)・
  [scanPresentation.ts](../../web/src/shell/scanPresentation.ts)・
  [ScanProgressIndicator.tsx](../../web/src/shell/ScanProgressIndicator.tsx)・
  [ScanNoticeProvider.tsx](../../web/src/shell/ScanNoticeProvider.tsx)、
  [web/src/settings/ScanStatusSection.tsx](../../web/src/settings/ScanStatusSection.tsx)、
  [web/e2e/scan-progress.e2e.ts](../../web/e2e/scan-progress.e2e.ts)。
  今の UI の判断は [specs/012-scan-progress/ui-design.md](../012-scan-progress/ui-design.md) にある。
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`・
  `task test-e2e`）

**Feature-specific context**:

- 追加する依存は無い。
- SQLite は、表を2つと列を1つ足す（[data-model.md](data-model.md)）。移行は `internal/store/migrations`
  の次の番号を使う。
- `Scan` の形と `/api/processing` の削除は、SPA がバイナリに同梱されて一緒に更新されるので、
  旧 SPA との互換は保たない（これまでの契約変更と同じ扱い）。ただし実装単位の途中で画面が壊れない
  よう、サーバーの単位は新しい項目を足すだけにする。古い項目と `/api/processing` は、画面を
  切り替える単位が消す。
- 英語化の feature（`specs/023-english-i18n`、統合 PR
  [#463](https://github.com/syudead/vv/pull/463)）が、同じ `web/src/shell`・`web/src/settings` の
  文字列と `Scan` の失敗の理由を変えている。
  - 画面の単位は、実装の時点の `main` の方式（023 が入っていれば文字列のカタログ）で言葉を足す。
  - `Scan.errorCode`・`errorPath` は、023 の形を引き継ぐ
    （[R-10](research.md#r-10-画面の言葉はサーバーが返す種類から-spa-が組み立てる)）。
  - 023 が先に `main` に入った場合、画面の単位の前に、feature branch へ `main` を取り込む。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: 状態の決め方（`status`）、問題の種類と重さ、分母と済みの数え方を、純粋関数と
    列挙として持つ。
  - `internal/media`: 代用したかを値として返すだけで、イベントも store も知らない（R-7）。
  - `internal/scanner`: ファイルの失敗と今のファイルを、自分が宣言する報告先に渡す。
  - `internal/jobs`: 仕事の開始を、今の `Finished` と同じ形の `Started` で知らせる。
  - `internal/app`: 今の処理をメモリに持ち、取り込みの状態を組み立てる。
  - `internal/httpapi`: それを `gen` の型に変えるだけである。
  - 兄弟のパッケージを互いに import しない（depguard の規則は変えない）。
- **ドメインイベント**（ARCHITECTURE.md のイベントの段落）: 合格。`domain.ScanActivityChanged` を
  足し、購読の登録は `cmd/mdm/events.go` だけで行う。store は今と同じくコミットのあとにだけ発行する。
- **API の正本**（ARCHITECTURE.md）: 合格。`api/openapi.yaml` を変えて `task generate` し、生成物は
  手で直さない（AGENTS.md）。
- **索引と利用者データの区別**（ARCHITECTURE.md「Rebuildable and user data」）: 合格。
  `scan_videos`・`scan_issues`・`scans.settled_at` は走査と準備のやり直しで作り直せる側に入る。
  利用者データの表には触れない。一覧に載せる変更は、表を足す単位が行う。
- **所有者だけに示す**（要件 11、`internal/httpapi/auth.go` の `accessRoutes`）: 合格。新しい経路は
  所有者だけで、ゲストに返す経路の表に足さない。SPA の購読も今と同じく所有者のときだけ行う。
- **文書は変更と同じ PR で直す**（core-beliefs.md、AGENTS.md）: 合格。走査・仕事・イベントの段落は
  サーバーの各単位が、`specs/012-scan-progress/ui-design.md` が今の UI の正本でなくなることは
  design 段階と画面の単位が直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/024-import-progress/
├── plan.md                # This file
│                          # No spec.md — the parent Issue is the specification
├── research.md            # 対象の集合、完了、分母、問題、代用、今の処理、API、言葉の決定
├── data-model.md          # scans.settled_at、scan_videos、scan_issues
└── contracts/
    └── scan-api.md        # Scan の新しい形、問題の一覧の経路、/api/processing の削除、SSE
```

`quickstart.md` は作らない。検証は各単位の自動テストと、今の `web/e2e` のフィクスチャ
（[web/e2e/media-fixtures.mjs](../../web/e2e/media-fixtures.mjs)）に足すファイルで行う。手で走らせる
feature 固有の手順は無い。`ui-design.md` は、この Plan のあとの design 段階が作る。

### Source Code

**Affected boundaries**:

- `internal/domain`: `status` の決め方、問題の種類と重さ、分母と済みの数え方、
  `ScanActivityChanged`
- `internal/store`: 移行、集合への追加の補助関数と各積み込み箇所、`refreshScanSettled`、問題の記録と
  消去、問題の一覧の読み出し、解析の結果とプレビューの積み込みの1トランザクション化
- `internal/scanner`: ファイルごとの失敗と今のファイルの報告、`progressInterval` の廃止
- `internal/media`・`internal/app`（`Generator`・`Ingest`）: 代用の印の受け渡し
- `internal/jobs`: `Started`
- `internal/app`（`Scans`）: 今の処理の保持と、取り込みの状態の組み立て
- `api/openapi.yaml`・`internal/httpapi`（`scans.go`・`events.go`）・`cmd/mdm`（`events.go`・`main.go`）
- `web/src/api`・`web/src/shell`・`web/src/settings`・`web/e2e`
- `ARCHITECTURE.md`

**New paths**:

- `internal/store/migrations/000NN_scan_import.sql`（`scan_videos`・`scans.settled_at`・`scans.issues_revision`）
- `internal/store/migrations/000NN_scan_issues.sql`

どちらも、実装の時点の次の番号を使う。

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。
取り込みの状態を組み立てる場所は `internal/app` の `Scans` にした。今の処理がメモリにあるので、
store の読み出しだけでは返せないからである。httpapi が store と app の両方を読んで組み立てる案は、
`status` の決め方が HTTP の層に漏れるので採らない。

## Implementation Work

### 取り込みの対象の動画を記録し、済みの本数と完了をサーバーで数える

**Scope**: [data-model.md](data-model.md) の §1・§2 を移行で足す。
- 集合への追加: 仕事を積むすべての箇所と、仕事を着手できるようにするメディアフォルダの追加・
  付け替えで、`scan_videos` に加える。`StartScan` で持ち越しと入れ替えを行う
  （[R-1](research.md#r-1-取り込みの対象を走査の記録に紐づく動画の集合として保存する)、
  [R-3](research.md#r-3-集合は直近の走査の分だけを持ち新しい走査の開始で入れ替える)）。
- プレビューの積み込み: `ApplyProbeForJob` の中で行う（[R-2](research.md#r-2-解析の結果とプレビューの仕事の積み込みを1つのトランザクションにする)）。
- 完了の時刻: 残りの仕事の数が変わりうるすべてのトランザクションで `refreshScanSettled` を呼ぶ
  （[R-4](research.md#r-4-完了は走査が閉じて集合に残りの仕事が無いときにする)）。移行で、直近の走査の未完了の仕事を持ち越す
  （[data-model.md](data-model.md) §1）。
- 状態の決め方: `internal/domain` に `status`、分母、済みの数え方を置く（[R-5](research.md#r-5-走査中の分母は集合にまだ登録していない対象のファイルの数を足す)）。
  失敗の問題の数は、この単位では 0 として渡す。
- 組み立て: `internal/app` の `Scans` が取り込みの状態を組み立てる。
- API: [contracts/scan-api.md](contracts/scan-api.md) §2 の `status`・`videos`・`settledAt` を `Scan`
  に足す。古い項目と `/api/processing` は残す。`/api/events` は `ProcessingChanged` でも `scan` を
  送る（§4）。
- 文書: ARCHITECTURE.md の走査・作り直せるデータ・SSE の記述を直す。

**Dependencies**: None.

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通る。
- `internal/domain` のテスト: `status` の5つの状態と優先順、分母と済みの数え方。
- `internal/store` のテストで、次を確かめる。
  - 10本の新しいファイルを登録すると `videos.total = 10` になる。仕事を順に終えると、
    `settled` が 0 から 10 へ減ることなく増える。
  - 解析を終えてプレビューが積まれるあいだも、`settled` が減らない。
  - 変化の無い動画に仕事が積み直されると、その動画が対象に数えられる。
  - 前回の未完了の動画が、新しい走査へ持ち越される。
  - 見つからないプレビューの積み直しと、解析のやり直しで、直近の走査の対象に加わり、
    `settled_at` が消える。
  - 閉じた走査の残りの仕事がメディアフォルダの削除で着手できなくなると、`settled_at` が入る。
  - 未完了の仕事がある状態から移行すると、それらの動画が対象に入り、`settled_at` が `null` になる。
  - 対象の動画の行が消えると、分母から除かれる。
  - `running` の仕事を積み直して再起動しても、`settled` が二重に数えられない。
- `internal/app` のテスト: 走査が閉じても仕事が残るあいだは `status = running` のままになる。
  最後の仕事の成否を記録した時点で `done` になり、`settledAt` が走査の終了より後になる。
- `GET /api/scans/current` の応答に、`status`・`videos`・`settledAt` がある。

### 取り込みで起きた失敗を問題として記録し、一覧を返す

**Scope**: [data-model.md](data-model.md) の §3 を移行で足す。
- 走査の失敗: `internal/scanner` のファイルごとの失敗（`unreadable`・`changed_during_import`・
  `register_failed`）を、scanner が宣言する報告先を通して `internal/app` が記録する。
- 仕事の失敗: やり直しの上限までの失敗（`*_failed`）を `recordTerminalFailure` と同じ
  トランザクションで記録する。後の成功で消す（[R-6](research.md#r-6-問題は出来事ごとの行で保存し読み出しで動画ごとの1件にまとめる)）。
- 前の走査の問題: `StartScan` で消す。
- 数え方: 問題の件数と、登録できなかったファイルを、分母と済みの本数と `status = partial` に
  反映する。
- 番号: 問題の行を変えるたびに `scans.issues_revision` を増やす。
- API: `Scan.issues`（`revision` を含む）と `GET /api/scans/current/issues` を足す（[contracts/scan-api.md](contracts/scan-api.md) §2・§3）。
- 文書: ARCHITECTURE.md の作り直せるデータの一覧に `scan_issues` を足す。

**Dependencies**: `取り込みの対象の動画を記録し、済みの本数と完了をサーバーで数える`。

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通る。
- `internal/scanner` のテスト: 読めないファイルを含むフォルダを走査すると、そのパスが
  `unreadable` として報告先に渡る。
- `internal/store` のテストで、次を確かめる。
  - 解析が上限まで失敗した動画は `probe_failed` の問題になり、`Scan.status` が `partial` になる。
  - 上限の手前で失敗して後で成功した動画は、問題にならない。
  - 解析のやり直しで成功すると、問題が消える。
  - 新しい走査を始めると、前の問題が消える。
  - 1本の動画に2つの種類が起きると、1件にまとまる。
  - 数千件の問題を、カーソルで重ならずに最後まで辿れる。
  - 問題のある動画に別の種類が加わると、件数は変わらずに `issues.revision` が増える。
- `internal/httpapi` のテスト: `GET /api/scans/current/issues` について、次の応答を確かめる。
  - 並び順と `nextCursor`
  - 不正な `cursor` での 400
  - 一度も走査していないときの 404
  - ゲストでの 401・403

### 代表サムネイルとシーク用サムネイルの代用を問題として記録する

**Scope**: 代用を問題として記録する（[R-7](research.md#r-7-代用は生成の関数が結果の値として返す)）。
- `internal/media`: `Thumbnail` が先頭のコマで作ったかを、`GenerateSeekSprite` が全編から作ったかを
  値として返す。
- `internal/app`: `Generator` と `Ingest` がその値を、結果を書く store の呼び出しに渡す。
- `internal/store`: 成功を書くトランザクションで、`thumbnail_first_frame`・
  `seek_thumbnail_full_decode` を記録する。代用なしで作り直されたら消す
  （[data-model.md](data-model.md) §3）。
- 文書: ARCHITECTURE.md の生成の段落に、代用を知らせることを足す。

**Dependencies**: `取り込みで起きた失敗を問題として記録し、一覧を返す`。

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通る。
- `internal/media` のテスト（ffmpeg があるとき）: 指定位置にコマの無い入力で、代表サムネイルが
  先頭のコマで作られたことを示す値が返る。区間ごとの抽出に失敗する入力で、全編から作ったことを
  示す値が返る。
- `internal/app` と `internal/store` のテスト: 代用だけの取り込みは `status = done` になり、
  `issues.substituted = 1` になる。問題の一覧に、その動画の種類が出る。

### 取り込み中の今の処理を知らせ、進み具合を1ファイルごとに更新する

**Scope**: 今の処理を知らせ、進み具合を1ファイルごとに更新する
（[R-8](research.md#r-8-今の処理は保存せずinternalapp-がメモリに持つ)）。
- `internal/app`: 今の処理を保持する。
- `internal/scanner`: 1ファイルごとに今のファイルと進みを報告し、`progressInterval` をやめる。
- `internal/jobs`: `Started` の hook を足す。
- イベント: `domain.ScanActivityChanged` を足し、`cmd/mdm/events.go` で `/api/events` に購読させる。
- API: `Scan.activity` を足す（[contracts/scan-api.md](contracts/scan-api.md) §2・§4）。
- 文書: ARCHITECTURE.md のイベントと SSE の段落を直す。

**Dependencies**: `取り込みの対象の動画を記録し、済みの本数と完了をサーバーで数える`。

**Acceptance**: 次の検査があり、`task check` と `task check-docs` が通る。
- `internal/app` のテスト、今の処理について:
  - 2つの仕事が重なったとき、後に始まった方を示す。それが終わると、残りの方に戻る。
  - すべて終わると `activity` が省かれる。
- `internal/scanner` のテスト: 5本のファイルの走査で、進みの報告が1ファイルごとに届く。
- `internal/httpapi` のテスト: 今の処理が変わると `scan` の event が送られ、`activity` に
  ファイル名と種類がある。

### 右下の表示と設定画面の概要を、本数による進み具合と今の処理に作り直す

**Scope**: 右下の表示と設定画面の概要を作り直す。
- 仕様: `ui-design.md`（design 段階が作る）に従う。
- 画面の状態: `ScanProvider`・`scanPresentation.ts`・完了の通知（`ScanNoticeProvider`）を、
  `Scan.status`・`videos`・`issues`（本数）・`settledAt`・`activity` だけから作る形にする。
  - 完了の通知は、`status` が `done`・`partial`・`failed` になったときに出す。
  - 一覧の読み直しは `Scan.state` の変化で行う。
- 右下の表示: `ScanProgressIndicator` を作り直す。
- 設定画面: `ScanStatusSection` の概要（状態、進み具合、今の処理、問題の本数、時刻、走査の失敗の
  理由と再試行）を作り直す。
- 古い値の削除: 次をなくし、`task generate` する。
  - `Scan` の古い項目
  - `/api/processing` と SSE の `processing`
  - `ProcessingBreakdown` と、その取得
- テスト: `web/e2e/scan-progress.e2e.ts` と、古い項目を読む e2e の fixture を直す。
- 文書: `specs/012-scan-progress/ui-design.md` が今の正本でなくなる旨を、design 段階の指示に
  従って直す。

**Dependencies**:
- `取り込みで起きた失敗を問題として記録し、一覧を返す`
- `取り込み中の今の処理を知らせ、進み具合を1ファイルごとに更新する`

**Acceptance**: 画面が変わる単位なので、360px・768px・1280px 幅で見た目と操作を確認する。
次の検査があり、`task check`・`task check-docs`・`task test-e2e` が通る。
- web の単体テストで、次を確かめる。
  - `finding` のあいだは割合を出さない。
  - `running` のあいだは「N 本のうち M 本」の1つの進み具合と今の処理を示す。
  - `videos.total = 0` の完了は、変化が無かったことを示す。
  - 仕事の件数や段階ごとの内訳の要素が無い。
  - `role="status"` は完了・一部失敗・失敗だけを読み上げる。
  - 今の処理が変わっても、読み上げと配置が変わらない。
- e2e で、次を確かめる。
  - 10本の取り込みで、進み具合が単位を変えずに増える。
  - 準備が残るあいだ完了と表示されない。
  - 完了の時刻が準備の終わりを示す。
  - 再読み込みのあとも、同じ状態と進み具合が出る。
  - ゲストに右下の表示が出ない。

### 設定画面に取り込みの問題の一覧を出し、動画へ移れるようにする

**Scope**: 設定画面に問題の一覧を出す。
- 仕様: `ui-design.md` に従う。
- 表示: `ScanStatusSection` に `GET /api/scans/current/issues` の一覧を足す。影響と理由の言葉は
  `kinds` から組み立てる（[R-10](research.md#r-10-画面の言葉はサーバーが返す種類から-spa-が組み立てる)）。
- 取得: `web/src/api` に取得関数を足す。`Scan.id`・`issues.revision` が変わったら読み直す
  （[contracts/scan-api.md](contracts/scan-api.md) §4）。
- 移動: 登録された動画の行から `/videos/{id}` へ移れる。
- 長い一覧: 続きを辿れるようにする。

**Dependencies**:
- `右下の表示と設定画面の概要を、本数による進み具合と今の処理に作り直す`
- `代表サムネイルとシーク用サムネイルの代用を問題として記録する`

**Acceptance**: 画面が変わる単位なので、360px・768px・1280px 幅で見た目と操作を確認する。
次の検査があり、`task check`・`task check-docs`・`task test-e2e` が通る。
- web の単体テストで、次を確かめる。
  - 失敗と代用が、色だけでなく文言かアイコンで区別される。
  - 長いファイル名と同じ名前のファイルを見分けられる。
  - 続きを読み込める。
  - 新しい取り込みで一覧が入れ替わる。
  - 各行をキーボードで辿り、動画へ移れる。
- e2e で、次を確かめる。
  - 読めないファイルと解析できない動画を含むフィクスチャを取り込むと、全体が一部失敗になる。
  - 一覧に、その2件の影響と理由が出る。
  - 解析できない動画の行から、再生画面へ移れる。
  - 再読み込みのあとも、同じ一覧が出る。
