# Seek Thumbnail HTTP Contract

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md)

machine-readable schema の正本は [api/openapi.yaml](../../../api/openapi.yaml) とする。本書は任意時刻の
画像応答、cache、request lifecycleを定める。既存の一覧用 `/thumbnail`、動画 `/stream`、
`/transcode.mp4` は変更しない。

## Video response delta

動画が `probeState=done` かつ正の `durationMs` を持つ場合、一覧と詳細の `Video` に
`seekThumbnailUrl` を含める。値は `/api/videos/{id}/seek-thumbnail?v=<content version>` 形式の
基底 URL で、client は `positionMs` を追加する。条件を満たさない動画では省略する。

`v` は動画内容由来の版であり、同じ内容では移動・改名後も変わらず、内容が変われば URL も変わる。

## `GET /api/videos/{id}/seek-thumbnail`

元動画の指定論理時刻に対応する JPEG を返す。

### Query

| Name | Type | Required | Rule |
| --- | --- | --- | --- |
| `positionMs` | int64 | yes | `0 <= positionMs < durationMs` |
| `v` | string | no | `Video.seekThumbnailUrl` が返した content version |

client は最寄りの1秒へ丸めた `positionMs` を送り、末尾を越える場合は `durationMs - 1` へ収める。
server はその位置以後で最初の有効な映像フレームを選び、選択時刻と要求時刻の差を1秒以内に収める。
要求位置以後にフレームがない末尾付近では、最大1秒手前から再度デコードして最後に得られるフレームを
返す。前後のframeを比較する厳密な最近傍選択は要求せず、コンテナの終端時刻を映像フレームの開始時刻
として扱わない。

### Success

- Status: `200 OK`
- `Content-Type: image/jpeg`
- `Cache-Control: public, max-age=31536000, immutable`（空でない `v` がある場合）
- `Cache-Control: no-store`（`v` が無い場合）
- Body: 指定位置との差が1秒以内で、上記の選択規則に対応する映像を表す1枚の JPEG
- Range response と `Content-Disposition` は提供しない

画像は response のためだけに生成し、data directoryまたはmedia folderへ保存しない。response開始前に
画像全体の生成を完了し、途中までのJPEGを成功応答にしない。

### Errors

| Condition | Status | Error code |
| --- | --- | --- |
| `id`または`positionMs`が不正、時刻が範囲外 | `400` | `invalid_request` |
| 動画、current location、実体がない／安全に開けない | `404` | `not_found` |
| probe未完了・失敗、尺なし、映像frameを取得不能 | `409` | `conflict` |
| 抽出processを開始できない、予期しないI/O失敗 | `500` | `internal` |

Error response は既存 `Error` JSON と `Cache-Control: no-store` を使い、filesystem path、content key、
process command、stderrを返さない。

## Extraction lifecycle

- current location は既存のmedia folder境界、symlink、regular-file検査を通ったものだけを使う
- 抽出は request context と10秒のうち先に終了した方で中断する
- 指定位置から画像を得られない場合は、`max(0, positionMs - 1000)` から終端までをデコードし、
  その区間で最後のframeを選ぶ。差が1秒を超える場合は画像取得不能とする
- request中断後5秒以内にprocessとpipeを終了し、別requestの抽出または動画再生へ影響させない
- JPEG は幅320px以内、縦横比を維持し、拡大しない
- 元動画、SQLite、既存thumbnail、playback progressを変更しない

## Client request lifecycle

- 時刻はpointer/touchの位置から元動画全体の論理時刻として求め、最寄りの1秒へ丸める
- 対象 bucket が150ms変わらなかった場合だけ画像を要求する。時刻表示は待たずに更新する
- bucket変更、シークバー離脱、drag終了、動画変更、page離脱で不要な要求を中断する
- 応答時の bucket が最新対象と異なる場合、その画像を表示しない
- 同じ版・bucketは同じ URL とし、browser cacheから再利用できる
- 画像失敗時は静止画を隠して時刻を残し、再生とシークを継続する
