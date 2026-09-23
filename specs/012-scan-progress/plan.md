# Implementation Plan: 動画取り込みの進捗表示

**Branch**: `codex/scan-progress-feature` | **Stage branch**: `codex/scan-progress-plan` | **Parent Issue**: [#170](https://github.com/syudead/vv/issues/170)

**Input**: The parent Issue. It is this feature's specification.

## Summary

既存の `ScanProvider` と `GET /api/scans/current` を状態の正本として使い、状態取得の一時失敗から自動回復できるよう polling を補う。route と再読み込みをまたぐ通知状態を `sessionStorage` に保持し、シェル右下のフローティング進捗、ホバー／フォーカス概要、`/settings#scan-status` の詳細を一貫して導く。設定画面には現在または直近の結果、時刻、失敗理由、再試行を表示する。

## Technical Context

**Canonical definitions**:

- システム境界、Web の所有分割、シェルと再生画面の関係: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- API の正本: [api/openapi.yaml](../../api/openapi.yaml) の `Scan` と `/api/scans*`
- 取り込み状態、ポーリング、完了通知の現行実装: [web/src/shell/ScanProvider.tsx](../../web/src/shell/ScanProvider.tsx)
- シェルの配置: [web/src/shell/AppShell.tsx](../../web/src/shell/AppShell.tsx)・[web/src/app/App.tsx](../../web/src/app/App.tsx)
- 設定画面: [web/src/settings/SettingsPage.tsx](../../web/src/settings/SettingsPage.tsx)
- 視覚規則と検証方針: [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)・[docs/how-to/ui-change-screenshots.md](../../docs/how-to/ui-change-screenshots.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）

**Feature-specific context**:

- `Scan` は必要な件数、開始・終了時刻、全体失敗理由を既に返すため、API、生成物、DB schema は変更しない。
- `ScanProvider` は実行中と開始要求の回復中に加え、状態取得に失敗して current scan を確定できない間も既存の2秒間隔で再試行し、成功時に通常の「実行中だけ polling」へ戻す。最後に取得できた scan は一時失敗で捨てない。
- 通知状態は `trackingScanId`、`acknowledgedTerminalScanId`、`completionNotice { scanId, expiresAt }` だけを version 付きの `sessionStorage` に保存する。保存値が無い、壊れている、または storage が利用できない場合は空の通知状態へ戻し、server の scan state と設定詳細は失わない。
- フローティング表示の対象は `AppShell` が包むライブラリ、フォルダ、設定画面である。独立したシアターモードである再生画面へシェルを持ち込まない。
- 新しい runtime dependency は追加しない。位置調整、Portal、Escape、focus の土台には既存の `@radix-ui/react-popover` を使う。

方式選択と代案は [research.md](research.md)、feature 固有の実行確認は [quickstart.md](quickstart.md) に置く。

## Constitution Check

- **Web の所有境界**（ARCHITECTURE.md「Web layer」）: 合格。サーバーとの通信とポーリングは既存 `web/src/api/` と `ScanProvider` に残し、全画面共通表示は `web/src/shell/`、設定の詳細は `web/src/settings/` が所有する。
- **API の正本と生成物**（ARCHITECTURE.md・AGENTS.md）: 合格。既存契約だけで要件を満たすため `api/openapi.yaml` と生成物は変更しない。
- **スクロールの所有**（ARCHITECTURE.md）: 合格。インジケーターは viewport 固定で本文の flow や `window` scrolling を変更せず、設定画面の詳細は既存 document flow に置く。
- **UI の正本**（library-ui.md 1・4・5）: 合格。既存 token と CSS breakpoint を使い、raw color や JavaScript の viewport 判定を増やさない。360px・768px・1280px の構図は実ブラウザで確認し、各 UI 実装 PR に画像を添える。
- **裏側のない入口を増やさない**（library-ui.md 7）: 合格。インジケーターからは同じ変更で追加する設定画面の詳細へ遷移し、操作不能なプレースホルダーを置かない。
- **利用者データと再構築可能データの保護**（ARCHITECTURE.md）: 合格。表示だけを追加し、動画索引、再生位置、原本を変更しない。

Phase 1 後も判定は同じで、正当化の必要な違反はない。

## Structural Decisions

1. **フローティング進捗は `AppShell` が所有し、トップバーや Toast から分離する。** `web/src/shell/ScanProgressIndicator.tsx` を `AppShell` に1度だけ置き、右下の固定表示としてライブラリ、フォルダ、設定画面で共有する。
   - 却下: `TopBar` の更新ボタンを進捗表示と詳細導線に兼用する案。親 Issue が置き換える現行挙動であり、開始操作と監視操作が同じ狭い場所に戻る。
   - 却下: 既存 Toast を拡張する案。Toast は 2.8 秒で消える短い通知で pointer events も受けないため、継続監視、ホバー概要、詳細への移動、失敗の確認待ちを同じ契約にすると役割が衝突する。
2. **状態取得の回復は `ScanProvider`、表示解釈は純粋な共有 mapper に置く。** `ScanProvider` は初回を含む取得失敗後も既存間隔で自動再試行し、`web/src/shell/scanPresentation.ts` は `ScanContextValue` から表示状態、確定／不確定 progress、割合、件数、時刻、説明文を導く。
   - 却下: 2つの component が `running`・`done + failed > 0`・`failed` を個別に分岐する案。一部失敗、総数未確定、読み上げ文言が表示面ごとにずれる。
   - 却下: 初回取得失敗を手動 refresh まで止める案。直接表示または再読み込み直後の一時障害から自動回復できない。
3. **結果通知の寿命は route より上の `ScanNoticeProvider` が所有し、再読み込みに必要な最小状態を `sessionStorage` に保存する。** provider を `ScanProvider` と `Routes` の間に置き、`trackingScanId`、`acknowledgedTerminalScanId`、scan ID と期限を組にした `completionNotice` を version 付きの tab session state として読み書きする。各 `AppShell` のインジケーターはこの安定した状態を描画するだけにする。実行中は current `scan` を追跡して id を保存し、同じ id の terminal scan は再読み込み直後でも結果通知へ移す。確認済みの failed scan と期限切れの完了通知は再表示しない。新しい running scan を観測したら、その id へ追跡状態を切り替え、別 id の `completionNotice` を消す。
   - 却下: `GET /api/scans/current` が返す過去の `done` を起動のたびに通知する案。前回結果を毎回新着の完了として見せ、待機時には進捗枠を置かない要件に反する。
   - 却下: 成否をすべて同じ timer で消す案。全体失敗が利用者の確認前に失われる。
   - 却下: timer と確認済み id を `ScanProgressIndicator` に置く案。route ごとに別の `AppShell` が mount されるため、画面移動で timer と確認状態がリセットされる。
   - 却下: provider のメモリだけに置く案。browser reload で追跡中と確認済みの id、完了期限が失われ、完了の見逃しまたは確認済み失敗の再表示が起きる。
   - 却下: `localStorage` に置く案。別 tab と後日の browser session まで通知の確認状態を共有する必要はなく、古い state を長期間残す。
4. **設定詳細への位置指定は `/settings#scan-status` を公開導線にする。** 設定画面の先頭に `id="scan-status"` の「取り込み状況」を置き、インジケーターは URL でそこへ移動する。直接表示と再読み込みでも同じ位置と server state を復元できる。
   - 却下: navigation state や component ref だけで section を開く案。直接 URL、再読み込み、別画面からの移動で意図した位置を復元できない。
   - 却下: 進捗専用 route や modal を増やす案。親 Issue が設定画面を詳細の置き場と定めており、履歴一覧や個別ログを持たない今回の情報量には別画面が過剰である。
5. **概要の浮動面は既存 Radix Popover を controlled mode で使う。** pointer hover と focus で同じ概要を開き、collision handling と Portal で右端・狭幅のはみ出しを避ける。trigger の click/tap/Enter/Space は popover の toggle ではなく詳細への移動に割り当てる。
   - 却下: CSS の absolute panel だけで実装する案。viewport collision、Portal の stacking、Escape、focus の扱いを独自に重ねる必要があり、既存依存が提供する土台を捨てることになる。

## Project Structure

### Documentation (this feature)

```text
specs/012-scan-progress/
├── plan.md
├── research.md
├── ui-design.md       # 次の design stage で追加
└── quickstart.md
```

新しい entity や field は無いため `data-model.md` は作らない。外部 API の追加・変更が無く、UI の詳細契約は次の design stage が `ui-design.md` に置くため `contracts/` は作らない。この workflow は `tasks.md` を作らず、下の Implementation Work を子 Issue にする。

### Source Code

**Affected boundaries**:

- `web/src/shell/`: scan 取得の回復、表示 mapper、route／再読み込みをまたぐ通知状態、フローティングインジケーター、`AppShell` への配置、トップバーから進捗表示責務を除く変更
- `web/src/settings/`: 「取り込み状況」section、再試行、anchor への focus/scroll
- `web/e2e/`: 画面間の継続、pointer/keyboard/touch 導線、再読み込み、Toast との共存の実ブラウザ検証

**New paths**:

- `web/src/shell/scanPresentation.ts` と対応 test
- `web/src/shell/scanNoticeSession.ts` と対応 test
- `web/src/shell/ScanNoticeProvider.tsx` と対応 test
- `web/src/shell/ScanProgressIndicator.tsx` と対応 test
- `web/src/settings/ScanStatusSection.tsx` と対応 test
- `web/e2e/scan-progress.e2e.ts`

## Implementation Work

### 取り込み状態の自動回復と共有表示モデル

**Scope**: 親 Issue の要件 13・15、Structural Decisions 2・3に従い、`ScanProvider` の初回を含む状態取得失敗後の自動再試行、最後の成功状態の保持、重複開始時に同じ scan へ合流する既存挙動の維持、純粋な共有表示 mapper、route より上で terminal notice の期限と確認状態を持つ `ScanNoticeProvider` を実装する。追跡中 id、確認済み terminal id、scan ID 付きの完了通知期限は例外を外へ出さない helper で `sessionStorage` に同期する。画面 component は変更しない。

**Dependencies**: なし。

**Acceptance**: unit/component tests が、初回取得失敗と実行中の一時失敗から手動操作なしで再取得し、最後の成功状態を保持することを示す。表示 mapper の state matrix が、未実行、開始中、総数未確定、実行中、完了、一部失敗、全体失敗、取得失敗を一意に導く。notice provider の test が、route 相当の子 component の入れ替えと provider の再 mount 後も、保存した tracking id に一致する terminal scan を通知し、確認済み failed scan と期限切れの完了を再表示しないことを示す。さらに、scan A の完了通知後に scan B を追跡すると A の通知期限を消し、reload 後に terminal になった B へ新しい期限を割り当てる。session helper の test が欠損・破損・書き込み例外を空状態へ縮退させる。対象 web tests と `task check` が成功し、画面変更がないため PR に「UI 変更なし」と記載する。

### フローティング進捗と設定画面の取り込み詳細を追加する

**Scope**: 親 Issue の要件 1〜12・14、承認済み `ui-design.md`、Structural Decisions 1・4・5に従い、shell 右下のインジケーター、hover/focus 概要、結果表示、設定詳細への操作を追加し、`TopBar` は待機時の開始操作に専念させる。設定画面ではメディアフォルダより前に「取り込み状況」を置き、共有 mapper から未実行、実行中、完了、一部失敗、全体失敗、取得の一時失敗、時刻、件数、失敗理由、再試行を表示する。`/settings#scan-status` の直接表示と再読み込み、画面移動、既存一覧再読込、再生中の非干渉を browser test で検証する。

**Dependencies**: 取り込み状態の自動回復と共有表示モデル、Design stage 完了。

**Acceptance**: component tests が、未実行では indicator と空の progress を出さず、実行中は概要と設定詳細を同じ値で更新し、完了後は最終件数と完了時刻を残し、全体失敗では確認まで残る indicator、理由、再試行を表示することを示す。mouse hover と keyboard focus で同じ概要が開き、Escape/focus 移動で閉じ、touch/click/Enter/Space で `/settings#scan-status` へ移動する。`scan-progress.e2e.ts` が直接 URL、実行中の画面移動と再読み込み、Toast との同時表示、完了直前の navigation、完了後の一覧更新、重複開始の合流、取り込み中の動画再生を確認する。`task test-e2e` と `task check` が成功する。UI 変更のため、PR に 360px・768px・1280px の両表示の画像と、視覚・pointer・keyboard・支援技術の review 結果を添える。
