# Implementation Plan: 動画取り込みの進捗表示

**Parent Issue**: [#170](https://github.com/syudead/vv/issues/170)

**UI design**: [ui-design.md](ui-design.md)

## Goal and scope

取り込みの進捗、完了、失敗を画面移動中も確認できるようにする。
フローティング表示は概要と設定画面への導線を持ち、設定画面が件数、時刻、理由と再試行を受け持つ。
表示と操作の判断は UI design に残す。

## Current structure

`ScanProvider` は最初に `GET /api/scans/current` で状態を取得し、その後は
`/api/events` の scan イベントで更新する。再接続、ウィンドウ復帰、手動更新時に再取得する。
結果通知の寿命と確認済み scan ID は `ScanNoticeProvider` が tab の `sessionStorage` で扱う。
HTTP 契約は [OpenAPI](../../api/openapi.yaml) が正本である。

## Validation

状態取得とイベント更新は `web/src/shell/ScanProvider.test.tsx`、通知と表示は
`web/src/shell/` の各テスト、画面間の操作は `web/e2e/scan-progress.e2e.ts` を参照する。
repository 全体の検査は `task check` を使う。
