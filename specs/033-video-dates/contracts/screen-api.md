# Contract: 画面の API の差分

親 Issue: #630。正本は [api/openapi.yaml](../../../api/openapi.yaml) で、ここには足す項目と値だけを書く。
`task generate` で `internal/httpapi/gen/` と `web/src/api/gen/` を作り直す。

## 0. `Video` に足す項目

```yaml
Video:
  required: [..., addedAt, updatedAt, fileCreatedAt, ...]
  properties:
    updatedAt:
      type: string
      format: date-time
      description: |
        vv 上で動画の情報（表示名・タグ・公開設定・代表サムネイル）を最後に編集した日時。
        一度も編集していなければ addedAt と同じ（specs/033-video-dates/data-model.md §3）
    fileCreatedAt:
      type: string
      format: date-time
      description: |
        一覧に出す所在のファイルの作成日時。ファイルシステムから取れないときはそのファイルの
        更新日時（mtime）（specs/033-video-dates/research.md R-4）
```

- 動画を返すすべての応答（一覧、`GET /api/videos/{id}`、関連、バージョン、`retry` の probe、
  `PUT /api/videos/{id}/display-name`・`thumbnail-position` の応答）に入る。
- ゲストの応答にも入る（[R-7](../research.md#r-7-応答の項目は-updatedatvv-上の更新日時と-filecreatedat所在の作成日時で画面と外部連携で同じ名前にする)）。
  `guest-api.md` の省く項目の一覧は変わらない。
- `LibraryGroup`（グループのカード）には足さない（対象外）。

## 1. `VideoSort` に足す値

```yaml
VideoSort:
  enum: [..., modifiedAsc, modifiedDesc, createdAsc, createdDesc, ...]
  description: |
    …、modified = 一覧に出す所在の更新日時（mtime）、created = 一覧に出す所在の作成日時
    （取れなければ mtime）、…
```

`GET /api/videos`、`GET /api/folders/{rootId}/videos`、`GET /api/library` の `sort` で受け付ける。
`GET /api/library/ids`（所有者だけの「すべて選択」）は `sort` を持たず並びを決めないので、変わらない。ゲストにも許す。`modifiedAsc`・`modifiedDesc` は名前も並びも変えない（要件 7）。

## 2. 変わらない経路

経路・誤りの形・イベントは足さない。タグの付け外し（`POST /api/video-tags`）と公開の切り替え
（`PUT /api/video-visibility`）の応答は今のまま。再生画面はそれらの成功後に `GET /api/videos/{id}` で
取り直す（[R-8](../research.md#r-8-再生画面はタグと公開の設定を変えたあと動画を取り直し新しいドメインイベントは足さない)）。

## 3. `web/src/api` の差分

- `videoSorts` に `createdAsc`・`createdDesc` を足す（`isVideoSort` がそれを受け付ける）。
- 画面の一覧の条件（`listCriteria` の URL の往復と `viewPreferences` の端末の設定）は、`videoSorts` のうち
  `sortKinds` に種類があるもの（`isListSort`）だけを受け付け、無い値は解釈できない sort と同じく端末の設定や
  既定に戻す。メニューに無い値を受け付けると、並べ替えの表示がランダムになり向きも切り替えられないからである。
  `created*` は「一覧の並び順に「作成日」を足す」で `sortKinds` に `created` を足したときに受け付けられる。
- 生成された `Video` 型で `updatedAt`・`fileCreatedAt` が必須になるので、Vitest の fixture を追従させる。
