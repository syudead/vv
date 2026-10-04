---
source: specs/039-external-tag-admin/research.md
sourceHash: 49bb26d8e6565ed3802a7c52393d20a3a66df7b08a167ff0d976505d533ffcb5
---

# 調査: 外部 API と MCP からのタグ管理 {#research-tag-administration-from-the-external-api-and-mcp}

引き継ぐ決定: 外部 API と MCP の境界、その互換性の方針とエラーの形 ([specs/026-external-api/research.md](../026-external-api/research.md) の R-3 から R-5、R-7、R-8、[docs/how-to/external-api.md](../../docs/how-to/external-api.md))。タグのテーブル、名前の規則、統合の意味 ([specs/014-video-tags/data-model.md](../014-video-tags/data-model.md))。仮のタグ、却下した名前、同時編集を決着させる書き込みの直列化 ([specs/031-tentative-tags/research.md](../031-tentative-tags/research.md) の R-1 から R-6)。タグ管理画面のページ単位の一覧、一括操作、`sourceIds` による統合 ([specs/036-tag-admin-scale/research.md](../036-tag-admin-scale/research.md) の R-1、R-4 から R-6、R-10)。このファイルは、この機能が加える決定だけを記録する。

## R-1: `GET /api/v1/tags` は画面の一覧のパラメータを受け取り、既定でページ単位にするのは MCP ツールだけである {#r-1-get-apiv1tags-takes-the-screens-list-parameters-and-only-the-mcp-tool-pages-by-default}

**決定**: `GET /api/v1/tags` に、画面の `GET /api/tags` と同じ意味の `q`、`tentative`、`unused`、`sort`、`cursor`、`limit` を加え ([036 contracts/screen-api.md、`GET /api/tags` のパラメータ](../036-tag-admin-scale/contracts/screen-api.md#get-apitags-parameters))、`TagList` に `total`、`totalAll`、`nextCursor` を加える。`Tag` には、`createdDesc` と `createdAsc` の並べ替えが使う値 `createdAt` を加える。`limit` のない要求は従来どおりすべてのタグを返す。MCP ツール `list_tags` は、呼び出し側が `limit` を省くと 100 を入れるので、既定の応答は 1 ページになる ([contracts/external-api.md、`GET /api/v1/tags`](contracts/external-api.md#get-apiv1tags))。

| 案 | v1 の互換性 | 2,048 タグのときの引数なしのツール呼び出し | 判定 |
| --- | --- | --- | --- |
| **パラメータを加える。REST は「`limit` なしならすべてのタグ」を保つ。ツールは `limit` の既定を 100 にする** | 保たれる (フィールドとパラメータの追加) | 1 ページ、約 10 KB | 採用 |
| パラメータを加え、REST の既定もページにする | 壊れる: `.items` を一覧全体として読むスクリプトが 100 タグしか得ない | 1 ページ | 不採用: 互換性の方針は既定の変更を `v2` に置く |
| パラメータを加える。ツールは REST の既定を保つ | 保たれる | すべてのタグ、約 147 KB | 不採用: 親 Issue の背景がこの応答を読めないものとして挙げており、受け入れ条件は応答が収まることを求める |
| 新しい操作 `GET /api/v1/tags/page` | 保たれる | エージェントがどちらのツールを選ぶかによる | 不採用: 形の違う 2 つの一覧操作ができ、エージェントはどちらがページ単位かを知る必要がある |

**理由**: ストアはすでに画面の `TagListQuery` に答えているので、外部の操作とツールは 1 つの問い合わせの形と 1 つのカーソル形式を再利用し、受け入れ条件「検索語に合う仮のタグをすべてのページにわたって読む」は画面が実行するのと同じコードで成り立つ。ツールは親 Issue が読めないと名指しした唯一の面であり、ツールの中の既定値はどの REST の応答も変えないので、`v1` は追加だけという方針は保たれる。

## R-2: タグの操作は、本文でタグを指定する固定の `POST` ルートである {#r-2-tag-operations-are-literal-post-routes-that-name-the-tag-in-the-body}

**決定**: 新しい操作は `POST /api/v1/tags/merge`、`/tags/rename`、`/tags/synonyms`、`/tags/batch` と、`GET` と `DELETE` を受ける `/tags/rejected-names` であり、どの本文もタグの id (`id`、`targetId`、`sourceIds`、`ids`) を持つ。パスに id を置く操作はない ([contracts/external-api.md](contracts/external-api.md))。

| 案 | MCP の入力 | 判定 |
| --- | --- | --- |
| **固定のルート、id は本文** | 既存のどのツールとも同じく、生成された要求の型 | 採用 |
| 画面に合わせる: `PATCH /tags/{id}`、`POST /tags/{id}/merge`、… | ツールごとに、パスの id と本文を結合する手書きの型 | 不採用: 026 の契約はツールの入力を操作の本文だとしており、その結合の唯一の写しをつなぎ込みのコードが持つことになる |

**理由**: 既存の外部操作はどれも対象を本文で指定する (`VideoRef`、`items[].video`) ので、1 つの慣習が API 全体を覆い、生成されたサーバーにはパスパラメータを持つ操作が 1 つもない状態が続く。タグ 1 つに働く操作は `rename` と `synonyms` だけであり、ライブラリを整理するエージェントはこれらをリソースの編集ではなく呼び出しとして送る。

## R-3: 確定、却下、削除は `POST /api/v1/tags/batch` だけを通る {#r-3-confirm-reject-and-delete-go-only-through-post-apiv1tagsbatch}

**決定**: タグ 1 つ用の確定、却下、削除の操作はない。一括操作は `action` と `ids` (1 から 20,000) を受け取り、画面の `POST /api/tags/batch` と同じく `appliedIds`、`notFoundIds`、`notApplicableIds` を返す ([036 contracts/screen-api.md、`POST /api/tags/batch`](../036-tag-admin-scale/contracts/screen-api.md#post-apitagsbatch))。タグ 1 つなら `ids: [id]` である。

| 案 | `reject` に送られた確定したタグ | 判定 |
| --- | --- | --- |
| **一括操作だけ** | `200`、id は `notApplicableIds`、何も変わらない | 採用 |
| 一括操作に加えて画面の `POST /tags/{id}/confirm`、`/reject`、`DELETE /tags/{id}` | 単一のルートでは `409 tag_not_tentative`、一括操作では `notApplicableIds` | 不採用: 1 つの状況に 2 つの答えがあり、要件 4 から 6 と境界ケースは一括操作の答えだけを記述している |

**理由**: 親 Issue の境界ケース (「却下に送られた確定したタグ、削除に送られた仮のタグ: そのままにして、処理しなかったものとして返す」) は一括操作の結果そのものであり、要件 6 は 3 つの操作が 1 回の呼び出しで複数のタグを受け取ることを求める。ツールが 6 つ減ることで、MCP のツールの一覧が読みやすく保たれる。

## R-4: 名前の衝突には、衝突したタグの `tagId` と `tagName` を付けた `409 conflict` で答える {#r-4-name-conflicts-answer-409-conflict-with-the-conflicting-tags-tagid-and-tagname}

**決定**: 名前の変更と同義語の追加は、名前が別のタグのものなら理由 `tag_name_taken` の `409 conflict` を返し、名前が別のタグの元の名前で、`mergeTagId` がそのタグを指していないなら `tag_merge_required` を返す。外部の `Error` に `tagId` と `tagName` を加え、この 2 つの理由のときだけ含める。これで呼び出し側は一覧を読み直さずに `mergeTagId` を送れる ([contracts/external-api.md、`POST /api/v1/tags/rename` と `POST /api/v1/tags/synonyms`](contracts/external-api.md#post-apiv1tagsrename))。

| 案 | 判定 |
| --- | --- |
| **`mergeTagId` は呼び出し側が統合を受け入れるタグを指し、エラーはそのタグの id を持つ** | 採用 |
| 真偽値の `merge: true` | 不採用: エラーと再試行の間に別のクライアントが名前を別のタグへ移すことがあり、真偽値は呼び出し側が見ていないタグを統合してしまう ([014 contracts/tags-api.md、タグの管理](../014-video-tags/contracts/tags-api.md#tag-management)) |
| 名前が別のタグの元の名前なら、尋ねずに統合する | 不採用: 要件 3 は、呼び出し側が指示したときだけ統合することを求める |
| 画面のエラーと同じく `tagName` だけ | 不採用: 画面は一覧を読み直して id を見つける。エージェントなら衝突のたびにページを 1 つ読むことになる |

**理由**: 画面の受け入れの規則をそのまま再利用するので、API で追加した同義語と画面で追加した同義語は同じ状態を生む (要件 9)。唯一の追加であるエラーの中の id は、エージェントが答えに基づいて動くのに必要なものである。`merge_same_tag` (`targetId` と等しい `sourceIds` の要素) は、画面と同じく `400 invalid_request` のままである。

## R-5: どの操作も JSON 本文を返すので、どのツールにも結果がある {#r-5-every-operation-returns-a-json-body-so-every-tool-has-a-result}

**決定**: 同義語の削除は変更後のタグを返し、却下した名前の消去は `{ name, removed }` を返す。`204` で答える新しい操作はない。`TagStore.RemoveSynonym` はトランザクションの中で読んだタグを返し、`TagStore.ForgetRejectedTagName` は行を削除したかどうかを返す ([plan.md "Source Code"](plan.md#source-code))。

| 案 | 判定 |
| --- | --- |
| **どの操作も本文付きの `200`** | 採用 |
| 画面と同じく `204` で答え、MCP のつなぎ込みに空の `2xx` を `{}` に変えさせる | 不採用: ツールの結果は何が変わったかを何も伝えず、つなぎ込みに 2 つの操作しか使わない分岐が増える |

**理由**: MCP のつなぎ込み (`mcpTools.call`) は操作の JSON 本文をツールの結果とし、本文が空だと失敗する。既存の外部操作はどれも本文を返す。同義語を削除するエージェントはタグに残った同義語を知りたく、名前を消すエージェントはその名前が一覧にあったかどうかを知りたい。

## R-6: 外部のハンドラは画面と同じ `Tags` のメソッドを呼び、新しい書き込み経路はない {#r-6-the-external-handlers-call-the-same-tags-methods-as-the-screen-with-no-new-write-path}

**決定**: `internal/httpapi` の新しいハンドラはそれぞれ、画面のハンドラが呼ぶ既存の `TagStore` の操作 (`ListTags`、`MergeTags`、`RenameTag`、`AddSynonym`、`RemoveSynonym`、`BatchTags`、`ListRejectedTagNames`、`ForgetRejectedTagName`) を `router.go` の `Tags` インターフェースを通じて呼ぶ。`internal/app` に操作は加えず、この機能のための SQL も加えない。

| 案 | 判定 |
| --- | --- |
| **同じインターフェースを通じて画面のストアの操作を再利用する** | 採用 |
| 画面と外部 API の両方が呼ぶユースケースを `internal/app` に置く | 不採用: 画面のタグのハンドラは、各操作が 1 つのトランザクションなので、すでにストアを直接呼んでいる ([014 contracts/tags-api.md](../014-video-tags/contracts/tags-api.md))。2 つ目の呼び出し側のために新しい層を作っても、画面の呼び出しを移すだけになる |
| 外部 API 専用のストアの操作 | 不採用: 要件 9 と受け入れ条件 5 は 2 つの面が同じ状態を生むことを求めるが、2 つのコード経路ではそれをテストでしか約束できない |

**理由**: 操作ごとに 1 つのトランザクションを両方の面で共有すれば、「結果は画面からの操作と同じ」が構造上成り立つ。また、SQLite が書き込みトランザクションを直列化するので、画面と API からの同時編集は親 Issue の境界ケースが求めるとおりに決着する ([031 R-6](../031-tentative-tags/research.md#r-6-rejection-and-a-tentative-attach-of-the-same-name-rely-on-sqlite-write-serialization))。
