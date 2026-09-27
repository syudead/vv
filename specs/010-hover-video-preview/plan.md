# Implementation Plan: 一覧の hover 動画プレビュー

**Parent Issue**: [#134](https://github.com/syudead/vv/issues/134)

**UI design**: [ui-design.md](ui-design.md)

## Goal and scope

一覧の動画カードにマウスを留めると、カード内で短い無音プレビューを再生する。
原本のストリームやライブ変換は開始しない。表示と入力の判断は UI design に残す。

## Current structure

probe の後に background job がプレビュー MP4 を生成し、content key に結び付けた
再構築可能な成果物として保存する。配信は生成済み MP4 のみを対象とし、Range に対応する。
HTTP 契約は [OpenAPI](../../api/openapi.yaml) の `/api/videos/{id}/preview` が正本で、
成功応答は `private, no-cache` と ETag による再検証を使う。
サーバー実装は [preview.go](../../internal/httpapi/preview.go)、
生成は [media/preview.go](../../internal/media/preview.go) にある。

## Validation

配信と Range の境界は `internal/httpapi`、生成と失敗時の扱いは `internal/media`、
カードの表示と操作は `web/src/videoList/VideoCard.test.tsx` のテストを参照する。
repository 全体の検査は `task check` を使う。
