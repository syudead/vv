---
source: specs/023-english-i18n/contracts/error-api.md
sourceHash: d5cb6446d52dfecd7b2b70fc67514a3541f3b3313e8530bf86202b30c0ae3715
---

# 契約: API のエラー理由と失敗理由コード {#contract-api-error-reasons-and-failure-reason-codes}

正本: [api/openapi.yaml](../../../api/openapi.yaml)。Go と TypeScript は `task generate` で生成する。この文書は、この機能が追加または変更するものだけを記述する。既存の `code` の値、各パスの HTTP ステータス、`state` のような機械可読な値は変わらない。

## すべての `message` を英語にする {#every-message-in-english}

`Error.message` は、ユーザーと API 利用者が読む英語の文になる。OS や外部プログラムが返した自由記述の文は含めない。ユーザーデータ (パス、名前) を含むとき、そのデータは翻訳せず、周りの説明だけを英語にする。`description` は「英語の説明。画面は `code` と `reason` から表示する。この文は API 利用者と未知のコードのための代替である。」になる。

`internal/httpapi/spa.go` のプレーンテキストの失敗応答 (`http.Error`) と、`writeJSON` が応答を作れないときに使う固定の JSON も英語になる。

## `Error` の `reason`、`limit`、`tagName` {#reason-limit-and-tagname-on-error}

`Error` に次の 3 つの省略可能なフィールドを加える (`additionalProperties: false` は維持する)。

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `reason` | 文字列の enum (`ErrorReason`) | 同じ `code` の中で状況を区別する下位の理由。下の表の状況でだけ返す |
| `limit` | integer | その理由の上限。表の `limit` 列に値があるときだけ返す |
| `tagName` | string | 要求が衝突したタグの元の名前 (ユーザーデータで、翻訳しない)。表が `tagName` を示すときだけ返す |

`reason` のない応答は今と同じで、画面は `code` の文言を表示する。

| `code` (HTTP) | `reason` | `limit` | 状況 (現在の場所) |
| --- | --- | --- | --- |
| `tag_name_taken` (409) | `name_is_tag` | — | 送った名前が既存のタグの元の名前である。`tagName` を返す (`tags.go` の `tagNameTakenMessage`) |
| `tag_name_taken` (409) | `name_is_synonym` | — | 送った名前が別のタグの同義語である。`tagName` はそのタグの元の名前 (同上) |
| `tag_merge_required` (409) | — | — | 名前の変更先が別のタグの名前である。`tagName` を返す (`tags.go`) |
| `invalid_request` (400) | `username_length` | `domain.MaxUsernameLength` | ユーザー名の長さ (`auth.go`) |
| `invalid_request` (400) | `password_length` | `domain.MaxPasswordBytes` | パスワードの長さ (`auth.go`) |
| `invalid_request` (400) | `tag_name_empty` | — | 空のタグ名 (`domain.NormalizeTagName`。`tags.go`、`video_tags.go`、`folder_groups.go` のフォルダ名からのタグ付け) |
| `invalid_request` (400) | `tag_name_control_characters` | — | タグ名の制御文字 (同上) |
| `invalid_request` (400) | `tag_name_too_long` | `domain.TagNameMaxLength` | 長すぎるタグ名 (同上) |
| `invalid_request` (400) | `merge_same_tag` | — | 統合元と統合先が同じタグ (`tags.go`) |
| `invalid_request` (400) | `search_too_long` | `maxQueryLength` | 長すぎる検索語 (`videos.go`) |
| `invalid_request` (400) | `too_many_tag_filters` | `maxTagFilterCount` | 多すぎるタグの絞り込み (`videos.go`) |
| `invalid_request` (400) | `too_many_videos` | `maxVideoTagsIDs` | 一括操作の動画が 0 件または多すぎる (`video_tags.go`、`visibility.go`) |
| `invalid_request` (400) | `too_many_tags` | `domain.MaxTagBatch` | 一括のタグ操作、影響件数、統合のタグが 0 件または多すぎる (`tags.go`。[036 screen-api.md §1–§3](../../036-tag-admin-scale/contracts/screen-api.md#post-apitagsbatch))。外部 API はタグ名の一覧が範囲外のときに返す (`external_video_tags.go`、`domain.ExternalVideoTagsMaxNames`) |
| `invalid_request` (400) | `guest_filter_not_allowed` | — | ゲストが所有者専用の条件を使った (`videos.go`) |
| `invalid_request` (400) | `invalid_cursor` | — | 読み込み位置を解析できない (`videos.go`、`library.go`、`folders.go`) |
| `invalid_request` (400) | `invalid_folder_path` | — | フォルダの指定が不正 (`folders.go`) |
| `invalid_request` (400) | `relative_directory_path` | — | ディレクトリ選択での相対パス (`directories.go`) |
| `not_found` (404) | `video_not_found` | — | 動画が存在しないか見えない (`videos.go`、`reprobe.go`) |
| `not_found` (404) | `folder_not_found` | — | フォルダが存在しないか見えない (`folders.go`、`folder_groups.go`) |
| `not_found` (404) | `not_folder_group` | — | フォルダがグループではない (`library.go`) |
| `not_found` (404) | `no_scan` | — | 取り込みがまだ一度も実行されていない (`scans.go`) |
| `not_found` (404) | `directory_not_found` | — | 選んだディレクトリが存在しない (`directories.go`) |
| `not_found` (404) | `file_unavailable` | — | 動画ファイルを開けない (`stream.go`、`transcode.go`) |
| `conflict` (409) | `media_folders_changed` | — | メディアフォルダが別の操作で変更された (`media_folders.go`) |
| `conflict` (409) | `root_group_not_taggable` | — | 登録したフォルダのグループそのものはタグにできない (`folder_groups.go`) |
| `conflict` (409) | `folder_not_group` | — | フォルダが今はグループではない (`folder_groups.go`) |
| `conflict` (409) | `probe_info_missing` | — | シークのプレビューまたはライブ変換に必要な probe のデータがない (`seek_thumbnail.go`、`transcode.go`) |
| `conflict` (409) | `seek_preview_generating` | — | シークのプレビューを生成中 (`seek_thumbnail.go`) |
| `conflict` (409) | `transcode_unavailable` | — | ライブ変換ができない (`transcode.go`) |
| `forbidden` (403) | `cross_origin` | — | 同一オリジンでない変更 (`media_folders.go`) |
| `forbidden` (403) | `open_not_local` | — | サーバー以外の PC から開こうとした (`open.go`) |

その他の `invalid_request` と `not_found` の応答 (並び順、`attempt`、Content-Type、パスの欠落、画面が送らないその他の値) は `reason` を返さない。

`limit` はサーバーの定数から埋める。画面は独自の上限を持たない。今の日本語の `message` が埋め込んでいる衝突したタグ名は `tagName` で返し、画面はそれを送った名前と組み合わせて具体的な説明にする (要件 6 の具体性)。`domain.NormalizeTagName` の失敗は、今の文字列の代わりに理由を持つドメインのエラーになり、`internal/httpapi` がそれを `reason` に対応付ける。

## `Video.probeErrorCode` {#videoprobeerrorcode}

`Video` に省略可能な `probeErrorCode` (文字列の enum `ProbeErrorCode`) を加える。`probeState = failed` で、コードを保存している行にだけ返す。値は [data-model.md、`videos.probe_error_code`](../data-model.md#videosprobe_error_code) の表にある。`probeError` (自由記述) は今と同じく返す。どちらもゲストへの応答では省く (今の `probeError` と同じ扱い)。

画面は `probeError` を表示しない。`probeErrorCode` があるときはその説明を、ないとき (更新前の行) は英語の一般的な要約を表示する。

## `Scan.errorCode` と `Scan.errorPath` {#scanerrorcode-and-scanerrorpath}

`Scan` に省略可能なフィールドを 2 つ加える。

| フィールド | 型 | 意味 |
| --- | --- | --- |
| `errorCode` | 文字列の enum (`ScanErrorCode`) | `state = failed` でコードを保存しているときの理由。値は [data-model.md、`scans.error_code` と `scans.error_path`](../data-model.md#scanserror_code-and-scanserror_path) |
| `errorPath` | string | 理由が特定の場所に結び付くとき、その絶対パス (メディアフォルダ、またはその下の読めない場所。ユーザーデータで、翻訳しない) |

`error` (自由記述) は今と同じく返す。画面は `error` を表示せず、`errorCode` と `errorPath` から英語の説明を組み立てる。`errorCode` のない `failed` (更新前の行) は英語の一般的な要約を表示する。
