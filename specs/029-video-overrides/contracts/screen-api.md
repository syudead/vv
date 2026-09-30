# Contract: 画面の API の表示名とサムネイルの位置

正本は `api/openapi.yaml` で、この文書は足す 2 つの経路と `Video` の差分だけを書く。決定は
[research.md R-8](../research.md#r-8-画面の-api-は動画ごとの-2-つの-put-にし空の表示名と-null-の位置が解除である)・
[R-10](../research.md#r-10-表示名の規則はタグ名の規則にそろえ上限は-200-符号位置にする)・
[R-11](../research.md#r-11-位置の検証は-internaldomain-の純粋関数が持ち誤りは-3-つに分ける)。

## 0. `Video` の差分

| 項目 | 型 | 規則 |
| --- | --- | --- |
| `title` | string（既存） | 有効な題名。表示名があればそれ、無ければ拡張子を除いたファイル名。説明を書き換える |
| `fileTitle` | string | 拡張子を除いたファイル名。所有者の応答にだけ入る |
| `displayName` | string | 表示名。設定されているときだけ、所有者の応答にだけ入る |
| `thumbnailPositionMs` | integer (int64) | 代表サムネイルの位置。設定されているときだけ、所有者の応答にだけ入る |
| `thumbnailUrl` | string（既存） | 位置が設定されているとき版が `<内容鍵の先頭>-<positionMs>` になる（[R-6](../research.md#r-6-thumbnailurl-の版に位置を含める)） |

ゲストの応答は `title` に表示名を受け取り（要件 1）、3 つの新しい項目を省く
（[specs/016-single-account-auth/contracts/guest-api.md §1](../../016-single-account-auth/contracts/guest-api.md)
の `location` と同じ扱い）。一覧・関連動画・ライブラリ項目・`RelatedVideo` の `title` も有効な題名である。

## 1. `PUT /api/videos/{id}/display-name`

`operationId: setVideoDisplayName`。所有者だけ（`security: sessionCookie`。`accessRoutes` の既定の分類）。
本文は `{ "displayName": string }`（`required`、`Content-Type: application/json`）。

| 状況 | 応答 |
| --- | --- |
| 保存した、または整えて空だったので解除した | `200` `Video`（`GET /api/videos/{id}` と同じ形。`title`・`fileTitle`・`displayName` が反映済み） |
| 制御文字を含む | `400 invalid_request` / `display_name_control_characters` |
| 200 符号位置を超える | `400 invalid_request` / `display_name_too_long`、`limit: 200` |
| 動画が無い、または登録フォルダの下に所在が無い | `404 not_found` / `video_not_found` |
| ゲスト | `401 unauthenticated`（境界が返す。要件 9） |

他の動画と同じ表示名は許す（Edge Case）。保存は最後に確定した取引が残る（同時の変更は後勝ち）。
確定後に `/api/events` の `video` がその動画で流れる。

## 2. `PUT /api/videos/{id}/thumbnail-position`

`operationId: setVideoThumbnailPosition`。所有者だけ。本文は `{ "positionMs": integer | null }`
（`required`、`null` は解除）。

| 状況 | 応答 |
| --- | --- |
| 指定の位置の画像を作って公開し、記録した | `200` `Video`（`thumbnailState: done`、`thumbnailUrl` の版が新しい、`thumbnailPositionMs`） |
| `null` で、自動の位置の画像を作り直して記録した | `200` `Video`（`thumbnailPositionMs` 無し） |
| 解析が終わっていない、または尺が分からない | `409 conflict` / `duration_unknown` |
| `positionMs < 0` または尺以上 | `400 invalid_request` / `thumbnail_position_out_of_range`、`limit` = 尺（ミリ秒） |
| 画像を作れなかった（ffmpeg が失敗、指定の位置で 0 枚） | `409 conflict` / `thumbnail_frame_unavailable`。前の画像と前の位置はそのまま |
| どの所在も開けない | `404 not_found` / `file_unavailable`（`getVideoStream` と同じ） |
| 動画が無い | `404 not_found` / `video_not_found` |
| ゲスト | `401 unauthenticated` |

応答は生成が終わってから返す（数秒かかりうる）。同じ動画への同時の指定は生成の錠で直列になり、最後に
記録した位置の画像が残る。確定後に `video` 通知が流れ、一覧のカードは新しい `thumbnailUrl` を読む。

## 3. 足す `code`・`reason`

`Error.code` には足さない（`conflict`・`invalid_request`・`not_found` を使う）。`ErrorReason` に
`display_name_control_characters`・`display_name_too_long`・`duration_unknown`・
`thumbnail_position_out_of_range`・`thumbnail_frame_unavailable` を足し、`web/src/i18n/errors.ts` に
それぞれの英語の文言を足す。
