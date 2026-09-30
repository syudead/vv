# Implementation Plan: 同じ動画の別バージョンを束ねる（スキャン時の検出と手動）

**Branch**: `feature/030-video-versions` | **Parent Issue**: #572

**Input**: The parent Issue. It is this feature's specification.

## Summary

同じ映像の別ファイル（再エンコード・解像度違い・コンテナ違い・中身を書き換えただけのもの）を
「同じ動画の別バージョン」として 1 つの集まりに束ね、一覧では代表の 1 本だけを見せる。集まりのタグ・
再生位置・公開の設定は 1 組で、代表を替えても変わらず、外したバージョンは束ねる前の値に戻る。
スキャンで同じパスの中身が変わったときは尺が合えば前の動画の値を引き継ぎ、映像の見た目が同じ別の
動画は指紋で見つけて候補として示す。

- **保存**: 集まりは利用者データの表 `video_bundles`・`video_bundle_members` で持ち、集まりの値は
  `playback_progress`・`video_tags`・`public_videos` の既存の表に、集まり自身の鍵（`bundle:<id>`）で置く
  （[research.md R-1](research.md)、[data-model.md §1・§2](data-model.md)）。
- **鍵の引き直し**: 利用者データを読む・書く箇所はすべて「利用者データの鍵」（束ねた動画なら集まりの鍵、
  そうでなければ `content_key`）を 1 つの式で引き、動画を返す読み出しは `Video.UserKey` に載せる
  （[R-2](research.md)、[data-model.md §3](data-model.md)）。
- **一覧**: 一覧・検索・フォルダ・グループは「見せる動画」（束ねていない動画と、各集まりの実効の代表）だけを
  項目にし、検索式は集まりの全所在に、範囲は代表の所在に掛ける（[R-3](research.md)、[R-4](research.md)、
  [data-model.md §4](data-model.md)）。
- **引き継ぎ**: `UpsertVideo` が同じパスの中身の変化を後継の候補として記録し、記録した走査が閉じて新しい
  中身の解析が終わったときに尺を比べて引き継ぐ（[R-5](research.md)、[data-model.md §5](data-model.md)）。
- **検出**: シーク用スプライトのコマの pHash を指紋にする取り込みの段階 `fingerprint` を足し、指紋を書く
  取引の中で候補を求めて表に置く。「違う動画」の判断は利用者データとして残す（[R-6](research.md)、
  [R-7](research.md)、[data-model.md §6・§7](data-model.md)）。
- **API**: バージョンの一覧、束ねる、代表を替える、外す、候補の一覧と却下の経路を足し、集まりは動画の id で
  指す（[R-8](research.md)、[contracts/screen-api.md](contracts/screen-api.md)）。束ねの変化は
  `domain.VideoBundleChanged` で `/api/events` の `video` に写す（[R-9](research.md)）。外部連携 API は
  畳まず、値だけが集まりのものになる（[R-10](research.md)）。
- **画面**: 動画ページのバージョンの導線、一覧の選択から束ねる操作、候補の一覧画面。`ui` ラベルがあるので、
  見た目と操作は次の design 段階の `ui-design.md` が親 Issue の「UI品質」を基準に決める。一覧のカードは
  変えない。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・索引と利用者データの区分・生成物の置き場・ドメインイベント・認証の境界:
  [ARCHITECTURE.md](../../ARCHITECTURE.md)、[.golangci.yml](../../.golangci.yml)（depguard）
- 内容鍵と取り込み: [internal/scanner/content_key.go](../../internal/scanner/content_key.go)、
  [internal/store/scan_index.go](../../internal/store/scan_index.go)（`UpsertVideo`）、
  [internal/store/ingest_results.go](../../internal/store/ingest_results.go)（`ApplyProbeForJob`）、
  [internal/app/ingest.go](../../internal/app/ingest.go)、[internal/jobs/worker.go](../../internal/jobs/worker.go)、
  [internal/domain/job.go](../../internal/domain/job.go)（`JobKinds`・`ClaimConditionFor`）
- 一覧・ライブラリの項目・フォルダ・関連: [internal/store/listing.go](../../internal/store/listing.go)
  （`chosenLocationsCTE`・`filteredFrom`）、[internal/store/library_items.go](../../internal/store/library_items.go)、
  [internal/store/search.go](../../internal/store/search.go)、[internal/store/folders.go](../../internal/store/folders.go)、
  [internal/store/related.go](../../internal/store/related.go)、
  [specs/013-library-search/data-model.md](../013-library-search/data-model.md)、
  [specs/017-folder-groups/data-model.md](../017-folder-groups/data-model.md)、
  [specs/027-partial-group-search/contracts/library-api.md](../027-partial-group-search/contracts/library-api.md)
- 内容鍵に結ぶ利用者データ: [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
  [specs/016-single-account-auth/data-model.md](../016-single-account-auth/data-model.md)、
  [specs/029-video-overrides/data-model.md](../029-video-overrides/data-model.md)、
  [internal/store/visibility.go](../../internal/store/visibility.go)、
  [internal/store/video_tags.go](../../internal/store/video_tags.go)、
  [internal/store/progress.go](../../internal/store/progress.go)
- シーク用スプライトと生成物: [docs/design-docs/seek-sprite-generation.md](../../docs/design-docs/seek-sprite-generation.md)、
  [internal/domain/seek_sprite.go](../../internal/domain/seek_sprite.go)、
  [internal/artifacts/store.go](../../internal/artifacts/store.go)（`SeekSprite`・`SeekSpriteSheet`）
- 取り込みの進捗と問題: [specs/024-import-progress/research.md](../024-import-progress/research.md)
- 画面の API と誤りの形: [api/openapi.yaml](../../api/openapi.yaml)、
  [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md)、
  [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)、
  [specs/014-video-tags/contracts/tags-api.md](../014-video-tags/contracts/tags-api.md)
- 外部連携 API と MCP: [api/external-v1.yaml](../../api/external-v1.yaml)、
  [specs/026-external-api/contracts/external-api.md](../026-external-api/contracts/external-api.md)
- 画面: [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md)（「Video facts」）、
  [specs/017-folder-groups/ui-design.md](../017-folder-groups/ui-design.md)（選択バー）、
  [web/src/player/VideoPage.tsx](../../web/src/player/VideoPage.tsx)、
  [web/src/library/SelectionBar.tsx](../../web/src/library/SelectionBar.tsx)、
  [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)、
  [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md)
- 先行の検討: ブランチ `claude/video-detection-synonym-zgwmlw` の `docs/design-docs/video-identity.md`
  （閉じた PR #492）。案 A（別名の鍵へ付け替える）はここで採らない（R-1）
- 生成と検査の入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task check-docs`・`task generate`）

**Feature-specific context**:

- 移行は表を足す単位ごとに 1 つ。`scripts/migrations-immutable.sh` が PR の基点にある移行の編集を
  禁じるため、feature branch に入った移行は変えず、後の単位は新しい番号の移行を足す。
  `00022_video_versions.sql` が利用者データの表 3 つ（`video_bundles`・`video_bundle_members`・
  `video_version_dismissals`）、`00023_video_successions.sql` が `video_successions`、
  `00024_video_fingerprints.sql` が `video_fingerprints` と `jobs.kind` の `fingerprint`、
  `00025_video_version_candidates.sql` が `video_version_candidates` を足す（[data-model.md §1](data-model.md)）。`videos`・`video_locations` と
  既存の利用者データの表には列を足さない。
- Go・npm とも依存は足さない。指紋は標準ライブラリの `image/jpeg` と DCT の数十行で作り、ffmpeg は
  1 回も余計に起動しない（R-6）。
- 尺の一致の幅は `max(1 秒, 尺の 0.5%)`（`domain.DurationsMatch`）。引き継ぎ（要件 8）と候補の絞り込み
  （要件 9）で同じ値を使う。
- 指紋の閾値（比べるコマの最小数 3、ハミング距離の中央値の上限 12、単色とみなす輝度の分散）は
  `internal/domain` の定数で、`FingerprintVersion` を持つ。値は再エンコードした試験用の動画で決め、
  実データで直すときは版を上げて作り直す（R-6）。
- `domain.FolderIndexVersion` を 2 に上げる。索引の規則（代表以外のバージョンの所在を入れない）が
  変わるため、起動時に作り直す（R-4）。
- `SearchKeyVersion` は上げない。検索の鍵は所在ごとのままで、当て方だけが変わる（R-3）。
- `quickstart.md` は作らない。受け入れ条件は store・app・media（ffmpeg あり）・httpapi・Vitest の試験で
  確かめられ、リポジトリの検査の外で実行する手順はない。
- 表示名（`video_overrides.display_name`）を要件 8 の引き継ぎに含めるかは親 Issue に無い。引き継ぐのは
  親 Issue のとおりタグ・再生位置・公開の設定の 3 つにし、表示名は Plan PR の本文で要求者に問う。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。
  - `internal/domain`: `Video.UserKey`・`VideoVersions`、尺の一致、指紋（`FrameHash`・`Fingerprint`・
    `CompareFingerprints`）、`JobFingerprint` と取り出しの条件、`VideoBundleChanged`。純粋な値と関数だけ
    （`image` は使わない。JPEG の読み出しは `internal/media`）。
  - `internal/store`: 移行、`VersionStore`、鍵の引き直し、見せる動画の CTE、後継の記録と引き継ぎ、指紋と
    候補の書き込み、SQLite の関数 `vv_fingerprint_distance`（`vv_shuffle_key` と同じ登録）。
  - `internal/media`: スプライトのシートから各コマの 32×32 の輝度を取り出して指紋にする（ffmpeg は呼ばない）。
  - `internal/app`: `Ingest.Fingerprint`（job）。生成物の読み出しは宣言した interface 越し。
  - `internal/httpapi`: 要求の解釈と応答への変換。`cmd/mdm`: ワーカーと購読の配線。
- **役割の型は他の役割の公開メソッドを呼ばない**（ARCHITECTURE.md `store.DB` の段落）: 合格。利用者データの
  鍵の式、見せる動画の CTE、集まりの値の写し、フォルダの索引の作り直し（`rebuildFolderIndex`）は
  パッケージ内の関数として共有する。
- **索引と利用者データの区別**（「Rebuildable and user data」）: 合格。集まり・却下は利用者データで
  `videos` への外部キーを張らず、指紋・候補・後継は索引で、内容の参照が無くなるときに消す
  （data-model.md §1・§6）。
- **ゲストに見せる読み出しは `visibleLocationCondition` を通す**: 合格。見せる動画の CTE は見る人の条件を
  受け、公開は集まりの鍵で判定する（R-2、R-3）。
- **生成物の置き場は変えない**（「Generated files have one owner」）: 合格。指紋はスプライトを読むだけで、
  新しい生成物を置かない。
- **ドメインイベントは確定後に発行し、購読の登録は `cmd/mdm/events.go` の 1 か所**: 合格（R-9）。
- **API の正本と生成物**（AGENTS.md）: 合格。`api/openapi.yaml` を直して `task generate`。
  `api/external-v1.yaml` は変えない（R-10）。
- **制約は試験で確かめる**（core-beliefs.md）: 合格。不変条件（集まりの代表はメンバーである、1 本の集まりは
  無い）を `invariants_test.go` に足し、指紋の閾値は ffmpeg で再エンコードした動画の試験で固定する。
- **文書は変更と同じ PR で直す**（core-beliefs.md）: 合格。各単位が ARCHITECTURE.md・`api/openapi.yaml` の
  自分の部分を直す。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/030-video-versions/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-11
├── data-model.md         # 表、domain の値、鍵の規則、見せる動画、引き継ぎ、指紋と候補、保存層の操作
└── contracts/
    └── screen-api.md     # 画面の API の差分（バージョン・束ね・候補・Video・events）
```

`ui-design.md` は次の design 段階が作る（`ui` ラベル）。`quickstart.md` は作らない（Technical Context）。

### Source Code

**Affected boundaries**:

- `internal/domain`（鍵・バージョン・尺の一致・指紋・job の種類・イベント）、`internal/store`（移行、
  `VersionStore`、読み出しと書き込みの鍵、CTE、後継、指紋と候補、フォルダの索引）、`internal/media`
  （スプライトの指紋）、`internal/app`（`Ingest.Fingerprint`、`Scans` の活動の種類）、`internal/jobs`
  （段階の追加は `cmd/mdm` の配線）
- `internal/httpapi`（バージョン・束ね・候補の経路、`Video.versions`、鍵の引き直し）、`api/openapi.yaml` と
  生成物、`cmd/mdm`（ワーカー・購読）
- `web/src/api`（新しい経路の関数）、`web/src/player`、`web/src/library`（選択バー）、新しい候補の画面、
  `web/src/shell`（導線）、`web/src/i18n`
- `ARCHITECTURE.md`

**New paths**:

- `internal/store/migrations/00022_video_versions.sql`・`00023_video_successions.sql`・
  `00024_video_fingerprints.sql`・`00025_video_version_candidates.sql`、`internal/store/versions.go`（`VersionStore`）、
  `internal/store/user_keys.go`（鍵の式と見せる動画の CTE）、`internal/store/successions.go`、
  `internal/store/fingerprints.go`
- `internal/domain/video_version.go`、`internal/domain/fingerprint.go`
- `internal/media/fingerprint.go`
- `internal/httpapi/video_versions.go`、`internal/httpapi/version_candidates.go`
- `web/src/player/` のバージョンの部品、`web/src/library/` の束ねる部品、候補の画面のディレクトリ
  （名前は design 段階に従う）

**Structure decision**: 集まりの読み書きは新しい役割 `VersionStore` に置く。束ねる・外す・代表を替える操作は
利用者データの表を書き、同じ取引でフォルダの索引を作り直し、確定後にイベントを発行するので、
`OverrideStore` と同じく `*DB` を持つ役割にする（R-1、R-4）。指紋と候補の書き込みは取り込みの段階の
結果なので `IngestStore`、後継の記録は走査の反映なので `ScanIndexStore` に置く（R-5、R-7）。指紋の
計算は `internal/media` が画像を扱い、ハッシュと比較の規則は `internal/domain` が持つ（R-6）。

## Implementation Work

### 集まりの保存と利用者データの鍵の引き直し

**Scope**: `00022_video_versions.sql` の利用者データの表（`video_bundles`・`video_bundle_members`・
`video_version_dismissals`）と `FolderIndexVersion` の更新、`domain.Video.UserKey`・`VideoVersions`・
`VideoBundleChanged`、`VersionStore` の `Bundle`・`MakeRepresentative`・`Unbundle`・`Versions`
（[data-model.md §1・§2・§8](data-model.md)）。利用者データの鍵の式と、それを使う読み出し・書き込みの
全経路（`Video.UserKey`、`publicVideoCondition`、タグの絞り込みと照合、視聴状態、`SaveProgress`、
タグの付け外しと要約と本数、公開の切り替えとゲストの打ち切り、外部連携の一覧と引き当て。[§3](data-model.md)）。
`cmd/mdm/events.go` の画面の購読。ARCHITECTURE.md の利用者データの一覧と `store.DB` の役割の一覧。
一覧の畳み込みはまだ行わない（次の単位）。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、タグ X の A とタグ Y の B を A を代表に束ねると
A・B どちらの `UserKey` も同じ集まりの鍵になり、その鍵のタグが X だけで、B の `content_key` の
行にタグ Y が残る（受け入れ条件 6 の保存の部分）。代表を B に替えても集まりの鍵とタグは変わらない
（受け入れ条件 8 の保存の部分）。B を外すと B の `UserKey` が `content_key` に戻りタグ Y が引け、
集まりのタグは X のまま（受け入れ条件 9）。残りが 1 本になると集まりの行が消え、残った 1 本の
`content_key` に集まりの値が写る。集まり同士を束ねると 1 つの集まりになり、値は選んだ代表の集まりの
もので、吸収した集まりの行は消える（その鍵の値は残る）。`SaveProgress` を B の `UserKey` に書くと A の `progress` が進む（受け入れ条件 7 の保存の部分）。
`SetVideosPublic` が集まりの鍵に書き、返す鍵に全メンバーの `content_key` が入る。不変条件
（代表はメンバー、メンバー 2 本未満の集まりは無い、`bundle:` で始まらない `user_key` は無い）が
`invariants_test.go` で通る。確定後に `VideoBundleChanged` が 1 回発行される。

### 一覧・検索・フォルダ・グループで集まりを代表の 1 件に畳む

**Scope**: 見せる動画の CTE（実効の代表の規則を含む）と、それを通す全読み出し: `chosenLocationsCTE`
（検索式を集まりの全所在に掛ける）、`libraryItemsCTE`・`LibraryIDs`、フォルダの直下の動画とフォルダの
本数（`folders.go`）、`DirectVideoPaths`・`VideosAddedNear`、フォルダの索引の入力（`folderIndexLocations`）と
束ねの操作での作り直し、タグの本数（[data-model.md §4](data-model.md)）。`api/openapi.yaml` の
`listLibrary`・`listVideos`・`listFolderVideos`・フォルダの説明文と生成物、ARCHITECTURE.md の一覧の段落。

**Dependencies**: 集まりの保存と利用者データの鍵の引き直し

**Acceptance**: `task check` が通る。store の試験で、A を代表に束ねると `ListLibrary`・`ListVideos` に A の
1 件だけが出て `total` が 1、タグ Y で絞っても出ない（受け入れ条件 6）。B の題名にだけ含まれる語で
検索すると A の 1 件が出る（受け入れ条件 10）。A と B が別のフォルダにあるとき、B のフォルダの
`ListFolderVideos` に B が出ず、A のフォルダに A が出る。B だけのフォルダは動画の無いフォルダとして
数えられ、B はどのフォルダのグループにも入らない（受け入れ条件 11）。代表を B に替えると 1 件の
題名とサムネイルが B のものになる（受け入れ条件 8）。代表の所在が消えると残っているバージョンが
1 件として出て、全部消えると出ない。ゲスト（集まりを公開）には A の 1 件が出る（受け入れ条件 12）。
`GET /api/videos/{id}` は代表以外のバージョンも返す。

### バージョンと束ねの画面の API

**Scope**: `GET /api/videos/{id}/versions`、`POST /api/video-bundles`、`POST /api/videos/{id}/make-representative`、
`POST /api/videos/{id}/unbundle`、詳細の `Video.versions`、`ErrorReason` の追加、`api/openapi.yaml` と
生成物、`web/src/api/client.ts` の関数と `web/src/i18n/errors.ts` の文言
（[contracts/screen-api.md §0〜§4](contracts/screen-api.md)）。`accessRoutes` の試験（`versions` の
GET はゲストも可、ほかは所有者だけ）。ARCHITECTURE.md の API の段落。

**Dependencies**: 一覧・検索・フォルダ・グループで集まりを代表の 1 件に畳む

**Acceptance**: `task check` が通る。httpapi の試験で、所有者の `POST /api/video-bundles` が `200` と
`VideoVersions`（代表が先頭、各項目に解像度・コーデック・大きさ・`location`）を返し、
`GET /api/videos/{id}/versions` が同じ並びを返し、`GET /api/videos/{id}` の `versions.count` が 2 になる。
`make-representative` のあと `GET /api/library` の 1 件が B になり `tags` は X のまま（受け入れ条件 8）。
`unbundle` のあと B が一覧に戻りタグが Y（受け入れ条件 9）。B の `GET /api/videos/{id}` の `progress` が
集まりのもので、`PUT /api/videos/{id}/progress` が一覧の A の視聴状態を進める（受け入れ条件 7）。
ゲストの `GET /api/videos/{id}/versions` が公開の集まりの全バージョンを返し、`POST` は `401`
（受け入れ条件 12）。1 本だけ・代表が含まれない・束ねていない動画への操作が契約の `400`。
`/api/events` に全メンバーの `video` が流れる。

### スキャン時の同じパスの中身の引き継ぎ

**Scope**: `00023_video_successions.sql` の `video_successions`（[data-model.md §1・§5](data-model.md)）、`UpsertVideo` での後継の記録と
取り消し（前の中身が別のパスに現れたとき）、`FinishScan`（`done`）と `ApplyProbe`・`ApplyProbeForJob` での
尺の比較と引き継ぎ
（集まりのメンバーなら位置の引き継ぎ、そうでなければ 3 つの表の行の付け替え）、内容の参照が無くなる
ときの行の削除、`domain.DurationsMatch`。ARCHITECTURE.md の走査の段落。

**Dependencies**: 集まりの保存と利用者データの鍵の引き直し

**Acceptance**: `task check` が通る。store の試験で、タグと再生位置を付けた動画 A のパスに、尺が同じで
`content_key` の違うファイルを upsert して解析を書くと、そのパスの動画にタグと再生位置が付いている
（受け入れ条件 1）。尺が幅の外なら何も付かない（受け入れ条件 2）。前の中身が集まりの代表なら新しい
中身が代表になり、代表以外なら同じ集まりのメンバーになる（Edge Case）。前の尺が null なら記録しない。
同じ走査で前の中身が別のパスに現れると記録が消え、引き継がない（Edge Case）。新しい中身の解析が
走査の途中で終わっても、走査が `done` で閉じるまで引き継がず、その後に前の中身が別のパスで見つかる
順序でも引き継がない。解析が失敗しても記録は残り、やり直しで成功したときに判定する。引き継ぐと、
前の中身が属していた集まりの全メンバーの `VideoBundleChanged` が発行される。

### シーク用スプライトから映像の指紋を作る取り込みの段階

**Scope**: `domain.JobFingerprint`（`JobKinds` の最後、取り出しの条件はシーク用サムネイルの完了）、
`domain.Fingerprint`・`FrameHash`・`CompareFingerprints`・閾値と `FingerprintVersion`
（[data-model.md §6](data-model.md)）、`media.SpriteFingerprint`（シートの JPEG から各コマの 32×32 の
輝度）、`app.Ingest.Fingerprint`、`IngestStore.ApplyFingerprintForJob`（候補の算出は次の単位）、
`00024_video_fingerprints.sql` の `video_fingerprints` と `jobs.kind` の移行（完成したスプライトの動画に
job を積む。[data-model.md §1](data-model.md)）、シーク用サムネイルの
完了の取引で job を積む、走査での指紋の欠けの積み直し（`IndexedVideo.FingerprintMissing` と
`EnsureJob`）、ワーカーと起床の配線、`ScanActivity` の種類と画面の文言。ARCHITECTURE.md の
取り込みの段落。

**Dependencies**: None

**Acceptance**: `task check` が通る。domain の試験で、同じ画像の明るさ・コントラストを変えたものの
ハミング距離が閾値の内、別の画像が外。media の試験（ffmpeg あり）で、同じ動画を解像度違いで
再エンコードした 2 本のスプライトの指紋が `CompareFingerprints` で一致し（405 秒を超えて間隔が違う 2 本も
コマを時刻で合わせて一致する）、尺だけ同じ別の動画が
一致しない（受け入れ条件 3・4 の判定の部分）。app の試験で、job がスプライトを読んで指紋を書き、
スプライトが無ければ失敗を返す。store の試験で、シーク用サムネイルの完了で `fingerprint` の job が
積まれ、取り出しがシーク用サムネイルの完了を待ち、走査の残りの仕事に数えられる。上限まで失敗した
`fingerprint` の job が、次の走査で積み直される。

### 候補の算出と候補の一覧・却下の API

**Scope**: `00025_video_version_candidates.sql` の `video_version_candidates`（[data-model.md §1](data-model.md)）・
`vv_fingerprint_distance`・指紋を書く取引での候補の算出
（[data-model.md §7](data-model.md)）、束ねの操作での候補の削除、内容の参照が無くなるときの削除、
`VersionStore.Candidates`・`Dismiss`、`GET /api/version-candidates`・`POST /api/version-candidates/dismiss`
（[contracts/screen-api.md §5](contracts/screen-api.md)）、`api/openapi.yaml` と生成物、
`web/src/api/client.ts` の関数。ARCHITECTURE.md の段落。

**Dependencies**: 集まりの保存と利用者データの鍵の引き直し、シーク用スプライトから映像の指紋を作る取り込みの段階

**Acceptance**: `task check` が通る。store の試験で、尺が幅の内で指紋が一致する 2 本を書くと候補が
1 組でき（受け入れ条件 3）、尺だけ同じで指紋が違う 2 本はできない（受け入れ条件 4）。却下した組は
指紋を書き直しても候補にならない（受け入れ条件 5）。同じ集まりの 2 本は候補にならず、束ねると候補が
消える。片方の動画の行が消えると候補が消える（Edge Case）。httpapi の試験で、所有者の
`GET /api/version-candidates` が両方の `Video` を持つ組を返し、`POST /api/video-bundles` で束ねると
消え、`dismiss` で消えて再スキャン後も出ない。ゲストは `401`。

### 動画ページで他のバージョンを見せ、再生・代表の変更・外す操作をする

**Scope**: 動画ページの情報の行に並ぶバージョンの導線（本数の 1 行）と、開いたときの各バージョンの
一覧（解像度・コーデック・大きさ・場所）、選んで再生（その `id` の動画ページへ移る）、代表に替える、
外す。集まりの再生位置がそのバージョンの尺以上なら最初から再生する（[R-11](research.md)）。
`useVideoDetail` の応答の差し替え、`video` イベントでの取り直し。見た目と操作は `ui-design.md` に従う。
ゲストには再生の切り替えだけを出す。

**Dependencies**: バージョンと束ねの画面の API

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、
束ねた動画のページに本数の 1 行が出て、開くと各バージョンの違いが並び、B を選ぶと B のページへ移って
B のファイルが再生される（受け入れ条件 7）。代表に替えると一覧の 1 件が B になり（受け入れ条件 8）、
外すと B が独立した 1 件に戻る（受け入れ条件 9）。ゲストには代表の変更と外す操作が無く、再生の
切り替えはできる（受け入れ条件 12）。集まりの再生位置がそのバージョンの尺以上のとき 0 から再生する。
題名・プレイヤー・タグの階層を崩さず、常に開いた大きな欄やタブを足さない（UI 品質）。

### 一覧の選択から複数の動画を別バージョンとして束ねる

**Scope**: 選択バーの「束ねる」操作と、代表を選ぶ手順、`POST /api/video-bundles` の送信、成功後の
一覧の取り直しと選択の解除、失敗の表示。候補に出ない組（劇場版と TV 版など）もこの経路で束ねられる。
見た目と操作は `ui-design.md` に従う。

**Dependencies**: バージョンと束ねの画面の API

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、
2 本以上を選ぶと束ねる操作が出て、代表を選んで送ると一覧に代表の 1 件だけが残る（要件 11、受け入れ
条件 6）。1 本の選択では出ない。グループの選択はメンバーに広げて送る。一覧のカードの見た目は変わらない
（UI 品質）。

### 「同じ動画かもしれない」候補の一覧画面

**Scope**: 所有者だけの候補の一覧の画面（置き場所と導線は `ui-design.md`）。各組の 2 本を違いが分かる形で
並べ、「同じ動画」（代表を選んで `POST /api/video-bundles`）と「違う動画」（`dismiss`）を先に置く。
`scan` イベントで取り直す。取り込み中に候補が増えることと、片方が消えて候補が消えることに耐える。

**Dependencies**: 候補の算出と候補の一覧・却下の API

**Acceptance**: 画面の変更がある（視覚と操作の確認が要る）。`task check` が通る。Vitest の試験で、
候補の組が 2 本の違い（解像度・コーデック・サイズ・場所）と共に並び、「同じ動画」で代表を選ぶと組が
消えて一覧に 1 件になり（受け入れ条件 3・6）、「違う動画」で組が消える（受け入れ条件 5）。ゲストには
導線が無い（要件 12）。
