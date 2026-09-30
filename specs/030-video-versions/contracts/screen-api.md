# Contract: 画面の API のバージョンと束ね

正本は `api/openapi.yaml` で、この文書は足す経路と `Video` の差分だけを書く。決定は
[research.md R-8](../research.md)・[R-9](../research.md)。誤りの形は
[specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md)、ゲストの
扱いは [specs/016-single-account-auth/contracts/guest-api.md](../../016-single-account-auth/contracts/guest-api.md)。

## 0. `Video` の差分

| 項目 | 型 | 規則 |
| --- | --- | --- |
| `versions` | `VideoVersionsRef {count, representativeId}` | 集まりのメンバーのときだけ、`GET /api/videos/{id}` の応答にだけ入る（`group` と同じ扱い）。`count` は見る人に見せてよい所在を持つバージョンの本数、`representativeId` は実効の代表の id |
| `tags`・`progress`・`public` | 既存 | 集まりのメンバーでは集まりの値（[data-model.md §3](../data-model.md)）。所有者・ゲストの省き方は変えない |

一覧（`GET /api/library`・`GET /api/videos`・`GET /api/folders/{rootId}/videos`）とフォルダの一覧には
代表以外のバージョンが出ない（[data-model.md §4](../data-model.md)）。`GET /api/library/ids` も同じ。
`GET /api/videos/{id}`・`related`・`stream`・`transcode.mp4`・`subtitles`・`thumbnail`・`seek-thumbnail`・
`preview` は代表以外のバージョンにもそのまま効く。

## 1. `GET /api/videos/{id}/versions`

`operationId: listVideoVersions`。ゲストも可（`GET /api/videos/{id}` と同じ分類）。

| 状況 | 応答 |
| --- | --- |
| 動画が見せられる | `200` `VideoVersions {representativeId, items: Video[]}`。代表が先頭、続きは題名の自然順。集まりに属さなければ `items` はその 1 本 |
| 無い、または見せられない | `404 not_found` / `video_not_found` |

`items` の各 `Video` は `GET /api/videos/{id}` と同じ形（所有者には `location`、どちらにも `folder`）で、
解像度・コーデック・`sizeBytes`・`container` を持つ。

## 2. `POST /api/video-bundles`

`operationId: bundleVideos`。所有者だけ。本文は `{ "videoIds": int64[], "representativeId": int64 }`。

| 状況 | 応答 |
| --- | --- |
| 束ねた | `200` `VideoVersions`（新しい集まり） |
| `videoIds` が 2 本未満（重複は 1 つ）、または上限（`POST /api/video-tags` と同じ）を超える | `400 invalid_request` / `too_few_videos`・`too_many_videos` |
| `representativeId` が `videoIds` に無い | `400 invalid_request` / `representative_not_selected` |
| どれかの id が無い、または登録フォルダの下に所在が無い | `404 not_found` / `video_not_found`。何も変えない |
| ゲスト | `401 unauthenticated` |

既に集まりに属する動画を含むときは、その集まりの全メンバーが新しい集まりに入る。この組が候補に
あれば消える。確定後に `/api/events` の `video` が全メンバーで流れる。

## 3. `POST /api/videos/{id}/make-representative`

`operationId: makeRepresentativeVersion`。所有者だけ。本文なし。

| 状況 | 応答 |
| --- | --- |
| 代表にした（既に代表でも同じ） | `200` `VideoVersions` |
| 集まりに属さない | `400 invalid_request` / `not_bundled` |
| 無い、または登録フォルダの下に所在が無い | `404 not_found` / `video_not_found` |

集まりのタグ・再生位置・公開の設定は変わらない（要件 7）。

## 4. `POST /api/videos/{id}/unbundle`

`operationId: unbundleVideo`。所有者だけ。本文なし。

| 状況 | 応答 |
| --- | --- |
| 外した | `200` `Video`（外した動画。`tags`・`progress`・`public` は束ねる前の自分の値、`versions` 無し） |
| 集まりに属さない | `400 invalid_request` / `not_bundled` |
| 無い、または登録フォルダの下に所在が無い | `404 not_found` / `video_not_found` |

残りが 1 本なら集まりは解け、その 1 本が集まりの値を持つ（Edge Case）。`video` が元の全メンバーで流れる。

## 5. 候補

### `GET /api/version-candidates`

`operationId: listVersionCandidates`。所有者だけ。引数なし。

`200` `VersionCandidatePage { items: VersionCandidate[], total }`。`VersionCandidate` は
`{ videos: [Video, Video], distance }`（`videos` は id の昇順、`Video` は `GET /api/videos/{id}` と同じ形）。
新しい順、上限 200 件。`total` は全件。

### `POST /api/version-candidates/dismiss`

`operationId: dismissVersionCandidate`。所有者だけ。本文は `{ "videoIds": [int64, int64] }`。

| 状況 | 応答 |
| --- | --- |
| 「違う動画」と記録した（候補に無くても記録する） | `204` |
| id が 2 つでない、または同じ | `400 invalid_request` |
| どちらかが無い、または登録フォルダの下に所在が無い | `404 not_found` / `video_not_found` |

「同じ動画」は `POST /api/video-bundles` に 2 本と代表を渡す。

## 6. `/api/events`

- `video`: 束ねる・代表を替える・外す・スキャン時の引き継ぎで、影響した全メンバーの id が流れる
  （[R-9](../research.md)）。
- `scan`: `fingerprint` の job の成否で流れる。候補の画面はこれで取り直す。新しい種類は足さない。

## 7. 変えないもの

- `api/external-v1.yaml`（[R-10](../research.md)）。`tags` と `POST /api/v1/video-tags` は集まりの値になる。
- 一覧の項目・`LibraryGroup`・`RelatedVideos` のスキーマ。
