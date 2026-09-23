# Data Model: Hover Preview

## Video State

`videos` に `preview_state TEXT NOT NULL DEFAULT 'pending'` を追加し、domain の `Video`、`IndexedVideo`、`UpsertResult` へ伝播する。

| State | Meaning |
| --- | --- |
| `pending` | probe 待ち、生成待ち、retry 待ち、または done file の欠落・破損を repair 待ち。配信可能 asset はない。 |
| `done` | current content key に対応する完成済み preview asset がある。 |
| `failed` | current content に対する preview job が最大試行回数へ達した。import、thumbnail、一覧表示は継続する。 |

DB は生成ファイル path、size、digest を保存しない。path は content key と configured thumbnail root から導出し、integrity metadata は asset と同じ場所の manifest、失敗理由は既存 `jobs.last_error` が所有する。

## Preview Job

既存 `jobs` に kind `preview` を追加する。unfinished job の `(kind, video_id)` uniqueness、`queued|running|done|failed`、attempt、startup requeue、claim 時の content key/location/version/location generation を使う。ただし source location claim の鮮度と生成 asset の有効性は分けて判定する。

- 新規動画: probe 成功後に `preview_state=pending` の current video へ enqueue する。
- 既存動画: migration 後、probe 完了かつ preview 未生成の動画を backfill queue へ入れる。
- probe pending: probe 成功まで enqueue しない。
- probe failed: preview generation は開始しない。

## Asset Identity

完成品と integrity manifest は次の deterministic path に置く。

```text
<thumbnails-root>/preview/<content-key-prefix>/<safe-content-key>.mp4
<thumbnails-root>/preview/<content-key-prefix>/<safe-content-key>.mp4.sha256
```

manifest は MP4 の byte length と SHA-256 digest を versioned structured format で保持する。temporary path は両 file の sibling とし、MP4 の生成と flush/close、digest 計算、manifest の flush/close 後に current video の content key を再確認する。両 file を個別に rename し、両方の publish が成功してから state を `done` にする。途中で失敗した場合は state を `pending` のままにし、retry が片方だけの完成 file を安全に置換する。

location/version/generation が変わっていても content key が同じなら、読み終えた source から作った asset は有効として採用する。content key が変わった、または video が消えた場合は temporary output だけを削除し、同じ content-key path の既存完成品は削除しない。API は DB path ではなく同じ resolver で current content key の MP4 path を得る。

## Transitions

| Trigger | From | To | Effect |
| --- | --- | --- | --- |
| probe success / backfill | absent or `pending` | `pending` | unfinished preview job を一度だけ queue |
| worker claim | `pending` | `pending` | temporary output へ生成 |
| MP4 + manifest publish success | `pending` | `done` | current content asset を可視化し job done |
| retryable failure | `pending` | `pending` | temp cleanup、job requeue |
| final failure | `pending` | `failed` | temp cleanup、preview のみ unavailable |
| content change | any | `pending` | 新 content key の job を queue、旧 asset は orphan |
| location-only change | `done` | `done` | 同じ content key asset を再利用 |
| MP4/manifest missing or mismatch | `done` | `pending` | reconciliation が job を queue |
| video/content orphan | any | removed | 未参照 asset を cleanup |

scan/startup reconciliation は MP4 と manifest が regular file であること、MP4 が non-zero であること、実際の byte length と SHA-256 digest が manifest に一致することを検証する。metadata だけ読める切断 MP4 や byte corruption も mismatch として扱う。検証失敗時は両 file を quarantine/delete し、state を `pending` に戻して unfinished job を一度だけ queue する。

job 完了時の preview-specific apply は video ID と content key を照合する。source location claim のみが stale でも同じ content key なら publish と `done` 更新を許可し、content key が stale なら temporary output を破棄して current content の job/reconciliation に任せる。

## Migration

`00006_hover_preview.sql` は column/check constraint と `preview` job kind を導入する。既存 row は probe 完了なら `pending`、それ以外も state contract 上は `pending` とし、実際の enqueue は probe state を確認する backfill/reconciliation が担う。preview state と asset は再構築可能なので、user data migration は発生しない。
