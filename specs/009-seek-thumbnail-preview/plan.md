# Implementation Plan: 動画シーク時のサムネイルプレビュー

**Parent Issue**: [#117](https://github.com/syudead/vv/issues/117)

**UI design**: [ui-design.md](ui-design.md)

## Goal and scope

再生画面のシーク位置に対応する静止画と時刻を、ポインターとタッチの操作中に表示する。
画像が取得できない場合も時刻とシーク操作を使えるようにする。表示と入力の判断は UI design に残す。

## Current structure

シーク画像は生成済み JPEG を配信する。生成は代表サムネイルとは独立した
`seek_thumbnail` 段階が担当する。分離した理由と処理順は
[020 の Plan](../020-seek-thumbnail-stage/plan.md) にある。
HTTP 契約は [OpenAPI](../../api/openapi.yaml) の `/api/videos/{id}/seek-thumbnail` が正本で、
成功応答は `private, no-cache` と ETag による再検証を使う。
サーバー実装は [seek_thumbnail.go](../../internal/httpapi/seek_thumbnail.go)、
画像生成は [media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go) にある。

## Validation

時刻と失敗の境界は `internal/httpapi/seek_thumbnail_test.go` と
`internal/media/seek_thumbnail_test.go`、操作と表示は `web/src/player/VideoPlayer.test.tsx` で検証する。
repository 全体の検査は `task check` を使う。
