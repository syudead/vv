# Seek Thumbnail HTTP Contract

**Feature**: GitHub Issue #117 | **Plan**: [plan.md](../plan.md)

machine-readable schemaの正本は[api/openapi.yaml](../../../api/openapi.yaml)とする。

## Video response delta

動画が`probeState=done`かつ正の`durationMs`を持つ場合、`Video.seekThumbnailUrl`に
`/api/videos/{id}/seek-thumbnail?v=<content version>`を返す。clientは`positionMs`を追加する。

## `GET /api/videos/{id}/seek-thumbnail`

background jobが事前生成した、指定論理時刻を含む5秒bucketのJPEGを返す。このrequestは元動画を開かず、
FFmpegを起動しない。

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `positionMs` | int64 | yes | `0 <= positionMs < durationMs` |
| `v` | string | no | `Video.seekThumbnailUrl`が返したcontent version |

### Success

- Status: `200 OK`
- `Content-Type: image/jpeg`
- `Cache-Control: public, max-age=31536000, immutable`（`v`がある場合）
- `Cache-Control: no-store`（`v`がない場合）
- Body: `floor(positionMs / 5000)`に対応する生成済みJPEG

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| `id`または`positionMs`が不正、時刻が範囲外 | `400` | `invalid_request` |
| 動画がない | `404` | `not_found` |
| probe未完了・尺なし・cache生成中または生成不能 | `409` | `conflict` |
| 予期しないcache I/O失敗 | `500` | `internal` |

Error responseは既存`Error` JSONと`Cache-Control: no-store`を使い、filesystem path、content key、
process command、stderrを返さない。

## Generation lifecycle

- 既存の直列thumbnail workerで動画ごとに一度生成する
- 5秒間隔、幅320px以内、縦横比維持、JPEG quality 4とする
- 一時directoryへ全画像を生成し、成功後にcontent keyのdirectoryへatomicにrenameする
- 中断・失敗時は一時directoryを削除し、不完全な画像群を公開しない
- 既存動画はmigrationで生成jobへ一度だけ再投入し、一覧用thumbnailの状態は維持する

## Client lifecycle

- サムネイル要求は5秒bucketへ丸め、時刻表示は実際のpointer/touch位置へ即時追従する
- bucketが150ms変わらなかった場合だけ画像を要求する
- bucket変更、シークバー離脱、drag終了、動画変更、page離脱で不要な要求を中断する
- 応答時のbucketが最新対象と異なる場合は表示しない
- 同じ版・bucketは同じURLとし、browser cacheから再利用する
- cache生成中または失敗時も時刻、再生、シークを維持する
