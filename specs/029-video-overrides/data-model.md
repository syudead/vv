# Data model: 動画の表示名と代表サムネイルの上書き

親 Issue: #517。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、
データの区分は [ARCHITECTURE.md](../../ARCHITECTURE.md)「Rebuildable and user data」、所在ごとの
検索の鍵は [specs/013-library-search/data-model.md](../013-library-search/data-model.md) にある。
ここには、この feature が足す表と値、それを読み書きする規則だけを書く。書いていない表は変えない
（`videos`・`video_locations` に列は足さない。`video_locations.title` はスキャンの事実のまま）。

## 1. `video_overrides`

`00021_video_overrides.sql` として足す（`main` の最後は `00020_api_tokens.sql`）。

```sql
-- 所有者が付けた表示名と代表サムネイルの位置（specs/029-video-overrides/research.md R-1）。
-- 作り直せない利用者データで、playback_progress・video_tags・public_videos と同じく
-- 内容の識別子に結び、videos への外部キーを張らない。
create table video_overrides (
    content_key           text    primary key,
    -- domain.NormalizeDisplayName を通した表示名。未設定は null（空文字は置かない）。
    display_name          text,
    -- 代表サムネイルにする場面の位置（ミリ秒）。未設定は null。
    thumbnail_position_ms integer check (thumbnail_position_ms is null or thumbnail_position_ms >= 0),
    -- 位置を記録するたびに書く改版番号（R-6）。thumbnailUrl の版に使い、位置の値を URL に出さない。
    -- 位置が null のときは null。
    thumbnail_revision    integer,
    updated_at            integer not null
) without rowid;
```

Down は表を落とす。

不変条件（`internal/store/invariants_test.go` に足す）: `display_name` と `thumbnail_position_ms` の
両方が null の行は無い（書く側が消す）。`thumbnail_revision` は `thumbnail_position_ms` が null で
ないときだけ null でない。空の `content_key` の行は無い。

区分: 作り直せない利用者データ。ARCHITECTURE.md の一覧に足す。生成物の片付け（`RemoveContent`）は
この表に触れない。内容が変わって別の論理動画になった動画は別の `content_key` なので、行は
引き継がれない（Edge Case。タグと同じ扱い）。

## 2. `domain` に足す値

| 値 | 中身 |
| --- | --- |
| `Video.FileTitle` | ファイル名由来の題名（`video_locations.title`）。`Title` は有効な題名（表示名があればそれ）になる（[R-2](research.md#r-2-有効な題名は保存層が決めdomainvideo-が表示名とファイル名由来の題名を両方持つ)） |
| `Video.DisplayName` | 表示名。未設定は空 |
| `Video.ThumbnailPositionMs` | `*int64`。未設定は nil |
| `Video.ThumbnailRevision` | `int64`。位置の改版番号（[R-6](research.md#r-6-thumbnailurl-の版に指定の改版番号を含める)）。未設定は 0。`thumbnailUrl` の版だけに使い、応答の項目にはしない |
| `NormalizeDisplayName(string) (name string, clear bool, err error)` | [R-10](research.md#r-10-表示名の規則はタグ名の規則にそろえ上限は-200-符号位置にする)。`clear` は整えて空だったこと。誤りは `*InvalidDisplayNameError`（`DisplayNameControlCharacters`・`DisplayNameTooLong`。`InvalidTagNameError` と同じ形） |
| `DisplayNameMaxLength = 200` | 符号位置数 |
| `CheckThumbnailPosition(Video, int64) error` | [R-11](research.md#r-11-位置の検証は-internaldomain-の純粋関数が持ち誤りは-3-つに分ける)。`ErrDurationUnknown`・`ErrThumbnailPositionOutOfRange` |
| `ErrThumbnailFrameUnavailable` | 指定の位置で画像を作れなかった。`internal/media` の `ThumbnailAt` の失敗を `internal/app` が包む |
| `VideoOverrideChanged{VideoID}` | 上書きが変わった（[R-7](research.md#r-7-上書きの変化は-domainvideooverridechanged-を発行し画面の-video-通知に写す)）。`Event` を実装する |
| `ExternalVideo` | 変えない。`Video` が増えた項目を運ぶ |

## 3. 保存層の操作

読み出しは `videoColumnsTemplate`・`listColumns`・外部連携の一覧の列に `video_overrides` の左結合
（`ov.content_key = videos.content_key and videos.content_key <> ''`）を足し、
`coalesce(ov.display_name, loc.title)` を `Title`、`loc.title` を `FileTitle`、`ov.display_name`・
`ov.thumbnail_position_ms`・`ov.thumbnail_revision` をそれぞれの項目に写す。`IndexedVideo` は変えない。

新しい役割 `OverrideStore`（`VisibilityStore` と同じく SQL 接続だけを持つ）:

| 操作 | 1 つの取引で行うこと |
| --- | --- |
| `SetDisplayName(ctx, videoID, name string)`（`name` が空なら解除） | `videoID` を `registeredContentKeysForVideoIDs` で内容鍵に引き直す（無ければ `ErrNotFound`）。行を upsert し、両方が null なら消す。その内容鍵の動画の全所在の `title_key`・`search_key` を書き直す（§4）。確定後に `VideoOverrideChanged` を発行し、有効な題名を含む `Video` を返す |
| `SetDisplayNames(ctx, []DisplayNameChange)`（外部連携の一括） | 各 `VideoRef` を引き当て（引けなければ `index` 付きの `ErrNotFound`）、上と同じことを全件行う。1 件でも失敗すれば何も残さない |
| `SetThumbnailPosition(ctx, videoID, positionMs *int64)` | 行を upsert（`nil` は null。両方 null なら消す）。位置を書くときは `thumbnail_revision` を「今のミリ秒時刻と前の値 + 1 の大きい方」にし、`nil` では null にする。同じ取引で、その内容鍵の動画の `thumbnail_state` を `done`・`updated_at` を今にし、`thumbnail_first_frame` の代用の行を消す（`applySubstitution` と同じ表）。確定後に `VideoOverrideChanged` を発行する。**呼ぶのは `app.Ingest` だけで、公開が済んだあとに呼ぶ**（[R-4](research.md#r-4-サムネイルの位置の指定は要求の中で生成してから記録する)） |

`IngestStore.GetVideo` も上書きを載せて返す（job が生成の錠の中で読み直して位置と状態を決める。[R-5](research.md#r-5-取り込みのサムネイル-job-も指定の位置で作り自動の規則は-internalmedia-に残す)）。

`app.Ingest` が宣言する interface（`IngestStore`）に `SetThumbnailPosition` を足し、`internal/httpapi` が
宣言する interface に `SetDisplayName`・`SetDisplayNames` と、`app.Ingest` の `SetThumbnailPosition` を
呼ぶための `ThumbnailPicker` を足す。配線は `cmd/mdm`。

## 4. 並び替えと検索の鍵

[R-3](research.md#r-3-並び替えと検索は所在ごとの-title_keysearch_key-に表示名を織り込む)。所在 1 件の鍵は
次のように変わる（[specs/013-library-search/data-model.md §3・§4](../013-library-search/data-model.md)
の規則に表示名を足す）。

| 列 | 表示名なし（今と同じ） | 表示名あり |
| --- | --- | --- |
| `title_key` | `NaturalSortKey(title)` | `NaturalSortKey(display_name)` |
| `search_key` | `part(title) + "\n" + part(rel)` | `part(title) + "\n" + part(rel) + "\n" + part(display_name)` |

`searchKeyTarget` に `displayName` を足し、鍵を作り直す 4 つの経路（`refreshSearchKeysByPath`、
`refreshSearchKeysUnder`（`AddMediaFolder`・`ReplaceMediaFolder` が呼ぶ）、
`refreshSearchKeysForContentKey`（新設。表示名の取引が呼ぶ）、起動時の `RefreshSearchKeys`）は
`video_locations` と `videos`・`video_overrides` を結んで表示名を読む。`SearchKeyVersion` は上げない。
検索語の照合（`domain` の `search.go`）は変えない。改行を境目に使う規則はそのままで、3 つ目の部分も
`searchKeyPart` で改行を空白にする。

## 5. 生成物

置き場の並べ方は変えない。指定の位置の画像も `<p>/<s>.jpg` に置く（[R-5](research.md#r-5-取り込みのサムネイル-job-も指定の位置で作り自動の規則は-internalmedia-に残す)）。
`internal/media` に `ThumbnailAt(ctx, videoPath string, positionMs int64, output string) error` を足す。
`thumbnailArgs` を再利用し、先頭のコマへの代用はしない。`app.Ingest` の `Generator` interface に同じ
署名を足す。
