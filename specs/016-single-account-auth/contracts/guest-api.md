# Contract: ゲストへの応答と公開フラグの API

親 Issue: #135。Plan: [plan.md](../plan.md)。

API の正本は [api/openapi.yaml](../../../api/openapi.yaml) で、この文書は「ゲストも」の
要求（[auth-api.md §1](auth-api.md#1-3つの扱い)）をゲストとして処理したときの応答の差と、
公開フラグを切り替える経路だけを書く。一覧のパラメータと応答の形は
[013 list-api.md](../../013-library-search/contracts/list-api.md) と
[014 tags-api.md](../../014-video-tags/contracts/tags-api.md) のまま変えず、ここに書く差だけを足す。

## 1. ゲストに見せる範囲

ゲストの応答には、[data-model.md §3](../data-model.md#3-見る人と公開の動画の条件) の公開の
動画だけが現れる。これは一覧の項目と `total`、`getVideo`、関連動画、フォルダの一覧と
その件数・プレビュー、配信・サムネイル・シークプレビュー・ホバープレビュー・ライブ変換の
すべてに当てはまる（要件 16）。

ゲストの応答から次を外す（要件 18）。

| スキーマ | ゲストでの扱い |
| --- | --- |
| `Video.location`（サーバー上の絶対パス） | 省く（任意の欄のまま） |
| `Video.progress` | 省く |
| `Video.tags` | 空の配列（必須の欄のまま） |
| `Video.probeError`（`ffprobe` の誤りの文にファイルの絶対パスが入る） | 省く |
| `FolderSummary.rootPath`（登録フォルダの絶対パス） | 省く。そのため必須から任意に変える |

`Video.folder`（登録フォルダからの相対パス）と `FolderSummary.name` は、フォルダ閲覧で
画面に出る名前なので残す。画面は、ゲストでは登録フォルダの表示名を `rootPath` からではなく
`name` から作る。

動画とフォルダの `id`・`rootId` は所有者と同じ連番のまま返す。公開の動画の `id` から
ライブラリ全体のおおよその本数が推測できることは、親 Issue の要件 16 が許容している。

## 2. ゲストに見せない動画

ゲストとして処理した `getVideo`・`getRelatedVideos`・`streamVideo`・`getVideoPreview`・
`transcodeVideo`・`getVideoThumbnail`・`getVideoSeekThumbnail` が公開でない動画を指したら、
存在しない動画と同じ応答を返す（既存の `404 not_found`「その動画はありません」、
要件 11）。本文・ヘッダー・キャッシュの指示を存在しない場合と揃える。

フォルダでも同じにする。

- `listRootFolders` は、ゲストでは公開の動画の所在を1つも含まない登録フォルダを省く。
- `getFolder`・`listFolderVideos` は、公開の動画の所在を1つも含まないフォルダを指したら、
  登録フォルダそのもの（`path` を省いた要求）を含めて、存在しないフォルダと同じ `404` にする。
  今は登録フォルダそのものを「存在する」と答えるので、ゲストが登録フォルダの `rootId` を
  数え上げられないように、この判定を足す。

## 3. ゲストが使えない条件

一覧の条件のうち、次は所有者のデータ（再生位置・タグ）に依るので、ゲストでは受け付けない。

| 条件 | ゲストでの扱い |
| --- | --- |
| `watch` が `all` 以外 | `400 invalid_request` |
| `sort` が `playedAsc`・`playedDesc` | `400 invalid_request` |
| `tag` | `400 invalid_request` |
| `query` | 題名と相対パスにだけ照合し、タグの名前とシノニムには照合しない（[014 data-model.md §7](../../014-video-tags/data-model.md#7-検索欄でのタグ名の照合) の照合を外す） |

画面は、ゲストのときこれらの選択肢を出さない。URL に残っていたときは、既定に丸めてから
要求する。

## 4. 公開フラグの切り替え

`Video` に必須の `public: boolean` を足す。`Video` を返すすべての経路が埋める。

| 経路 | 本文 | 成功 | 誤り |
| --- | --- | --- | --- |
| `PUT /api/video-visibility` | `{ videoIds, public }` | 200 `{ applied }` | 400 |

- 「所有者だけ」の経路である。詳細画面の1本も、選択バーの複数本も、この経路を使う。
- `videoIds` の数の上限・重複の扱い・`applied` の意味・1つの取引で反映することは、
  タグの付け外し（[014 tags-api.md §4](../../014-video-tags/contracts/tags-api.md#4-付与と取り外し)）と同じにする。
- 既に同じ状態の動画は誤りにしない。
- `requiresJSONBody` と `openapi_routes_test.go` の対応に足す。

## 5. 生成物のキャッシュ

サムネイル・シークプレビュー・ホバープレビューの成功の応答は、今の
`public, max-age=31536000, immutable`（`internal/httpapi/router.go` の `cacheImmutable`）をやめ、
所有者にもゲストにも `private, no-cache` と `ETag` を付ける。

- `private` で、逆プロキシなどの共有キャッシュに所有者の応答が残り、別の人へ返るのを防ぐ。
- `no-cache` で、ブラウザは使うたびにサーバーへ確かめる。ログアウト後や非公開にした後に、
  ブラウザのキャッシュから非公開の生成物が出続けない（受け入れ条件 12、Edge Case
  「非公開にした場合」）。内容が変わっていなければ `304` で返るので、帯域はほぼ増えない。
- 動画本体（`private, max-age=0, must-revalidate`）とライブ変換（`no-store`）は今のままで
  同じ性質を持つので変えない。

## 6. 公開をやめたときの配信

非公開にした動画について、ゲストとして処理中の配信・ライブ変換・プレビューの応答を
打ち切る（Edge Case「公開の動画を見ている間に非公開にした」、
[plan.md Structural Decisions 5](../plan.md#structural-decisions)）。一覧は、その後の取得から
その動画を含まない。所有者として処理中の応答は打ち切らない。
