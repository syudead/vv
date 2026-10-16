---
source: docs/design-docs/activity.md
sourceHash: 032729660260c4bb554178e215583b6ef01e88fbc6179c3e635802cb239578cc
---

# 活動ログ {#activity-log}

VVMDM は、動画または組に対して実行する処理と、所有者のタグとバージョンの操作を、実行と対象ごとに 1 つの活動項目として永続的なログに残し、新しい順に所有者へ一覧表示する。項目とその規則は [`internal/domain/activity_entry.go`](../../internal/domain/activity_entry.go)、テーブルと一覧は [`internal/store/activity.go`](../../internal/store/activity.go) で、再試行と取り消しは [`internal/app/activity_log.go`](../../internal/app/activity_log.go) が振り分ける。各規則の理由は [055 research](../../specs/055-activity/research.md) にあり、スキーマは [055 data-model](../../specs/055-activity/data-model.md) だ。活動項目は、取り込みがいま実行している内容を示すメモリ上の行であるスキャンの活動とは別物だ ([R-1](../../specs/055-activity/research.md#r-1-a-persistent-activity-log-named-apart-from-the-scan-activity))。

図は、項目を書く場所と、所有者がそれを読む方法を示す。

```mermaid
flowchart LR
  role[記録するストアの役割] -->|同じトランザクション| entries[(activity_entries)]
  role -->|コミット後| bus[ActivityChanged]
  bus --> sse[activity 通知]
  sse --> screen[所有者の画面]
  screen -->|一覧、再試行、取り消し| api["/api/activity"]
  api --> app[app.Activity]
  app --> entries
```

## 項目はその変更のトランザクションで書く {#an-entry-is-written-in-the-transaction-of-its-change}

記録対象を実行、完了、または決定するストアの役割は、自身のトランザクションの中で 1 つのパッケージ非公開のヘルパーを通して項目を書くか更新し、コミットが `ActivityChanged` を発行する。そのため項目とそれが記録する変更は常に一致し、どの役割も他の役割のメソッドを呼ばない ([R-2](../../specs/055-activity/research.md#r-2-an-entry-is-written-in-the-transaction-of-the-change-it-records))。`ActivityStore` がするのは、一覧、1 つの項目の読み取り、起動時の回復だけだ。

テーブルはユーザーデータだ。決して再構築も間引きもせず、対象を内容の鍵と書いた時点のラベルで示すので、項目はその動画より長く残る ([R-10](../../specs/055-activity/research.md#r-10-the-subject-is-a-content-key-with-a-label-kept-at-write-time))。

## 状態、理由、結果 {#status-reason-and-outcome}

項目は理由を伴う `running` か `attention`、または結果を伴う `done` だ。項目を書くと、同じ種類と対象の古い `attention` 項目を `done`/`superseded` として閉じるので、attention の数はまだ解決していない問題の数になる。`version_judgement` は組ごとに 1 つの項目を持ち、この方法で閉じることは決してない ([R-4](../../specs/055-activity/research.md#r-4-three-statuses-with-a-reason-and-a-newer-run-closes-an-older-problem))。

```mermaid
stateDiagram-v2
  [*] --> running: 待機、その後に処理
  running --> done: 完了
  running --> attention: 失敗、接続なし、未決定
  running --> attention: 起動時に中断
  attention --> done: 新しい項目に置き換えられる
```

`import` 項目は、そのジョブのいずれかが保留中の間は `running` (いずれかが取得されるまでは `waiting`)、段階が諦めたときは `attention`/`failed`、それ以外は `done`/`completed` だ (`ImportActivityState`)。

## 起動は取得済みの項目を中断として記録する {#startup-marks-claimed-entries-interrupted}

ワーカーが開始する前に、理由が `processing` の `running` 項目はすべて `attention`/`interrupted` になる。`waiting` 項目は開始されたことがなく、そのまま残る ([process-lifecycle.md](process-lifecycle.md))。

## 一覧は id でページ分けし、同じ読み取りで数える {#the-list-pages-by-id-and-counts-in-the-same-read}

`GET /api/activity` は不透明なカーソルとともに項目を id の降順で返すので、所有者がスクロールする間に書かれた項目で次のページがずれることはない ([R-9](../../specs/055-activity/research.md#r-9-keyset-paging-by-id-newest-first-filtered-by-status))。ページ、状態ごとの数、機能ごとの数は 1 つの読み取り専用スナップショットから得る。

| 絞り込み | 状態ごとの数 | 機能ごとの数 |
| --- | --- | --- |
| `status` | 無視する | 適用する |
| `feature` | 適用する | 無視する |
| `decider`、`since`、`q` | 適用する | 適用する |

`q` は大文字と小文字を区別せずに対象のラベルに一致する。`undo` 項目は、それが取り消した項目の機能を持つ。契約は [055 screen-api.md](../../specs/055-activity/contracts/screen-api.md) だ。

## 再試行と取り消しは種類ごとに振り分ける {#retry-and-undo-are-dispatched-by-kind}

`app.Activity` は、再試行を提供する項目 (再試行できる種類で、理由が `failed` または `interrupted` の `attention`) に対してだけ再試行に応じ、まだ取り消していない自動の決定に対してだけ取り消しに応じ、その後、その種類に登録されたユースケースへ項目を渡す。何も登録されていない種類は `409 activity_not_retryable` または `409 activity_not_undoable` で応答する。まだどの種類も登録されていない。

## 画面は activity 通知で更新する {#the-screen-refreshes-on-an-activity-notification}

`ActivityChanged` は `/api/events` 上のペイロードのない `activity` イベントになり、所有者のストリームにだけ送られ、`candidates` と同じように接続ごとにまとめられる ([R-8](../../specs/055-activity/research.md#r-8-the-screen-refreshes-on-an-activity-notification))。

| 採用しなかった案 | 理由 |
| --- | --- |
| 通知のペイロードに項目を含める | ほかの通知はすべて状態を読み直させるものだ。ペイロードには独自の順序付けとページ分けが必要になる |
| 項目を書くイベントバスの購読者 | コミット後にクラッシュすると項目が失われ、イベントは決定者も確率も持たない |
