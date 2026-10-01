# Implementation Plan: 動画の更新日時とファイルの作成日時を持ち、表示・並び替え・外部 API で使う

**Branch**: `feature/033-video-dates` | **Parent Issue**: #630

**Input**: The parent Issue. It is this feature's specification.

## Summary

動画ごとに「vv 上で情報を最後に編集した日時（更新日時）」と「一覧に出す所在のファイルの作成日時」を持ち、
動画ページの情報欄に出し、一覧の並び順に「作成日」を足し、外部連携 API の動画に両方を載せる。今の「更新日」
（mtime）の並び替えは名前も中身も変えない。

- **更新日時**: 内容の識別子に結ぶ利用者データの表 `video_edits` に持ち、読み出しで追加日時に倒す。進めるのは
  表示名・代表サムネイルの位置・公開の設定・手で付けるタグの付け外しの 4 種の書き込みだけで、実際に行が変わった
  内容の識別子にだけ書く。同じパスの引き継ぎで付け替える
  （[research.md R-1](research.md#r-1-更新日時は内容の識別子に結ぶ利用者データの表-video_edits-に持ち読み出しで追加日時に倒す)、
  [R-2](research.md#r-2-更新日時を進めるのは動画の情報を書く-4-種の操作だけでタグ自体集まり取り込みでは進めない)、
  [R-3](research.md#r-3-変わらなかった編集は進めず進める対象は書き込みが実際に変えた内容の識別子だけにする)、
  [data-model.md §1・§3](data-model.md)）。
- **作成日時**: 所在の列 `video_locations.file_created_at`（取れなければ null）に走査が書き、読み出しと並べ替えは
  `coalesce(file_created_at, mtime)`。`internal/scanner` が OS ごとに読み（Linux は `x/sys/unix` の statx）、
  変わっていないファイルでも違えば所在の列だけを書き直す
  （[R-4](research.md#r-4-ファイルの作成日時は所在の列-video_locationsfile_created_at-に持ち取れなければ-null-にして読み出しで-mtime-に倒す)、
  [R-5](research.md#r-5-作成日時はファイルシステムのアダプタ-internalscanner-が-os-ごとに読みlinux-は-golangorgxsysunix-の-statx-を使う)、
  [R-6](research.md#r-6-登録済みの所在は変わっていないファイルでも作成日時が違えば次の走査で書き直す)、
  [data-model.md §4・§5](data-model.md)）。
- **API**: `Video`・`ExternalVideo` に必須の `updatedAt`・`fileCreatedAt`、`VideoSort` に `createdAsc`・`createdDesc`
  （[R-7](research.md#r-7-応答の項目は-updatedatvv-上の更新日時と-filecreatedat所在の作成日時で画面と外部連携で同じ名前にする)、
  [contracts/screen-api.md](contracts/screen-api.md)、[contracts/external-api.md](contracts/external-api.md)）。
- **画面**: 動画ページの情報欄に 2 項目、一覧の並び順に「作成日」。タグと公開の設定の変更後は動画を取り直す
  （[R-8](research.md#r-8-再生画面はタグと公開の設定を変えたあと動画を取り直し新しいドメインイベントは足さない)）。
  `ui` ラベルがあるので、見た目・項目の名前と順・並び順の名前は次の design 段階の `ui-design.md` が親 Issue の
  「UI品質」を基準に決める。一覧のカードは変えない。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・索引と利用者データの区分・役割の型の規則・ドメインイベント・認証の境界:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml)（depguard）
- 走査と所在: [internal/scanner/scanner.go](../../internal/scanner/scanner.go)（`Index` interface、変わっていない
  ファイルの分岐）、[internal/store/scan_index.go](../../internal/store/scan_index.go)（`UpsertVideo`・
  `IndexedVideosByPath`）、[internal/domain/video.go](../../internal/domain/video.go)（`VideoFile`・`IndexedVideo`）
- 動画の読み出しと一覧: [internal/store/videos.go](../../internal/store/videos.go)（`videoColumnsTemplate`）、
  [internal/store/listing.go](../../internal/store/listing.go)（`listColumns`・`listOrders`）、
  [internal/store/library_items.go](../../internal/store/library_items.go)（`libraryItemsCTE`・`itemOrderValues`）、
  [internal/domain/library.go](../../internal/domain/library.go)（`VideoSort`）、
  [specs/013-library-search/contracts/list-api.md](../013-library-search/contracts/list-api.md)、
  [specs/017-folder-groups/contracts/library-api.md](../017-folder-groups/contracts/library-api.md)
- 内容の識別子に結ぶ利用者データとその書き込み: [specs/029-video-overrides/data-model.md](../029-video-overrides/data-model.md)、
  [internal/store/overrides.go](../../internal/store/overrides.go)、[internal/store/visibility.go](../../internal/store/visibility.go)、
  [internal/store/video_tags.go](../../internal/store/video_tags.go)、
  [internal/store/external_video_tags.go](../../internal/store/external_video_tags.go)、
  [internal/store/user_keys.go](../../internal/store/user_keys.go)（`userKeysForVideoIDs`・`contentKeysForUserKeys`）、
  [internal/store/successions.go](../../internal/store/successions.go)（`moveUserData`）、
  [specs/030-video-versions/data-model.md](../030-video-versions/data-model.md) §3・§5
- ゲストへの応答: [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)
- 外部連携 API: [api/external-v1.yaml](../../api/external-v1.yaml)、
  [specs/026-external-api/contracts/external-api.md](../026-external-api/contracts/external-api.md)、
  [internal/httpapi/external_videos.go](../../internal/httpapi/external_videos.go)、
  [docs/how-to/external-api.md](../../docs/how-to/external-api.md)
- 画面の API と変換: [api/openapi.yaml](../../api/openapi.yaml)、[internal/httpapi/videos.go](../../internal/httpapi/videos.go)
  （`toAPIVideo`）
- 画面: [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md)「Video facts」、
  [specs/013-library-search/ui-design.md](../013-library-search/ui-design.md)「Sort and direction」、
  [web/src/player/VideoFacts.tsx](../../web/src/player/VideoFacts.tsx)、[web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx)、
  [web/src/videoList/listCriteria.ts](../../web/src/videoList/listCriteria.ts)（`sortKinds`）、
  [web/src/videoList/SortControls.tsx](../../web/src/videoList/SortControls.tsx)、
  [web/src/preferences/viewPreferences.ts](../../web/src/preferences/viewPreferences.ts)、
  [web/src/i18n/en.ts](../../web/src/i18n/en.ts)、[docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）

**Feature-specific context**:

- 移行は 1 つ（`00027_video_dates.sql`: `video_edits` の表と `video_locations.file_created_at` の列）。
- Go の直接依存に `golang.org/x/sys` を足す（今は間接依存。`internal/scanner` の Linux 向けファイルだけが使う）。
  npm の依存は足さない。
- ドメインイベントは足さない（R-8）。`SearchKeyVersion` は上げない。
- `quickstart.md` は、実際のファイルシステムの作成日時に依存する受け入れ条件 4・5 の確認手順を持つ。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: `Video`・`VideoFile`・`IndexedVideo`・`VideoLocation` の `time.Time` の項目と `VideoSort` の
    2 値。`os` にも `x/sys` にも触れない。
  - `internal/scanner`: 作成日時の読み取り（OS ごとの build tag）。ファイルシステムの事実はアダプタが読む。
  - `internal/store`: 移行、読み出しの列、`touchEditedAt`、`UpdateLocationCreatedAt`。
  - `internal/httpapi`: 応答への変換と `sort` の検査。`internal/app`・`cmd/mdm`: 触らない。
- **役割の型は他の役割の公開メソッドを呼ばない**（ARCHITECTURE.md `store.DB` の段落）: 合格。`touchEditedAt` は
  パッケージ内の関数で、`OverrideStore`・`VisibilityStore`・`TagStore` がそれぞれの取引の中で呼ぶ
  （data-model.md §3）。
- **索引と利用者データの区別**: 合格。`video_edits` は利用者データ、`video_locations.file_created_at` は索引。
  ARCHITECTURE.md の一覧に足す（data-model.md §1）。
- **メディアフォルダを歩くのは利用者が始めた走査だけ**（ARCHITECTURE.md `internal/scanner` の段落）: 合格。既存の
  所在の作成日時も走査の中で埋める（R-6）。
- **API の正本と生成物**（AGENTS.md）: 合格。`api/openapi.yaml`・`api/external-v1.yaml` を直して `task generate`。
  外部連携 API は項目の追加だけ（026 の互換の方針）。
- **ゲストは所有者のデータを見ない**（guest-api.md）: 合格。2 項目と `created*` の並び順は公開の動画の事実で、
  所有者の操作の経路は足さない（R-7）。
- **サーバーの出力は英語、画面の文言はカタログ**（`.golangci.yml` の gosmopolitan、i18n.md）: 合格。
- **設計文書は今どうなっているかを書く**（docs/design-docs/index.md「設計文書の方針」）: 合格。各単位が
  ARCHITECTURE.md（利用者データの一覧、「thirteen sort orders」、`TagStore`・`VisibilityStore`・`OverrideStore`・
  `ScanIndexStore` の段落）、`docs/how-to/external-api.md`、`api/*.yaml` の自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/033-video-dates/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-8
├── data-model.md         # video_edits、file_created_at、domain の値、進める規則、読み出し、走査
├── quickstart.md         # 実際のファイルシステムでの作成日時の確認（受け入れ条件 4・5）
└── contracts/
    ├── screen-api.md     # Video の 2 項目、VideoSort の 2 値、web/src/api の差分
    └── external-api.md   # ExternalVideo の 2 項目
```

`ui-design.md` は次の design 段階が作る（`ui` ラベル）。

### Source Code

**Affected boundaries**:

- `internal/domain`（値と `VideoSort`）、`internal/scanner`（作成日時の読み取りと変わっていないファイルの分岐）、
  `internal/store`（移行、読み出し、`touchEditedAt` と 4 種の書き込み、`UpdateLocationCreatedAt`、引き継ぎ、不変条件）
- `internal/httpapi`（`toAPIVideo`、`external_videos.go`、`sort` の検査）、`api/openapi.yaml`・`api/external-v1.yaml`
  と生成物、`go.mod`
- `web/src/api`（`videoSorts`、型）、`web/src/player`（情報欄、取り直し）、`web/src/videoList`（並び順）、
  `web/src/preferences`、`web/src/i18n`
- `ARCHITECTURE.md`、`docs/how-to/external-api.md`、`specs/013-library-search/contracts/list-api.md` へは足さず、
  この feature の [data-model.md §4](data-model.md#4-読み出し) が差分を持つ

**New paths**:

- `internal/store/migrations/00027_video_dates.sql`、`internal/store/video_edits.go`（`touchEditedAt`）
- `internal/scanner/file_created_at_linux.go`・`_darwin.go`（darwin・freebsd・netbsd）・`_windows.go`・`_other.go`

**Structure decision**: 更新日時の書き込みは新しい役割の型にせず、変化を起こす 3 つの役割がそれぞれの取引の中で
パッケージ内の `touchEditedAt` を呼ぶ。変わった行だけを進める（R-3）には書き込みと同じ取引で変化を知る
必要があり、別の役割では「役割の型は他の役割の公開メソッドを呼ばない」の下でその取引を組めない。作成日時の
読み取りは `internal/scanner` に置き、`internal/mediafs` には足さない。`mediafs` は「開いてよいか」の規則の
持ち主で、走査が `stat` した結果を使う場所は `scanner` である。

## Implementation Work

### 更新日時と作成日時を保存し、編集で更新日時を進め、動画の読み出しに両方を載せる

**Scope**: `00027_video_dates.sql`、`domain` の値（[data-model.md §2](data-model.md#2-domain-に足す値)）、
`touchEditedAt` と 4 種の書き込みでの呼び出しと変化の判定（[§3](data-model.md#3-更新日時を進める規則)）、
`moveUserData` への追加、動画を返す読み出しの 2 列と `VideoLocations`（[§4](data-model.md#4-読み出し)）、
`UpsertVideo` の `file_created_at` と `IndexedVideosByPath`・新設の `UpdateLocationCreatedAt`
（[§5](data-model.md#5-走査と所在の書き込み)。走査からの呼び出しは次の単位）、`invariants_test.go` の不変条件。
ARCHITECTURE.md の利用者データの一覧と、`TagStore`・`VisibilityStore`・`OverrideStore`・`ScanIndexStore` の段落。
一覧の並び順の値（`createdAsc`・`createdDesc` の `listOrders`・`itemOrderValues`・`VideoSort.Valid`）。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、`SetDisplayName`・`SetThumbnailPosition`・`SetVideosPublic`・
`AttachTagByID`・`DetachTag`・`ApplyVideoTags` のあと `GetVideo` の `EditedAt` が操作の時刻になる（受け入れ条件 1）。
同じ表示名・同じ位置・既に付いているタグ・既に同じ公開の設定では変わらない（Edge Case）。一括のタグ付けで
変わった動画だけが進む（Edge Case）。再生位置の保存・`UpsertVideo`・解析の結果・タグの改名と削除・束ねる操作では
変わらない（受け入れ条件 2、R-2）。編集していない動画の `EditedAt` が `AddedAt` と等しい（受け入れ条件 3）。
集まりのメンバーへのタグ付けで全メンバーが進む。同じパスの引き継ぎで新しい内容の `EditedAt` が前の値になる
（Edge Case）。`UpsertVideo` の `FileCreatedAt` が所在に入り、ゼロ値なら `FileCreatedAt` が `MTime` と等しい
（受け入れ条件 5）。`createdAsc`・`createdDesc` が `ListVideos`・`ListFolderVideos`・`ListLibrary`（グループは
メンバーの最大）で `coalesce(file_created_at, mtime)` の順に並び、同じ値は id で決着し、`modifiedAsc`・
`modifiedDesc` の順が `listing_sort_test.go` で変わらない（受け入れ条件 6・8）。移行のあと `video_edits` が空で、
既存の所在の `file_created_at` が null である。

### 走査がファイルの作成日時を読み、所在に記録する

**Scope**: `internal/scanner` の OS ごとの `fileCreatedAt`（[R-5](research.md#r-5-作成日時はファイルシステムのアダプタ-internalscanner-が-os-ごとに読みlinux-は-golangorgxsysunix-の-statx-を使う)）、
`go.mod` の `golang.org/x/sys` の直接依存化、`VideoFile.FileCreatedAt` の設定、変わっていないファイルでの
`UpdateLocationCreatedAt` の呼び出し（[R-6](research.md#r-6-登録済みの所在は変わっていないファイルでも作成日時が違えば次の走査で書き直す)、
[data-model.md §5](data-model.md#5-走査と所在の書き込み)）、`scanner.Index` interface の追加とテストの偽物の追従。
ARCHITECTURE.md の `internal/scanner` の段落。[quickstart.md](quickstart.md) の確認。

**Dependencies**: 更新日時と作成日時を保存し、編集で更新日時を進め、動画の読み出しに両方を載せる

**Acceptance**: `task check` が通る（`build-windows-check` を含む）。scanner の試験で、新しいファイルの
`UpsertVideo` に作成日時が渡り（作成日時を持つ OS ではゼロ値でない）、索引の値と違う変わっていないファイルで
`UpdateLocationCreatedAt` が呼ばれ、同じなら呼ばれず、`UpsertVideo` も job の積み直しも起きない（Edge Case
「既に登録済みの動画」）。作成日時を読めないときも失敗にならない。quickstart.md の手順 1〜4 の結果を PR の本文に
残す（受け入れ条件 4・5）。

### 画面の API で更新日時と作成日時を返し、「作成日」の並び順を受け付ける

**Scope**: `api/openapi.yaml` の `Video.updatedAt`・`fileCreatedAt` と `VideoSort` の 2 値と生成物
（[contracts/screen-api.md §0・§1](contracts/screen-api.md#0-video-に足す項目)）、`toAPIVideo` の変換、`sort` の
検査（ゲストにも許す）、`web/src/api` の `videoSorts` と Vitest の fixture の追従（[§3](contracts/screen-api.md#3-websrcapi-の差分)）。
ARCHITECTURE.md の「thirteen sort orders」。

**Dependencies**: 更新日時と作成日時を保存し、編集で更新日時を進め、動画の読み出しに両方を載せる

**Acceptance**: `task check` が通る。httpapi の試験で、`GET /api/videos/{id}`・一覧・関連・バージョンの `Video` に
`updatedAt`・`fileCreatedAt` が入り、編集前は `updatedAt` が `addedAt` と等しく、`PUT /api/videos/{id}/display-name`
の応答で進む（受け入れ条件 1・3）。ゲストの応答にも入る。`sort=createdDesc`・`createdAsc` が `GET /api/videos`・
`GET /api/folders/{rootId}/videos`・`GET /api/library`・`GET /api/library/ids` で受け付けられ、ゲストでも `400`
にならない（要件 5）。`openapi_routes_test.go` と生成物の検査が通る。

### 外部連携 API の動画に更新日時と作成日時を含める

**Scope**: `api/external-v1.yaml` の `ExternalVideo.updatedAt`・`fileCreatedAt` と生成物、
`internal/httpapi/external_videos.go` の変換（[contracts/external-api.md](contracts/external-api.md)）、
`docs/how-to/external-api.md`「動画の一覧を読む」の説明。

**Dependencies**: 更新日時と作成日時を保存し、編集で更新日時を進め、動画の読み出しに両方を載せる

**Acceptance**: `task check` が通る。httpapi の試験で、トークン付きの `GET /api/v1/videos` と
`GET /api/v1/videos/lookup` の動画に `updatedAt`・`fileCreatedAt` が入り（受け入れ条件 7）、
`POST /api/v1/video-tags` で付けたあとの `lookup` で `updatedAt` が進み、同じタグをもう一度付けても進まない。
MCP の `lookup_video` の出力に 2 項目が出る。

### 動画ページの情報欄に更新日時と作成日時を出す

**Scope**: `web/src/player/VideoFacts.tsx` の 2 項目（長さ・サイズ・追加日と並べる。項目の名前・順・アイコン・
形式は `ui-design.md` に従う）、`VideoTags`・`VisibilitySwitch` の成功時の `onChanged` と `VideoPage` の取り直し
（[R-8](research.md#r-8-再生画面はタグと公開の設定を変えたあと動画を取り直し新しいドメインイベントは足さない)）、
英語のカタログの文言。

**Dependencies**: 画面の API で更新日時と作成日時を返し、「作成日」の並び順を受け付ける

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、情報欄に追加日と
同じ形式で更新日時と作成日時が出て、読み上げ名で 3 つが区別できる（要件 4、UI 品質）。ゲストでも出る。表示名の
保存・タグの付け外し・公開の切り替え・代表サムネイルの設定と解除のあと、画面の更新日時が取り直した値になる
（受け入れ条件 1）。

### 一覧の並び順に「作成日」を足す

**Scope**: `web/src/videoList/listCriteria.ts` の `sortKinds`（`created`、選んだときの向きは降順）、
`SortControls.tsx` のアイコンと `md` 未満のまとめ、`viewPreferences.ts`、`listCriteria` の URL の往復、
英語のカタログの文言（「作成日」と既存の「更新日」（ファイル）の名前の区別は `ui-design.md` に従う）。
ライブラリとフォルダの両画面は `SortControls` を共有する（要件 5）。`docs/design-docs/library-ui.md` の
ツールバーの説明に並び順の種類が増えることを足す。

**Dependencies**: 画面の API で更新日時と作成日時を返し、「作成日」の並び順を受け付ける

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、並び順の
メニューに「作成日」が出て、選ぶと `sort=createdDesc` で一覧を取り、向きの切り替えで `createdAsc` になり、
URL と端末の設定に往復する（要件 5、受け入れ条件 6）。既存の「更新日」の種類の名前と値（`modified*`）は
変わらない（要件 7）。ゲストのメニューにも出る。
