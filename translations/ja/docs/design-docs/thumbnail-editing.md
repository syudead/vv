---
source: docs/design-docs/thumbnail-editing.md
sourceHash: fa66ad339a7b568d6330875c90784b8d3a635774b09ef7d887857b2757127554
---

# サムネイル編集のセッション {#thumbnail-editing-sessions}

ComfyUI による動画フレームの編集は、[`app.ThumbnailEdits`](../../internal/app/thumbnail_edits.go) にある動画ごとのメモリ上のセッションで、`/api/thumbnail-edits` のルートが動画ページに提供する ([契約](../../specs/045-comfyui-thumbnails/contracts/screen-api.md))。

図は、セッションがやり取りする相手を示す。

```mermaid
flowchart LR
  page[動画ページ] -->|開始、実行、ポーリング、適用| api[画面 API]
  api --> edits[app.ThumbnailEdits]
  edits -->|フレーム| media[internal/media]
  edits -->|接続のスナップショット| settings[(設定)]
  edits -->|ImageEditor ポート| comfy[internal/comfyui]
  edits -->|適用| ingest[app.Ingest]
```

## セッションはメモリ上にあり、ポーリングで読まれる {#a-session-lives-in-memory-and-is-read-by-polling}

セッションはフレーム、現在の実行、結果の画像をメモリに保持し、実行の進行中はページがそれを毎秒読み、読むたびにプロバイダに実行の状態を問い合わせる ([R-4](../../specs/045-comfyui-thumbnails/research.md#r-4-an-edit-is-an-in-memory-session-per-video-read-by-polling))。実行は開いている 1 つのページに属し、再起動の後に適用されてはならないので、キューのジョブでもドメインイベントでもなく、再起動ですべてのセッションが破棄される。各読み取りはページのハートビートだ。5 分間読まれなかったセッションは、プロバイダでの実行を取り消して終了する。

図は、セッションの状態を示す。

```mermaid
stateDiagram-v2
  [*] --> ready: 編集の開始
  ready --> queued: 実行
  queued --> preparing
  queued --> running
  preparing --> running
  running --> succeeded: 出力を取得した
  queued --> failed: エラーまたは 10 分
  preparing --> failed: エラーまたは 10 分
  running --> failed: エラーまたは 10 分
  queued --> cancelled: cancel
  preparing --> cancelled: cancel
  running --> cancelled: cancel
  succeeded --> queued: 再実行
  failed --> queued: 再実行
  cancelled --> queued: 再実行
  succeeded --> [*]: 適用
```

破棄、5 分間読まれないこと、同じ動画の新しい編集、再起動は、どの状態のセッションも終了させ、進行中の実行はその前にプロバイダで取り消される。送信から 10 分後に終了状態にない実行は、取り消されて `timed_out` として失敗する ([R-11](../../specs/045-comfyui-thumbnails/research.md#r-11-a-new-session-replaces-the-previous-one-a-run-is-limited-to-10-minutes))。30 秒ごとのバックグラウンドの掃除が、誰も読まないセッションに両方の制限を適用し、各リクエストは自身のセッションにそれらを適用する。新しい編集は、新しいセッションが見えるようになる前に、前のセッションをそのセッションのロックの中で終了させるので、前のセッションで進行中の適用または実行が先に終わり、セッションを取り除くのはセッションの終了だけなので、取り除かれたセッションは `404` を返す。

プロバイダが確認しない取り消しでも実行は `cancelled` と示すが、vv はプロバイダのジョブを保持し、セッションが終了した後も、最長 1 時間、掃除のたびに取り消しを再試行する。その取り消しが確認されるまで、同じ動画の新しい実行は送信されず `unreachable` (`502 provider_failed`) として失敗するので、確認されていないジョブが vv にもう取り消せないジョブに置き換えられることはない。1 つのジョブの再試行は一度に 1 つだけ進行し、新しい実行は、その動画のジョブの再試行があればそれが終わるのを待ってから送信される。遅れて届いた Comfy Cloud の中断は、アカウントの実行中のジョブをすべて止めるからだ。異なるジョブの再試行は並行して進むので、確認されていないジョブが多い掃除でも、かかる時間はプロバイダのタイムアウト 1 回分ほどだ。

プロバイダが成功を報告すると、読み取りが出力を取得する。届かなかった取得 (`unreachable` またはタイムアウト) は実行をその状態のままにし、次の読み取りが 10 分の制限の中で再び取得する。出力がないかデコードできないときは実行が `no_output` として失敗するので、`succeeded` には常に適用が読める画像がある。

| 採用しなかった案 | 理由 |
| --- | --- |
| 永続キューのジョブの種類 | 再試行と `failed` の記録がページより長く残り、取り込みの問題に表示される |
| `/api/events` による進捗 | このストリームは確定したドメインイベントをすべての閲覧者に届けるが、実行の状態は 1 つのページのものだ |

## 実行は開始時の接続を使う {#a-run-uses-the-connection-it-started-with}

実行の開始時に保存済みの ComfyUI 接続を読んで実行の中に保持するので、その間に所有者が接続を変更または削除しても、読み取り、取り消し、出力の取得はそのプロバイダに送られる。接続の削除後に開始した実行は `409 thumbnail_edit_not_ready` を返す。セッションの新しい実行は、送信の前に進行中の実行を取り消す。

## 入力はアップロードの前に検査する {#inputs-are-checked-before-anything-is-uploaded}

マスクはフレームのピクセルサイズの PNG、参照画像は 20 MiB 以下の PNG、JPEG、WebP でなければならず、どちらかを満たさないときはアップロードせずに `400` を返す。どちらも最後までデコードするので、途中で切れたファイルや壊れたファイルはここで失敗し、偽造したヘッダーでデコードに際限なく割り当てをさせられないよう、8192 × 8192 ピクセルを超える画像は読めないものとして扱う。WebP は `golang.org/x/image/webp` でデコードし、これは静止画しか読まないので、アニメーション WebP は拒否される。プロバイダの出力も実行が成功する前に同じ検査を通り、WebP の出力を適用するときは ffmpeg のために PNG に変換する。フレームはサーバーが動画のサイズで切り出し、長辺を 2048 px までに抑える ([R-8](../../specs/045-comfyui-thumbnails/research.md#r-8-the-frame-is-extracted-by-the-server-at-up-to-2048-px)) ので、ページはプロバイダが受け取るのと同じピクセルに描く。プロバイダが拒否したアップロードまたは送信は、失敗の理由を付けて `502 provider_failed` を返し、セッションは同じ理由を `failed` として示す。

## 適用は生成のロックを通る {#applying-goes-through-the-generation-lock}

適用は結果を `Ingest.ApplyThumbnailImage` に渡し、これが内容のサムネイル生成のロックの中で結果を収まる大きさに合わせて公開するので、適用、元に戻す操作、位置の上書きのうち最後に確定したものが勝つ ([R-10](../../specs/045-comfyui-thumbnails/research.md#r-10-apply-revert-and-the-position-override-are-ordered-by-the-generation-lock))。セッションは、適用が成功した後と、動画がなくなったか別の内容を持つときの `content_changed` の後に終了し、デコードできない結果ではセッションが残るので、所有者は再実行できる。
