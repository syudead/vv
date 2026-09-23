# Quickstart: 取り込み進捗表示の検証

## Prerequisites

[README.md](../../README.md) のローカル開発手順に従って依存を用意する。実ブラウザ検証では `ffmpeg` / `ffprobe` と Playwright の Chromium を利用できる状態にする。

通常の実データ確認には、短い動画を2本以上置いたテスト専用メディアフォルダを用意する。失敗や境界状態は実ファイルを壊さず、component test の mock と `scan-progress.e2e.ts` の `page.route()` で次の応答列を作る。

| Scenario     | `/api/scans/current` の応答列                        |
| ------------ | ---------------------------------------------------- |
| 未実行       | 404                                                  |
| 総数未確定   | `running(total=0, completed=0)`                      |
| 通常進行     | `running(10, 2)` → `running(10, 7)` → `done(10, 10)` |
| 対象0件      | `running(0, 0)` → `done(0, 0)`                       |
| 一部失敗     | `running(10, 5, failed=1)` → `done(10, 9, failed=1)` |
| 全体失敗     | `failed(error=<長い理由>)`                           |
| 一時通信失敗 | request failure → 同じ id の `running` → `done`      |

時刻を確認する response には固定した `startedAt` / `finishedAt` を含め、DOM と screenshot の期待値を安定させる。重複開始は `POST /api/scans` が同じ running scan id を返す応答で検証する。

## Automated Checks

```powershell
task check
task test-e2e
```

実装中に対象の unit/component tests だけを回す場合は、次を使う。

```powershell
npm --prefix web exec -- vitest run src/shell/ScanProvider.test.tsx src/shell/scanPresentation.test.ts src/shell/scanNoticeSession.test.ts src/shell/ScanNoticeProvider.test.tsx src/shell/ScanProgressIndicator.test.tsx src/settings/ScanStatusSection.test.tsx
```

`task check` では API 契約と生成物に差分が無いことも確認する。`task test-e2e` では既存の実サーバー fixture に加え、上表の browser route mock を使う `web/e2e/scan-progress.e2e.ts` を実行する。

## State And Recovery

1. 未実行 response ではフローティング表示が無く、設定画面に「まだ取り込んでいません」と表示され、空または 100% の progress bar が無いことを確認する。
2. 開始要求中と総数未確定 response では不確定 progress と件数確定前の文言を表示し、割合を表示しないことを確認する。
3. 通常進行 response では polling ごとに割合、`completed / total`、失敗件数が同じ scan id のまま更新されることを確認する。
4. 対象0件 response は 100% と誤表示せず完了し、完了件数0と時刻を設定画面に残すことを確認する。
5. 一部失敗と全体失敗を区別し、全体失敗は理由と再試行を表示して、利用者が確認する前にフローティング表示が消えないことを確認する。
6. 初回取得と実行中 polling の一時通信失敗後に、手動更新なしで同じ scan の追跡を再開することを確認する。
7. 状態取得に失敗したまま別画面へ移動しても再試行が続き、回復後に同じ current scan を両表示へ反映することを確認する。
8. running scan の id を追跡した状態で provider を再 mount し、API が同じ id の terminal scan を返すと結果通知が現れることを確認する。
9. 全体失敗の通知を閉じるか設定詳細へ移動したあと provider を再 mount し、同じ failed scan の通知が再表示されないことを確認する。新しい id の failed scan は表示されることも確認する。
10. 完了通知の表示中に provider を再 mount し、保存した期限までの残時間だけ表示されること、期限後は再表示されないことを確認する。
11. `sessionStorage` の値が空、壊れた JSON、version 不一致、不正な id／期限である場合と、読み書きが例外になる場合に、画面が blank にならず server state から設定詳細を表示できることを確認する。
12. scan A の完了通知が残っている間に scan B の running を観測し、A の `completionNotice` が消えることを確認する。その後 provider を再 mount して API が terminal の B を返すと、B の id と新しい期限を持つ通知が表示されることを確認する。

## Interaction

1. `task dev` でアプリを起動し、設定画面でテスト専用メディアフォルダを登録する。トップバーから取り込みを開始すると、トップバーとは別に画面右下へインジケーターが現れることを確認する。
2. mouse/trackpad hover と keyboard focus で同じ概要が開き、pointer を外す、focus を移す、Escape を押す各操作で閉じることを確認する。
3. click、tap、Enter、Space の各操作で `/settings#scan-status` へ移動し、「取り込み状況」が見えて focus の文脈を失わないことを確認する。
4. ライブラリ、フォルダ、設定画面を移動しても同じ scan の表示と完了期限が続き、完了直前または結果表示中の移動でも通知が再開・重複しないことを確認する。
5. 実行中に別の開始操作を行っても indicator が増えず、同じ scan id と進捗へ合流することを確認する。
6. 取り込み中にライブラリの閲覧、検索、filter、フォルダ移動を行い、動画を開いて再生できることを確認する。再生画面にはシェルの indicator を重ねない。
7. 完了後にライブラリとフォルダの内容が読み直され、設定画面には直近結果が残ることを確認する。
8. browser を取り込み中に再読み込みし、current scan の追跡が復元されることを確認する。reload 中に同じ scan が完了した場合は結果通知を表示する。
9. 全体失敗の通知を確認して閉じたあと browser を再読み込みし、同じ failed scan の通知が復活せず、設定詳細だけが直近結果を示すことを確認する。

## Layout And Accessibility

1. 360px、768px、1280px と高さの狭い viewport で、通常表示と hover/focus 概要を撮影する。右端・下端から安全な余白があり、画面外へはみ出さず、主要操作を隠さないことを確認する。
2. 既存 Toast を同時に表示し、狭幅を含めて重ならず、両方の文言と操作を利用できることを確認する。
3. 200% 拡大、長い全体失敗理由、件数の桁が大きい状態で、文字が切れず概要と設定 section が読み取れることを確認する。
4. Tab 順、focus ring、Escape、Enter、Space を確認し、progress bar の accessible name/value と状態変化が screen reader に伝わる一方、2秒ごとの polling を毎回読み上げないことを確認する。
5. 色を見なくても、実行中、完了、一部失敗、全体失敗を文言または icon で判別できることを確認する。
6. `prefers-reduced-motion` で装飾的な animation が止まり、状態と進捗は引き続き判別できることを確認する。

実装 PR には [UI 変更の画像手順](../../docs/how-to/ui-change-screenshots.md) に従って画像と visual review、interaction/accessibility の確認結果を添える。
