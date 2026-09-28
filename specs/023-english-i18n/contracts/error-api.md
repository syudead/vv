# Contract: API エラーの理由と失敗理由のコード

正本は [api/openapi.yaml](../../../api/openapi.yaml) で、Go と TypeScript は `task generate` で
生成する。ここには、この feature が足す・変える部分だけを書く。既存の `code` の値、各経路の HTTP
状態、`state` などの機械可読な値は変えない。

## 0. すべての `message` を英語にする

`Error.message` は、利用者と API 利用者が読む英語の文にする。OS や外部プログラムが返す自由文を
含めない。利用者のデータ（パス、名前）を含めるときは、そのデータを翻訳せず、周りの説明だけを
英語にする。`description` を「英語の説明。画面は `code` と `reason` から表示し、この文は API
利用者と未知のコードに対するフォールバックである」に改める。

`internal/httpapi/spa.go` の平文の失敗応答（`http.Error`）と、`writeJSON` が応答を組み立てられない
ときの固定の JSON も英語にする。

## 1. `Error` の `reason`・`limit`・`tagName`

`Error` に次の任意の 3 項目を足す（`additionalProperties: false` は保つ）。

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `reason` | string enum（`ErrorReason`） | 同じ `code` の中で状況を区別する下位の理由。下の表の状況だけで返す |
| `limit` | integer | その理由の上限値。表で `limit` の欄に値があるときだけ返す |
| `tagName` | string | 競合の相手になったタグの元の名前（翻訳しない利用者のデータ）。表で `tagName` の欄に印があるときだけ返す |

`reason` の無い応答は今と同じで、画面は `code` の文言を出す。

| `code`（HTTP） | `reason` | `limit` | 状況（今の箇所） |
| --- | --- | --- | --- |
| `tag_name_taken`（409） | `name_is_tag` | — | 送った名前が既にあるタグの元の名前。`tagName` を返す（`tags.go` の `tagNameTakenMessage`） |
| 〃 | `name_is_synonym` | — | 送った名前が別のタグのシノニム。`tagName` はそのタグの元の名前（同上） |
| `tag_merge_required`（409） | — | — | 名前の変更先が別のタグの名前。`tagName` を返す（`tags.go`） |
| `invalid_request`（400） | `username_length` | `domain.MaxUsernameLength` | ユーザー名の長さ（`auth.go`） |
| 〃 | `password_length` | `domain.MaxPasswordBytes` | パスワードの長さ（`auth.go`） |
| 〃 | `tag_name_empty` | — | タグ名が空（`domain.NormalizeTagName`。`tags.go`・`video_tags.go`・`folder_groups.go` のフォルダ名からのタグ化） |
| 〃 | `tag_name_control_characters` | — | タグ名に制御文字（同上） |
| 〃 | `tag_name_too_long` | `domain.TagNameMaxLength` | タグ名が長すぎる（同上） |
| 〃 | `merge_same_tag` | — | 統合元と統合先が同じタグ（`tags.go`） |
| 〃 | `search_too_long` | `maxQueryLength` | 検索語が長すぎる（`videos.go`） |
| 〃 | `too_many_tag_filters` | `maxTagFilterCount` | タグの絞り込みが多すぎる（`videos.go`） |
| 〃 | `too_many_videos` | `maxVideoTagsIDs` | 一括操作の動画が 0 件または多すぎる（`video_tags.go`・`visibility.go`） |
| 〃 | `guest_filter_not_allowed` | — | ゲストが所有者向けの条件を使った（`videos.go`） |
| 〃 | `invalid_cursor` | — | 読み込み位置を解釈できない（`videos.go`・`library.go`・`folders.go`） |
| 〃 | `invalid_folder_path` | — | フォルダの指定が正しくない（`folders.go`） |
| 〃 | `relative_directory_path` | — | ディレクトリの選択に相対パス（`directories.go`） |
| `not_found`（404） | `video_not_found` | — | 動画が無い・見えない（`videos.go`・`reprobe.go`） |
| 〃 | `folder_not_found` | — | フォルダが無い・見えない（`folders.go`・`folder_groups.go`） |
| 〃 | `not_folder_group` | — | フォルダがグループでない（`library.go`） |
| 〃 | `no_scan` | — | まだ取り込みをしていない（`scans.go`） |
| 〃 | `directory_not_found` | — | 選んだディレクトリが無い（`directories.go`） |
| 〃 | `file_unavailable` | — | 動画のファイルを開けない（`stream.go`・`transcode.go`） |
| `conflict`（409） | `media_folders_changed` | — | メディアフォルダが別の操作で変わった（`media_folders.go`） |
| 〃 | `root_group_not_taggable` | — | 登録フォルダそのもののグループはタグにできない（`folder_groups.go`） |
| 〃 | `folder_not_group` | — | フォルダが今グループでない（`folder_groups.go`） |
| 〃 | `probe_info_missing` | — | シークプレビュー・ライブ変換に必要な解析情報が無い（`seek_thumbnail.go`・`transcode.go`） |
| 〃 | `seek_preview_generating` | — | シークプレビューを生成中（`seek_thumbnail.go`） |
| 〃 | `transcode_unavailable` | — | ライブ変換できない（`transcode.go`） |
| `forbidden`（403） | `cross_origin` | — | same-origin でない変更（`media_folders.go`） |
| 〃 | `open_not_local` | — | サーバーと別の PC から開こうとした（`open.go`） |

これ以外の `invalid_request`・`not_found`（並び順・`attempt`・Content-Type・経路が無いなど、画面が
送らない値）は `reason` を返さない。

`limit` の値はサーバーの定数から埋め、画面は上限を自分で持たない。今の日本語の `message` が埋め込んでいる
競合先のタグ名は `tagName` で返し、画面は送った名前と合わせて具体的な説明を作る（要件 6 の具体性）。`domain.NormalizeTagName` の
失敗は、今の文字列ではなく理由を持つ domain の誤りにし、`internal/httpapi` がそれを `reason` に写す。

## 2. `Video.probeErrorCode`

`Video` に任意の `probeErrorCode`（string enum `ProbeErrorCode`）を足す。`probeState = failed` で
コードが保存されている行だけで返す。値は [data-model.md §1](../data-model.md#1-videosprobe_error_code)
の表のとおり。`probeError`（自由文）は今のまま返す。どちらもゲストの応答では省く（今の
`probeError` と同じ扱い）。

画面は `probeError` を表示しない。`probeErrorCode` があればその説明を、無ければ（アップグレード前の
行）一般的な英語の概要を出す。

## 3. `Scan.errorCode` と `Scan.errorPath`

`Scan` に任意の 2 項目を足す。

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `errorCode` | string enum（`ScanErrorCode`） | `state = failed` でコードが保存されているときの理由。値は [data-model.md §2](../data-model.md#2-scanserror_code-と-scanserror_path) |
| `errorPath` | string | 理由が特定の場所に結び付くときの、その絶対パス（メディアフォルダ、またはその下の読めなかった場所。翻訳しない利用者のデータ） |

`error`（自由文）は今のまま返す。画面は `error` を表示せず、`errorCode` と `errorPath` から英語の
説明を作る。`errorCode` が無い `failed`（アップグレード前の行）は一般的な英語の概要を出す。
