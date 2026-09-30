# Implementation Plan: 動画の表示名と代表サムネイルを変えられるようにする

**Branch**: `feature/029-video-overrides` | **Parent Issue**: #517

**Input**: The parent Issue. It is this feature's specification.

## Summary

所有者が、画面（動画ページ）からでも外部連携 API と MCP からでも、動画ごとに表示名と代表サムネイルの
場面を上書きできるようにする。元のファイルには触れない。

- **保存**: 上書きは `content_key` に結ぶ利用者データの表 `video_overrides` の 1 行で持ち、再スキャン・
  移動・改名・所在の追加を越えて残る（[research.md R-1](research.md#r-1-上書きは-content_key-に結ぶ-1-つの利用者データの表に置く)、
  [data-model.md §1](data-model.md#1-video_overrides)）。
- **題名**: 有効な題名（表示名があればそれ、無ければファイル名由来）は保存層が決めて `Video.Title` に
  載せ、`FileTitle`・`DisplayName` を別に運ぶ。並び替えと検索は所在ごとの `title_key`・`search_key`
  に表示名を織り込む（[R-2](research.md#r-2-有効な題名は保存層が決めdomainvideo-が表示名とファイル名由来の題名を両方持つ)、
  [R-3](research.md#r-3-並び替えと検索は所在ごとの-title_keysearch_key-に表示名を織り込む)）。
- **サムネイル**: 位置の指定と解除は要求の中で画像を作ってから記録し、失敗したら前の画像と位置を残す。
  取り込みの `thumbnail` job も指定の位置で作る。置き場の並べ方は変えず、`thumbnailUrl` の版に位置の記録ごとの
  改版番号を含める（[R-4](research.md#r-4-サムネイルの位置の指定は要求の中で生成してから記録する)、
  [R-5](research.md#r-5-取り込みのサムネイル-job-も指定の位置で作り自動の規則は-internalmedia-に残す)、
  [R-6](research.md#r-6-thumbnailurl-の版に指定の改版番号を含める)）。
- **通知**: 上書きの変化は `domain.VideoOverrideChanged` で `/api/events` の `video` に写す
  （[R-7](research.md#r-7-上書きの変化は-domainvideooverridechanged-を発行し画面の-video-通知に写す)）。
- **API**: 画面は動画ごとの 2 つの `PUT`（[contracts/screen-api.md](contracts/screen-api.md)）、
  外部連携 API は一括の 2 操作と `ExternalVideo` の 3 項目、MCP は同じ 2 ツール
  （[contracts/external-api.md](contracts/external-api.md)、
  [R-8](research.md#r-8-画面の-api-は動画ごとの-2-つの-put-にし空の表示名と-null-の位置が解除である)、
  [R-9](research.md#r-9-外部連携-api-は一括の-2-操作を足しmcp-は同じ-2-ツールを足す)）。
- **規則**: 表示名はタグ名と同じ整え方で上限 200 符号位置、空は解除。位置の検証と誤りの分け方は
  `internal/domain` の純粋関数が持つ（[R-10](research.md#r-10-表示名の規則はタグ名の規則にそろえ上限は-200-符号位置にする)、
  [R-11](research.md#r-11-位置の検証は-internaldomain-の純粋関数が持ち誤りは-3-つに分ける)）。
- **画面**: 動画ページの題名の編集、元のファイル名の表示、プレイヤーの「今の場面をサムネイルにする」。
  `ui` ラベルがあるので、見た目と操作は次の design 段階の `ui-design.md` が親 Issue の「UI品質」を
  基準に決める。一覧のカードは `title` を出すだけなので変えない。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・索引と利用者データの区分・生成物の置き場・ドメインイベント・認証の境界:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml)（depguard）
- 動画の読み出しと題名の出どころ: [internal/store/videos.go](../../internal/store/videos.go)
  （`videoColumnsTemplate`）、[internal/store/listing.go](../../internal/store/listing.go)（`listColumns`）、
  [internal/store/library_items.go](../../internal/store/library_items.go)、
  [internal/store/external_videos.go](../../internal/store/external_videos.go)
- 検索と並び替えの鍵: [specs/013-library-search/data-model.md](../013-library-search/data-model.md)、
  [internal/store/search_keys.go](../../internal/store/search_keys.go)、
  [internal/domain/search.go](../../internal/domain/search.go)（`NaturalSortKey`・`FoldForMatch`）
- 内容鍵に結ぶ利用者データの先例: [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
  [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)、
  [internal/store/visibility.go](../../internal/store/visibility.go)
- サムネイルの生成と公開: [internal/media/thumbnail.go](../../internal/media/thumbnail.go)、
  [internal/app/ingest.go](../../internal/app/ingest.go)（`Thumbnail`）、
  [internal/app/artifacts.go](../../internal/app/artifacts.go)（生成の錠）、
  [internal/artifacts/store.go](../../internal/artifacts/store.go)（`PublishThumbnail`）、
  [internal/store/ingest_results.go](../../internal/store/ingest_results.go)（`setStageStateForJob`）、
  [specs/024-import-progress/research.md](../024-import-progress/research.md) R-7（代用の記録）
- 所在を開いてよいかの規則: [internal/httpapi/stream.go](../../internal/httpapi/stream.go)
  （`openMediaFile`）、[internal/mediafs](../../internal/mediafs/media_file.go)
- 画面の API と誤りの形: [api/openapi.yaml](../../api/openapi.yaml)、
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)、
  [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)
- 外部連携 API と MCP: [api/external-v1.yaml](../../api/external-v1.yaml)、
  [specs/026-external-api/contracts/external-api.md](../026-external-api/contracts/external-api.md)、
  [specs/026-external-api/contracts/mcp.md](../026-external-api/contracts/mcp.md)、
  [internal/httpapi/mcp.go](../../internal/httpapi/mcp.go)、
  [docs/how-to/external-api.md](../../docs/how-to/external-api.md)
- 画面: [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md)、
  [web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx)、
  [web/src/player/VideoFacts.tsx](../../web/src/player/VideoFacts.tsx)、
  [web/src/api/useVideoDetail.ts](../../web/src/api/useVideoDetail.ts)、
  [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)、
  [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）

**Feature-specific context**:

- 移行は 1 つ（`video_overrides`、`00021`）。`videos`・`video_locations` に列は足さない。
- Go・npm とも依存は足さない。`ffmpeg` の使い方は今の 1 コマ抽出と同じ引数で、位置だけ指定になる。
- `SearchKeyVersion` は上げない。表示名の無い所在の鍵は今と同じ値である（data-model.md §4）。
- `quickstart.md` は作らない。受け入れ条件は store・app・httpapi・Vitest の試験で確かめられ、
  リポジトリの検査の外で実行する手順はない。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: 表示名の整え方、位置の検証、誤りの値、`Video` の項目、`VideoOverrideChanged`。
    純粋な値と関数だけ。
  - `internal/store`: 移行、`OverrideStore`、読み出しの左結合、鍵の書き直し、確定後の発行。
  - `internal/media`: `ThumbnailAt`（ffmpeg を呼ぶだけ）。
  - `internal/app`: `Ingest.SetThumbnailPosition`（生成の錠の中で生成→記録）と job の位置の読み替え。
    生成・保存・生成物は宣言した interface 越し。
  - `internal/httpapi`: 要求の解釈、所在の解決（`openMediaFile` と同じ規則）、応答への変換、外部
    連携 API と MCP。規則は domain・store・app に置く。
  - `cmd/mdm`: 配線と `VideoOverrideChanged` の購読の登録。
- **役割の型は他の役割の公開メソッドを呼ばない**（ARCHITECTURE.md `store.DB` の段落）: 合格。内容鍵の
  引き直し（`registeredContentKeysForVideoIDs`）、鍵の書き直し（`writeSearchKeys`）、代用の行
  （`applySubstitution` の表）はパッケージ内の関数として共有する。
- **索引と利用者データの区別**: 合格。`video_overrides` は利用者データの一覧に足し、生成物の片付けは
  それに触れない（data-model.md §1）。
- **生成物の置き場は変えない**（ARCHITECTURE.md「Generated files have one owner」）: 合格。指定の画像も
  `<p>/<s>.jpg`（R-5）。
- **ドメインイベントは確定後に発行し、購読の登録は `cmd/mdm/events.go` の 1 か所**: 合格（R-7）。
- **API の正本と生成物**（AGENTS.md）: 合格。`api/openapi.yaml`・`api/external-v1.yaml` を直して
  `task generate`。外部連携 API は項目と操作の追加だけ（互換の方針）。
- **ゲストは所有者のデータを見ない**（guest-api.md）: 合格。`fileTitle`・`displayName`・
  `thumbnailPositionMs` は所有者の応答にだけ入り、書き込みは境界の既定の分類で `401`（要件 9）。
- **サーバーの出力は英語、画面の文言はカタログ**（`.golangci.yml` の gosmopolitan、i18n.md）: 合格。
- **文書は変更と同じ PR で直す**（core-beliefs.md）: 合格。各単位が ARCHITECTURE.md・
  `docs/how-to/external-api.md`・`api/*.yaml` の説明の自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/029-video-overrides/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-11
├── data-model.md         # video_overrides、domain の値、保存層の操作、鍵の規則
└── contracts/
    ├── screen-api.md     # 画面の API の 2 つの PUT と Video の差分
    └── external-api.md   # /api/v1 の 2 操作、ExternalVideo の差分、MCP の 2 ツール
```

`ui-design.md` は次の design 段階が作る（`ui` ラベル）。`quickstart.md` は作らない（Technical Context）。

### Source Code

**Affected boundaries**:

- `internal/domain`（表示名・位置の規則、`Video` の項目、イベント）、`internal/store`（移行、
  `OverrideStore`、読み出し、鍵）、`internal/media`（`ThumbnailAt`）、`internal/app`（`Ingest`）
- `internal/httpapi`（画面の 2 経路、`toAPIVideo`、外部連携の 2 操作、`toExternalVideo`、MCP）、
  `api/openapi.yaml`・`api/external-v1.yaml` と生成物、`cmd/mdm`（配線・購読）
- `web/src/api`（`setVideoDisplayName`・`setVideoThumbnailPosition`）、`web/src/player`、`web/src/i18n`
- `ARCHITECTURE.md`、`docs/how-to/external-api.md`

**New paths**:

- `internal/store/migrations/00021_video_overrides.sql`、`internal/store/overrides.go`、
  `internal/domain/video_override.go`
- `internal/httpapi/video_overrides.go`、`internal/httpapi/external_video_overrides.go`
- `web/src/player/` の編集の部品（名前は design 段階に従う）

**Structure decision**: 位置の設定の使い方は `app.Ingest` に置く。生成物の作成と削除を直列にする錠を
持つのが `Ingest` で、要求の中の生成も job の生成もその錠を通る必要があるため（R-4）。表示名の保存は
生成を伴わないので、`VisibilityStore` と同じく保存層の役割だけで完結させ、`internal/httpapi` が直接
呼ぶ。

## Implementation Work

### 表示名を保存し、題名・並び替え・検索に反映する

**Scope**: `00021_video_overrides.sql`、`domain.NormalizeDisplayName` と誤りの値、`Video.FileTitle`・
`DisplayName`・`ThumbnailPositionMs`、`VideoOverrideChanged`（[data-model.md §1・§2](data-model.md#1-video_overrides)）。
`OverrideStore.SetDisplayName`・`SetDisplayNames` と、動画を返す全読み出し（詳細・一覧・ライブラリ項目・
関連・外部連携・`IngestStore.GetVideo`）の左結合、鍵の書き直しの 4 経路（[§3・§4](data-model.md#3-保存層の操作)）。
`cmd/mdm/events.go` の画面の購読。ARCHITECTURE.md の利用者データの一覧と `store.DB` の役割の一覧。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、表示名を付けた動画の `Title` が表示名、`FileTitle` が
所在の題名になり、`titleAsc` で表示名の位置に並び、表示名でもファイル名でも検索に出る（受け入れ条件 4）。
所在を別のパスへ upsert し直しても（改名・移動の再スキャン）表示名と鍵が残る（受け入れ条件 3）。空・
空白だけの名前で行が消え、制御文字と 201 符号位置は `*InvalidDisplayNameError` で何も書かない。同じ
内容の 2 つ目の所在からも同じ `Title` が返る。表示名を付けた動画のあるメディアフォルダを
`AddMediaFolder`・`ReplaceMediaFolder` で登録し直しても、`title_key` と `search_key` が表示名を含む。確定後に `VideoOverrideChanged` が 1 回発行される。

### 画面の API で表示名を設定・解除する

**Scope**: `PUT /api/videos/{id}/display-name`、`Video` の 3 項目とゲストでの省略、`ErrorReason` の 2 値、
`api/openapi.yaml` の `title` の説明と生成物、`web/src/api/client.ts` の関数と `web/src/i18n/errors.ts`
の文言（[contracts/screen-api.md §0・§1・§3](contracts/screen-api.md#0-video-の差分)）。`accessRoutes`
の試験（所有者だけ）。

**Dependencies**: 表示名を保存し、題名・並び替え・検索に反映する

**Acceptance**: `task check` が通る。httpapi の試験で、所有者の `PUT` が `200` と反映済みの `Video` を返し、
`GET /api/videos/{id}`・`GET /api/library`・`GET /api/videos/{id}/related`・ゲストの `GET /api/videos/{id}`
の `title` が表示名になる（受け入れ条件 1）。空文字の `PUT` で元の題名に戻る（受け入れ条件 2）。ゲストの
応答に `fileTitle`・`displayName` が無く、ゲストの `PUT` は `401`（受け入れ条件 9）。制御文字と長すぎる
名前が `400` と各 `reason`、`limit: 200`。`/api/events` に `video` が流れる。

### 指定の位置で代表サムネイルを作り直して記録する

**Scope**: `media.ThumbnailAt`（[data-model.md §5](data-model.md#5-生成物)）、`domain.CheckThumbnailPosition`
と誤りの値、`OverrideStore.SetThumbnailPosition`（[§3](data-model.md#3-保存層の操作)）、
`app.Ingest.SetThumbnailPosition`（R-4）と job の位置の読み替え（R-5）、`thumbnailURL` の版（R-6）。

**Dependencies**: 表示名を保存し、題名・並び替え・検索に反映する

**Acceptance**: `task check` が通る。app の試験で、`SetThumbnailPosition` が生成の錠の中で
`PublishThumbnail`→`SetThumbnailPosition` の順に呼び、生成の失敗では保存を呼ばず
`ErrThumbnailFrameUnavailable` を返す。解析前は `ErrDurationUnknown`、尺以上は
`ErrThumbnailPositionOutOfRange`。`Thumbnail`（job）が位置のある動画で `ThumbnailAt` を、無い動画で
`Thumbnail` を呼ぶ。job が錠の外で動画を読んだあと、錠を待つ間に `SetThumbnailPosition` が位置と
`done` を記録した場合、job は錠の中で読み直して生成を飛ばし、記録された位置の画像が残る。store の試験で、記録の取引が `thumbnail_state` を `done` にし `thumbnail_first_frame`
を消し、`nil` で行の列が null（両方 null なら行が無い）になる。`media` の試験（ffmpeg あり）で
`ThumbnailAt` が指定の秒の画像を書く。

### 画面の API で代表サムネイルの位置を設定・解除する

**Scope**: `PUT /api/videos/{id}/thumbnail-position`、所在の解決（`openMediaFile` と同じ規則）、
`ErrorReason` の 3 値、`api/openapi.yaml` と生成物、`web/src/api/client.ts` の関数と文言
（[contracts/screen-api.md §2・§3](contracts/screen-api.md#2-put-apivideosidthumbnail-position)）。

**Dependencies**: 指定の位置で代表サムネイルを作り直して記録する、画面の API で表示名を設定・解除する

**Acceptance**: `task check` が通る。httpapi の試験で、所有者の `PUT` が `200` と `thumbnailPositionMs`
付きの `Video` を返し、`thumbnailUrl` の版が前と変わり、`GET /api/library` のその動画も同じ URL になる
（受け入れ条件 5）。同じ位置を指定し直しても版が変わり、ゲストの `thumbnailUrl` に位置の値が
入らない。`null` で位置が消え URL が元に戻る（受け入れ条件 6）。解析前は `409 duration_unknown`、
尺以上は `400 thumbnail_position_out_of_range` と `limit`、生成の失敗は `409 thumbnail_frame_unavailable`
で前の `thumbnailUrl` のまま。ゲストは `401`。

### 外部連携 API と MCP で表示名とサムネイルの位置を一括で設定・解除する

**Scope**: `POST /api/v1/video-display-names`・`POST /api/v1/video-thumbnails`、`ExternalVideo` の 3 項目、
`api/external-v1.yaml` の `code`・`reason` と生成物、MCP の 2 ツール
（[contracts/external-api.md](contracts/external-api.md)）。`docs/how-to/external-api.md` の使い方
（スクレイパーが題名を整える例、サムネイルの 20 件の区切り、途中の失敗の `index` の扱い）、
ARCHITECTURE.md の MCP の段落（8 ツール）と 026 の `contracts/mcp.md` の表。

**Dependencies**: 画面の API で代表サムネイルの位置を設定・解除する

**Acceptance**: `task check` が通る。httpapi の試験で、トークン付きの `POST /api/v1/video-display-names` が
`200` を返し、画面の `GET /api/videos/{id}` の `title` と `GET /api/v1/videos/lookup` の `title`・
`displayName`・`fileTitle` に出る（受け入れ条件 8）。`null` で戻る。引けない動画を含む要求は `404` と
`index` で何も反映しない。`POST /api/v1/video-thumbnails` が `200` を返し画面の `thumbnailUrl` が変わる。
21 件は `400 too_many_videos`、途中の生成の失敗は `409 thumbnail_frame_unavailable` と `index` で、それより
前の項目だけ反映される。MCP のクライアントから 8 つのツールが並び、`update_video_display_names` の結果が
`get_video` に出る。全操作が `bearerAuth` で Bearer に分類される。

### 動画ページで表示名を編集し、元のファイル名を見せる

**Scope**: 動画ページの題名の編集（保存・解除、誤りの表示）と、表示名が設定されているときの元のファイル名
の表示。`useVideoDetail` の応答の差し替え、英語のカタログの文言。見た目と操作は `ui-design.md` に従う。
ゲストには編集の入口を出さない。

**Dependencies**: 画面の API で表示名を設定・解除する

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、所有者が
動画ページから移動せずに表示名を保存すると題名が置き換わり、元のファイル名が従の扱いで見え、解除で
戻る（受け入れ条件 1・2・7）。ゲストの動画ページに編集の入口が無い（受け入れ条件 9）。長すぎる名前で
理由が出る（Edge Case）。

### プレイヤーの今の場面を代表サムネイルにする

**Scope**: プレイヤーの今の再生位置（再生中・一時停止中）を 1 回の操作で `PUT
/api/videos/{id}/thumbnail-position` に送る入口と、解除、待ちと失敗の表示。ゲストには出さない。
見た目と操作は `ui-design.md` に従う。

**Dependencies**: 画面の API で代表サムネイルの位置を設定・解除する

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、
一時停止中の位置がミリ秒で送られ、応答の `Video` で画面が更新され、失敗の理由が出て前の状態が
残る（受け入れ条件 5・6、Edge Case）。時刻を数値で入力させる入口が無い（UI 品質）。
