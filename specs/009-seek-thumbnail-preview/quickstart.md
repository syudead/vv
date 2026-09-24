# Validation Quickstart: 動画シーク時のサムネイルプレビュー

共通の build、test、起動方法は [Taskfile.yml](../../Taskfile.yml) を使う。本書はこの機能だけの検証matrixを示す。

## Automated validation

1. `task check` で formatter、lint、Go tests、Web build/unit tests、OpenAPI生成差分、migration不変性を
   確認する。
2. 対象browser testで、pointer hover、pointer drag、touch drag、古い応答、画像失敗、page離脱を確認する。
3. API contract testで、先頭、中央、末尾、範囲外、probe未完了、元fileなし、frameなし、process開始失敗、
   request cancellationを確認する。

## Fixture matrix

| Case | Setup | Expected evidence |
| --- | --- | --- |
| 直接配信 | 時刻を画像内に描いたMP4/H.264/AAC | 表示時刻に対応する5秒bucketの画像 |
| ライブ変換 | 同内容のMKV/非対応codec | 直接配信と同じ論理時刻の画像 |
| 先頭・末尾 | 0秒付近と`durationMs - 1` | 対応する5秒bucketのJPEG、画面外へのはみ出しなし |
| rapid move | 10秒間に100回以上位置変更 | 1秒以内に最新位置へ収束し、古い画像へ戻らない |
| duplicate bucket | 同じ5秒内を反復 | URLが同一で、追加のnetwork取得を繰り返さない |
| image failure | 画像応答を409または切断 | 時刻、再生、シークが継続する |
| lifecycle | hover中に一覧へ戻る／動画変更 | request、表示、object URLが残らない |

## Visual and interaction review

テスト用の時刻表示動画を使い、360px、768px、1280pxで変更前の再生画面と変更後を撮影する。各幅で
先頭、中央、末尾を表示し、[親IssueのUI品質](https://github.com/syudead/vv/issues/117)の5観点を判定する。

- 動画が主表示のままで、プレビューはpointer hoverまたはdrag中だけ現れる
- 静止画と時刻以外の情報や操作が増えていない
- 静止画、時刻、シーク位置が近接しつつ、シークバーと既存操作を覆わない
- 時刻が一行で判読でき、先頭・末尾でも切れない
- プレビューがclick、tap、Tabの対象にならず、pointer/touch/keyboardのシークを妨げない

実装PRへviewport、画像、visual reviewの指摘と修正、pointer／touch／keyboard／支援技術の結果、残課題を
記録する。私的な動画やファイル名は使わない。
