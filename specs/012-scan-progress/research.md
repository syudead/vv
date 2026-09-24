# Research: 取り込み進捗表示

技術スタック、依存方向、Web の所有境界は [ARCHITECTURE.md](../../ARCHITECTURE.md)、既存の scan API は [api/openapi.yaml](../../api/openapi.yaml)、視覚規則は [library-ui.md](../../docs/design-docs/library-ui.md) を継承する。ここでは Issue #170 のために追加した判断だけを記録する。

## フローティング表示の所有

**Decision**: インジケーターは `web/src/shell/` が所有し、`ScanNoticeProvider` 内かつ `Routes` の外に1度だけ描画する。ライブラリ、フォルダ、設定、再生画面で同じインスタンスを維持する。`web/src/app/App.tsx` の route 分岐が placement variant を決め、通常画面では右下、再生画面では player control を避けて右上に置く。通知の寿命は route より上の `ScanNoticeProvider` が所有し、再読み込みに必要な最小状態は `sessionStorage` に置く。

**Rationale**: 表示位置は shell の責務だが、現在の route 構成では画面移動ごとに `AppShell` が mount し直され、再生画面には `AppShell` がない。描画と通知寿命を route より上へ置き、tracking id、確認済み terminal id、完了期限を tab session に保存することで、画面移動と reload の直後も同じ取り込みを追跡し、確認済み失敗と期限切れ完了を復活させない。

**Alternatives considered**: トップバーへの集約は開始と監視を再び同じ狭い操作へ混ぜるため不採用。Toast は短時間・非 interactive という既存契約と衝突するため不採用。indicator component や provider のメモリだけに通知状態を置く案は browser reload で失われるため不採用。`localStorage` は別 tab と後日の session まで確認状態を残すため不採用。

## 通知 session state

**Decision**: version 付きの `sessionStorage` record に `trackingScanId`、`acknowledgedTerminalScanId`、`completionNotice { scanId, expiresAt }` だけを保存する。新しい running scan を追跡すると別 id の completion notice を消す。読み書きは total function とし、欠損、schema 不一致、非有限値、storage 例外では空状態へ戻す。

**Rationale**: server の scan が正本なので payload 全体を複製する必要はない。この最小 record があれば、reload 前から追跡した scan の terminal 化、失敗の確認済み判定、scan ごとの完了 timer の残時間を server response と現在時刻から復元できる。deadline を scan id と組にすることで、前の scan の残り時間を次の scan へ適用しない。保存に失敗しても設定詳細は server state から表示できる。

**Alternatives considered**: `Scan` payload 全体の保存は server state と二重の正本を作るため不採用。cookie や DB への保存は tab 内の一時的な表示状態に対して範囲が広すぎるため不採用。保存失敗を画面全体の error にする案は、補助通知の都合で取り込み詳細を失うため不採用。

## 状態取得失敗からの回復

**Decision**: `ScanProvider` は current scan を確定できない取得失敗の間、既存の2秒間隔で自動再試行する。取得済み scan は一時失敗で消さず、成功後は実行中だけを追う通常 polling に戻る。

**Rationale**: 初回表示や再読み込み直後の一時失敗でも、利用者に手動更新を求めず current scan へ合流する必要がある。既存間隔を使えば新しい設定値や backoff policy を増やさず、実行中の回復と同じ頻度で扱える。

**Alternatives considered**: 手動 refresh だけで回復する案は要件 15 に反するため不採用。回数制限付き retry は停止後に利用者操作が必要になるため不採用。別の background poller は同じ endpoint と scan state に複数 owner を作るため不採用。

## 表示状態の導出

**Decision**: `scanPresentation.ts` の純粋関数で未実行、開始中、総数未確定、実行中、完了、一部失敗、全体失敗、取得失敗を導き、インジケーターと設定詳細で共有する。

**Rationale**: API の `state` は `done` の個別失敗を別 state にしないため、`done + failed > 0` の解釈と progress の確定条件を2画面で一致させる必要がある。純粋関数なら全状態を table test で固定できる。

**Alternatives considered**: component ごとの分岐は文言、割合、読み上げがずれるため不採用。API に `partial_failed` を追加する案は表示上の導出だけのために server contract を広げるため不採用。

## 設定詳細への導線

**Decision**: 詳細の安定した入口を `/settings#scan-status` とし、設定画面の先頭 section に対応する id を置く。

**Rationale**: click、tap、keyboard のすべてで同じ URL を使え、直接表示と再読み込みでも目的の section を復元できる。直近1件だけの情報量なら既存設定画面の運用情報として収まる。

**Alternatives considered**: navigation state や ref だけの移動は direct URL と reload で失われるため不採用。専用 route や modal は履歴・個別ログを対象外にした今回の情報量には過剰なため不採用。

## Hover と画面端の処理

**Decision**: 既存の `@radix-ui/react-popover` を controlled mode で使い、hover と focus で同じ非 modal 概要を開く。trigger の activation は設定詳細への移動に使う。

**Rationale**: Portal、collision handling、Escape、focus の土台を既存依存で揃えつつ、hover の無い端末では activation を直接導線として保てる。

**Alternatives considered**: CSS の absolute panel は viewport collision と stacking を独自実装する必要があるため不採用。Tooltip は短い補足向けの既存 styling と interaction で、progress bar と複数件数を読む概要には密度が合わないため不採用。
