# Contract: フォルダ閲覧 API

正本は [api/openapi.yaml](../../../api/openapi.yaml) であり、実装単位
「フォルダ閲覧 API」がこの差分をそこへ移す。ここに書くのは本機能が追加する
経路・型・誤りの意味だけで、既存の `Video`・`VideoPage`・`VideoSort`・`Error`
の定義は変えない。

## 識別子: 登録フォルダの id と相対パス

フォルダは **登録済みメディアフォルダの id（`rootId`）と、そこからの相対パス
（`path`）** の組で指す。絶対パスは応答に載せるが、要求では受け取らない。

- `path` は `/` 区切りの相対パスで、空文字は登録フォルダそのものを指す。
- 空の区切り（`a//b`）、先頭・末尾の `/`、`.`・`..` の段、NUL を含む `path` は
  400 `invalid_request` とする。
- サーバーは `path` を OS の区切りへ直して（`filepath.FromSlash`）登録フォルダの
  パスへ連結する。連結結果が登録フォルダの外に出ることは上の検証で起きない。

## 「フォルダが存在する」の定義

次のどちらかを満たすとき、`(rootId, path)` は存在する（親 Issue 要件 11）。

- `path` が空で、`rootId` が登録済みである（動画が無くても存在する）。
- `path` が空でなく、その配下（深さを問わない）に、登録フォルダの下にある
  動画の所在（`video_locations`）が1件以上ある。

ディスクを見に行かない。取り込み済みの索引だけから導く。

## 型

### FolderSummary

フォルダカード1枚分。最上位の一覧と、あるフォルダの子フォルダの一覧で共通に使う。

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `rootId` | int64 | 登録フォルダの id |
| `path` | string | 登録フォルダからの相対パス。登録フォルダ自身は空文字 |
| `name` | string | 表示名。`path` の最後の段、`path` が空なら登録フォルダの絶対パスの最後の段（最後の段が無い `/` のような登録フォルダは絶対パスそのもの） |
| `rootPath` | string | 登録フォルダの絶対パス（同名の登録フォルダを区別する。親 Issue 要件 2） |
| `videoCount` | int | **直下**の動画の件数（孫以降を含めない。要件 5） |
| `folderCount` | int | **直下**の子フォルダの件数 |
| `previews` | FolderPreview[] | 直下の動画のうちサムネイル生成済みのもの、最大4件（要件 4） |

### FolderPreview

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `videoId` | int64 | 動画の id |
| `thumbnailUrl` | string | 既存の `Video.thumbnailUrl` と同じ版付き URL |

`previews` は直下の動画のうち `thumbnailState = done` のものだけから、所在の
パスの昇順で最大4件を選ぶ。1件も無ければ空配列（Edge Case「サムネイル未生成」）。

### FolderListing

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `folder` | FolderSummary | 開いているフォルダ自身。パンくずの名前もここから作る |
| `folders` | FolderSummary[] | 直下の子フォルダ。名前の自然順（下記） |

### RootFolderListing

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `folders` | FolderSummary[] | 登録済みメディアフォルダすべて。動画が無くても含む |

## 並び順

- 子フォルダと最上位のフォルダは `name` の**自然順**で並べる。数字の連続は数値として
  比べ（`2` < `10`）、それ以外は大文字小文字を区別せずに比べ、同順位は元の文字列で、
  最後に `rootPath` で決着させる（要件 7）。並びはサーバーが決め、クライアントは
  並べ替えない。
- 動画は既存の `VideoSort`（`addedDesc`・`titleAsc`）に従う。`titleAsc` の題名は
  そのフォルダにある所在の題名である。

## 経路

### `GET /api/folders`

最上位。登録済みメディアフォルダの `RootFolderListing` を返す。

- 200: `RootFolderListing`。登録が0件なら `folders` は空配列。

### `GET /api/folders/{rootId}`

- query `path`（string, 省略時は空文字）
- 200: `FolderListing`
- 400 `invalid_request`: `path` が上の規則に反する
- 404 `not_found`: `rootId` が登録されていない、または `(rootId, path)` が存在しない
  （Edge Case「開いているフォルダが無くなる」。クライアントは空の格子ではなく
  「見つかりません」を出す）

### `GET /api/folders/{rootId}/videos`

直下の動画だけを、既存の一覧と同じカーソル方式で返す（要件 3・6・12）。

- query `path`（同上）、`sort`・`cursor`・`limit`（既存 `listVideos` と同じ意味と範囲）
- 200: 既存の `VideoPage`。`total` は直下の動画の件数（=`videoCount`）
- 400 `invalid_request`: `path` の不正、並び順の不正、カーソルが解釈できない
- 404 `not_found`: `GET /api/folders/{rootId}` と同じ条件

`items` の `Video` は既存の形のままで、次の2点だけが一覧と違う。

- `title`・`sizeBytes` は、そのフォルダにある所在のもの（Edge Case「同じ内容の動画が
  複数の場所にある」）。`id`・サムネイル・再生位置は動画に属するので、どのフォルダから
  見ても同じ。
- 同じ動画の所在が同じフォルダに2つ以上あるとき、`Video` は1件だけ返し、題名は
  パスの昇順で先の所在から取る。

## 変更しないもの

- `GET /api/videos` と `GET /api/videos/{id}` の意味と応答。
- 再生・サムネイル・再生位置の経路。フォルダ画面の動画カードは既存の
  `/videos/{id}` へ遷移する。
- 新しい `Error.code` は足さない。
