# Seek Thumbnail HTTP Contract

**Feature**: GitHub Issue #117 | **Plan**: [plan.md](../plan.md)

machine-readable schemaの正本は[api/openapi.yaml](../../../api/openapi.yaml)とする。

> この契約の API・画像の形式・間隔・client の取得は、
> [021-seek-thumbnail-sprite](../../021-seek-thumbnail-sprite/plan.md) でスプライトシートに置き換えた。
> 現在の形は [021 の契約](../../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md) を正本とする。以下は 009 で決めたことのうち、今も有効な部分だけを残す。

## Video response delta

動画が`probeState=done`かつ正の`durationMs`を持つ場合、`Video.seekThumbnailUrl`に
content versionを含むURLを返す（形は[021 の契約](../../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)）。

## `GET /api/videos/{id}/seek-thumbnail`

background jobが事前生成した画像だけを返す。このrequestは元動画を開かず、FFmpegを起動しない。
query、response、status、cacheの形は[021 の契約](../../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)に従う。Error responseは既存`Error` JSONを使い、
filesystem path、content key、process command、stderrを返さない。

## Generation lifecycle

- 直列の`seek_thumbnail` worker（[020-seek-thumbnail-stage](../../020-seek-thumbnail-stage/plan.md)）で動画ごとに生成する
- 間隔と画像の形式は[021 の契約](../../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)に従う
- 一時directoryへ全画像を生成し、成功後にcontent keyのdirectoryへatomicにrenameする
- 中断・失敗時は一時directoryを削除し、不完全な画像群を公開しない
- 既存動画はmigrationで生成jobへ一度だけ再投入し、一覧用thumbnailの状態は維持する
- scan完了時にDBのcontent keyと照合し、削除済み動画の確定済みcacheを削除する
- 生成中の一時directoryは掃除対象外とし、workerの後始末に任せる
- workerは確定後にcontent keyの参照を再確認し、scan中に参照が消えていれば即座に削除する

## Client lifecycle

- 表示する画像は[021 の契約](../../021-seek-thumbnail-sprite/contracts/seek-sprite-api.md)の規則で決め、時刻表示は実際のpointer/touch位置へ即時追従する
- 取得と中断の規則は[021 の R-5](../../021-seek-thumbnail-sprite/research.md#r-5-プレイヤーの取得と切り出し)に従う
- 応答時の画像が最新対象と異なる場合は表示しない
- cache生成中または失敗時も時刻、再生、シークを維持する
