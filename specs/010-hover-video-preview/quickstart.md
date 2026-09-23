# Quickstart: Hover Preview Validation

## Fixtures

- 9 秒を超え、場面変化が分かる動画。
- 9 秒以下の短い動画。
- browser が原本を直接再生できないが ffmpeg では decode できる動画。
- ffmpeg generation を意図的に失敗させられる破損または unsupported fixture。

## Repository Checks

```powershell
task generate
task check
```

generated file に手編集差分がなく、repository 全体の test/lint が成功することを確認する。

## Generation

1. migration 前から存在する probe 完了動画を含む DB を起動し、再 import なしで preview job が一度だけ backfill されることを確認する。
2. 新規動画を scan し、probe 成功後に preview job が queue されることを確認する。
3. 9 秒超の動画は全尺に分散した 12 x 0.75 秒、短尺は全体一回の出力になっていることを frame/timestamp test で確認する。
4. `ffprobe` で最大幅 640px、H.264、`yuv420p`、audio stream なしを確認し、先頭 byte range だけで再生開始できる fast-start MP4 であること、manifest の byte length/SHA-256 が一致することを確認する。
5. worker を生成途中で終了し、partial file が完成 path に見えず、再起動時に job が queue へ戻ることを確認する。
6. failure を retry 上限まで発生させ、preview が `failed` でも import、thumbnail、一覧取得が成功することを確認する。
7. MP4/manifest の片方を削除する、MP4 を truncate/bit-flip する、manifest の size/digest を変更する各 case で、scan/startup reconciliation が state を `pending` に戻して一度だけ再生成することを確認する。metadata が読める MP4 payload corruption も fixture に含める。
8. 生成中に location/代表場所だけを変更しても、content key が同じなら完成 asset が採用され再生成されないことを確認する。content を変更した場合は stale temporary output が公開済み asset を削除せず、新 asset が生成され旧 asset が orphan cleanup されることを確認する。

## API

1. 一覧/detail response が常に `previewState` を返し、`previewUrl` は done asset にだけあることを確認する。
2. preview endpoint の full request が 200、valid Range が 206 と正しい headers/body、invalid Range が 416 になることを確認する。
3. pending/failed/missing/zero-size file が 404 かつ `no-store` で、stream/transcode への redirect や ffmpeg process 起動がないことを確認する。
4. current content key を持つ URL の immutable cache と、version 不一致の non-immutable behavior を確認する。

## Library UI

1. done preview の grid card へ mouse/trackpad を置き、delay 後に muted preview が同じ thumbnail surface 内で始まることを確認する。
2. browser-incompatible な原本でも生成済み preview が再生でき、pending/failed/missing preview は thumbnail/placeholder のままであることを確認する。
3. pointer leave、別 card、navigation、filter/sort/page/list追加、grid/list 切替、viewport resize、unmount で停止・resource 解放・表示復帰することを確認する。更新後も mounted の card を含め、active preview が同時に1件だけであることを確認する。
4. `play()` rejection と media error で fallback し、次の hover で再試行できることを確認する。
5. touch contact、pen、keyboard focus で開始せず、touch 主体端末へ接続した mouse では開始することを確認する。
6. selection mode、checkbox、Enter/click navigation、focus ring、progress/watched/unplayable indicator が保たれることを確認する。
7. network panel で preview URL だけが取得され、`/stream`、`transcode.mp4`、progress PUT/beacon が発生しないことを確認する。
8. 360px、768px、1280px と reduced-motion で、UI contract の視覚的階層、情報密度、余白、タイポグラフィ、操作優先順位を screenshot と操作記録で確認する。
