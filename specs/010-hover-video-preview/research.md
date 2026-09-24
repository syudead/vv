# Research: 一覧画面の hover 動画プレビュー

## Inherited Sources

- 境界、API 正本、既存生成処理は [plan.md](plan.md#technical-context) の canonical definitions を継承する。
- 要求と受け入れ条件は [GitHub Issue #134](https://github.com/syudead/vv/issues/134) を正本とする。
- preview は全尺 sampling、短尺処理、低解像度 H.264、無音という性質を採用する。

## Decisions

### 全尺から 12 個の短い区間を抽出する

**Decision**: 9 秒を超える動画では、全尺へ等間隔に配置した 12 区間を各 0.75 秒抽出し、時系列で 9 秒の preview に連結する。9 秒以下は区間分割せず全体を一度だけ変換する。

**Rationale**: hover の短い滞在時間で動画全体の内容を判断でき、冒頭の title card や黒画面だけに偏らない。固定 policy は生成量と test expectation を安定させる。

**Alternatives considered**: 冒頭だけの clip は代表性が低い。ランダム sampling は再生成結果が不安定になる。segment 数・長さの設定 UI は親 Issue の対象外であり、運用と互換性を増やすため採用しない。

### Browser-compatible な小さい無音 MP4 を生成する

**Decision**: 出力は最大幅 640px、縦横比維持、H.264、`yuv420p`、audio track なし、fast-start MP4 とする。

**Rationale**: 一覧カード以上の解像度を避けながら主要 browser で同じ asset を再生でき、音声 autoplay 制約と不要な転送を避けられる。元動画の container/codec に依存しないため、直接再生不能な原本も preview 可能になる。

**Alternatives considered**: 原本 stream は転送量が大きく browser compatibility も引き継ぐ。live transcode は hover ごとに重い処理を起動する。animated image は帯域・色・seek/停止制御で MP4 より不利なため採用しない。

### Preview 専用 job と state を持つ

**Decision**: `preview` job kind と `pending|done|failed` の preview state を追加し、probe 成功後に enqueue する。既存 probe 完了動画は migration/reconciliation で backfill する。

**Rationale**: duration 確定後に生成でき、preview failure を probe/thumbnail/import から分離できる。既存 worker の claim、retry、startup requeue を再利用できる。

**Alternatives considered**: probe 内の同期生成は import latency と failure domain を広げる。thumbnail job への同居は成果物ごとの再試行・repair・観測を曖昧にするため採用しない。

### Content key path へ atomic に公開する

**Decision**: preview path は content key から決定し、MP4 と byte length/SHA-256 manifest を sibling temporary files から公開する。両方が完成するまで state を `done` にしない。publish 前は location claim と content identity を分けて確認し、current content key が同じなら location 変更後も採用する。古い content の両 file は orphan cleanup する。

**Rationale**: location-only change で再生成せず、content change と stale job を安全に区別できる。manifest の照合により、metadata が読める部分破損も検出できる。途中 publish があっても state は `pending` なので endpoint から完成品として見えず、retry で安全に置換できる。

**Alternatives considered**: video ID path は内容変更時の cache invalidation と stale overwrite が難しい。DB への絶対 path/digest 保存は storage root 移動と再構築可能 asset の metadata を DB migration 対象にする。ffprobe の metadata 読み取りだけでは payload corruption を見逃し、全 preview の complete decode は reconciliation cost が高いため採用しない。

### 専用 endpoint で保存済み preview だけを配信する

**Decision**: `GET /api/videos/{id}/preview?v=<content-key>` が保存済み MP4 を Range 対応で返す。未生成、失敗、欠落、明白な不正 file は 404 とし、stream/transcode へ fallback しない。内部破損は scan/startup reconciliation が integrity manifest との size/SHA-256 照合で検出する。

**Rationale**: UI の network behavior と performance が明確になり、versioned immutable cache を使える。生成状態と配信可能性を同じ contract で表せる。

**Alternatives considered**: `/stream` 再利用は原本の大きさと codec を引き継ぐ。`transcode.mp4` fallback は hover が compute を起動し、事前生成の目的を壊す。

### Card-owned media と library coordinator を組み合わせる

**Decision**: timer、video、error fallback、cleanup は grid `VideoCard` が所有し、`LibraryPage` は active card ID と reset epoch だけを調停する。開始は `pointerenter` の `pointerType === "mouse"` に限定する。

**Rationale**: media resource は thumbnail 面に閉じつつ、別 card と、filter/sort/page/list追加や resize 後も mounted のまま残る card を確実に停止できる。イベント自身を判定すれば touch contact を除外し、touch 主体端末へ接続した mouse は許可できる。

**Alternatives considered**: card-local state だけでは mounted card の一覧更新を検知できない。media element 自体を `LibraryPage` に置く案は責務を広げる。`(hover: hover) and (pointer: fine)` は primary input だけを表す。focus/long press 開始は既存 keyboard/touch 操作を妨げるため採用しない。
