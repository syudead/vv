---
source: specs/047-video-work-info/research.md
sourceHash: 4ab767c9612bc4fd6b3233de26bb331cfbc5c731a308693ead282fbdcaea9238
---

# 調査: 動画の作品情報 (コード、日付、人物、組織、URL、外部 ID) {#research-work-information-on-videos-code-dates-people-organizations-urls-external-ids}

親 Issue: #967。引き継ぐ決定:

| 項目 | 正本 |
| --- | --- |
| 境界、依存の方向、索引とユーザーデータの区別、コミットの後のイベント | [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| ユーザーキー (まとまりのメンバーでは `bundle:<id>`、それ以外では `content_key`)、まとまりのコピーと復元、同じパスの引き継ぎ | [specs/030-video-versions/data-model.md、ユーザーキー](../030-video-versions/data-model.md#user-key)、[ストアの操作 (`VersionStore`)](../030-video-versions/data-model.md#store-operations-versionstore)、[同じパスの内容の引き継ぎ](../030-video-versions/data-model.md#carry-over-of-content-at-the-same-path) |
| 名前の規則 (制御文字、前後の空白の除去、空、コードポイントの上限、保存する表記を畳み込まない) | [specs/014-video-tags/data-model.md、名前の規則](../014-video-tags/data-model.md#name-rules)、[specs/029-video-overrides/research.md R-10](../029-video-overrides/research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit) |
| 検索で照合する形 (`FoldForMatch`) と、タグ名が検索ボックスに加わる方法 | [specs/013-library-search/data-model.md、`search_key` の規則](../013-library-search/data-model.md#search_key-rules)、[specs/014-video-tags/data-model.md、検索ボックスでのタグ名の照合](../014-video-tags/data-model.md#matching-tag-names-in-the-search-box) |
| `q`、`sort`、`cursor`、`limit`、`total` を持つページ単位の管理の一覧と、自然順の `sort_key` | [specs/036-tag-admin-scale/research.md R-1](../036-tag-admin-scale/research.md#r-1-the-server-pages-the-list-and-applies-search-filters-and-sort-to-every-tag)、[R-10](../036-tag-admin-scale/research.md#r-10-a-natural-order-name-key-sort_key-on-tag_names-and-rejected_tag_names-filled-by-the-startup-key-refresh) |
| 一覧の API とライブラリの URL でのタグの絞り込み | [specs/014-video-tags/contracts/tags-api.md、一覧の絞り込みとすべて選択](../014-video-tags/contracts/tags-api.md#list-filter-and-select-all)、[list-url.md](../014-video-tags/contracts/list-url.md) |
| ゲストに見えるものと、ゲストが使えない条件 | [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| 編集時刻 (`updatedAt`) と、それを進める書き込み | [specs/033-video-dates/research.md R-2](../033-video-dates/research.md#r-2-only-the-four-writes-of-video-information-advance-the-edit-time-tag-level-operations-bundles-and-ingestion-do-not) |
| 外部 API の取り決め、一括操作、固定の `POST` のルート、MCP ツール | [specs/026-external-api/contracts/external-api.md、共通の規則](../026-external-api/contracts/external-api.md#common-rules)、[specs/039-external-tag-admin/research.md R-2](../039-external-tag-admin/research.md#r-2-tag-operations-are-literal-post-routes-that-name-the-tag-in-the-body)、[R-6](../039-external-tag-admin/research.md#r-6-the-external-handlers-call-the-same-tags-methods-as-the-screen-with-no-new-write-path) |

このファイルは、この機能が加える決定だけを記録する。置き換えられた最初の計画 (6 つの文字列フィールド、ブランチ `archive/047-video-work-info-v1`) は入力ではない。この計画がその選択の 1 つを保つところでは、ここでその選択を述べ直す。

## R-1: 項目の種類ごとの型付きのテーブル。後の項目は新しいテーブルか新しい種類の値になる {#r-1-typed-tables-per-item-kind-so-a-later-item-is-a-new-table-or-a-new-kind-value}

**決定**: 作品情報は、単一の値のフィールド (`code`、`description`) の 1 行と、複数の値を持つ項目ごとの 1 つのテーブルとして保存する。各行は、決まった集合に属するテキストの値として種類か役割を持つ ([data-model.md、マイグレーション](data-model.md#migration))。後の項目 (要件 15: シリーズ、原題、…) は、追加だけのマイグレーションで加える新しいテーブルか新しい列、または既存の種類の集合の新しい値になる。既存の列の意味は変わらず、行を書き換えることもない。

| 案 | 判定 |
| --- | --- |
| **複数の値を持つ項目ごとに 1 つのテーブル、種類はテキストの値** | 採用 |
| オブジェクト全体を持つ 1 つの JSON の列 | 不採用: 検索 (R-6)、一覧の絞り込み (R-7)、人物の統合 (R-8) は行ごとに 1 人の人物か 1 つの名前を照合し、候補のすべての行に対する `json_each` は一覧のページの動画ごとに 1 回解析を実行する |
| 日付、URL、外部 ID のための 1 つの汎用の `video_work_values (kind, value)` テーブル | 不採用: 日付、URL、`(source, value)` の組は列と規則が異なり、1 つのテーブルではそれらを型のないテキストとして持ち、規則を種類に置くことになる |
| `videos` の列 | 不採用: 索引はスキャンで作り直される (要件 13) |

**理由**: 親 Issue は項目の集合と役割の集合を固定し、後の追加の後もその形が保たれることを求めている。項目の種類ごとのテーブルなら各項目が独自の列と索引を持ち、決まった集合に属する種類の値は `CHECK` 制約と生成した列挙型が持てるものなので、要件 2、5、6 の決まった集合はデータのある場所で強制される。

## R-2: ユーザーキーをキーとする動画ごとのテーブル。それぞれが独立し、まとめる経路と引き継ぎの経路がコピーする {#r-2-per-video-tables-keyed-by-the-user-key-each-standing-alone-copied-by-the-bundle-and-succession-paths}

**決定**: 動画ごとのどのテーブル (`video_work_info`、`video_work_dates`、`video_work_urls`、`video_work_external_ids`、`video_work_people`、`video_work_organizations`) も、ユーザーキーを持つ `content_key` を持ち、`videos` への外部キーも `video_work_info` への外部キーも持たず、`userDataTables` と `moveUserData` に加わる ([data-model.md、まとまりと引き継ぎ](data-model.md#bundles-and-succession))。`video_work_info` の行は、`code` か `description` が設定されている間だけ存在する。

| 案 | 判定 |
| --- | --- |
| **ユーザーキー、各テーブルは独立** | 採用 |
| `video_overrides` のような内容の鍵 | 不採用: Edge Case はまとまりごとに 1 つの単位を求めている。この情報は作品を記述し、作品はすべてのバージョンが共有する |
| ほかのテーブルが `on update cascade` で参照する親の行 (置き換えられた計画) | 不採用: そうすると `moveUserData` は、ほかのすべてのテーブルに使う文で子のテーブルのキーを付け替えられず、日付だけを持つ動画には空の親の行が必要になる |

**理由**: ユーザーキーをキーにすると、要件 13 と Edge Cases (複数の場所にある同じ内容、まとまり、移動、名前の変更) は既存の経路がすでに与える性質になる。ほかの行に依存する行がないので、どのテーブルも `video_favorites` と同じ文でコピーされ置き換えられ、日付だけや URL だけを持つ動画をまとめても何も失われない。引き継ぎは 6 つのテーブルを 1 つの単位として扱う唯一の経路である: 古いキーが作品情報を持つとき、6 つすべての新しいキーの行を置き換えるので、引き継ぎが、ある動画の人物を別の動画の日付の隣に残すことは決してない。

## R-3: 人物と組織は名前が一意でない id を持つエンティティであり、1 つの `CreditStore` が扱う {#r-3-people-and-organizations-are-id-entities-whose-names-are-not-unique-served-by-one-creditstore}

**決定**: `people` と `organizations` は id のテーブルである。その名前と別名は `people_names` と `organization_names` の行であり (エンティティごとに表記ごとに 1 行、それぞれ `canonical = 1` の行が 1 つ)、2 つのエンティティが同じ表記を持ってよい。1 つの役割の型 `CreditStore` が、両方の種類について一覧、読み取り、作成、更新、統合、削除を実装し、テーブルが名前だけで異なるところでは `domain.CreditKind` を受け取る。親組織は組織だけが持つ唯一のフィールドである ([data-model.md、エンティティ](data-model.md#entities))。

| 案 | 判定 |
| --- | --- |
| **2 つのエンティティの種類、1 つのストアの実装** | 採用 |
| `kind` の列を持つ 1 つの `credits` テーブル | 不採用: 組織は親を持つが人物は持たず、決まった役割の集合も異なる。1 つのテーブルは、行の半分にとって意味のない null を許す親を持つことになる |
| 2 つの役割の型 (`PeopleStore`、`OrganizationStore`) | 不採用: 一覧、名前、別名、外部 ID、統合、削除の SQL は両方の種類で同じなので、2 つの型は AGENTS.md が禁じるコピーになる |
| タグのような一意の名前 | 不採用: Edge Case は、同じ名前の 2 人の人物が 2 人であることを求めている |

**理由**: 要件 3、4、12 は、独自の画面、名前の変更、統合を持つエンティティを記述しており、統合や名前の変更はすべてのリンクを一度に移す必要があり、それができるのは id だけである。表記は何も識別しないので、画面と API は常に名前とともに id を運ぶ。

## R-4: 動画の書き込みは人物と組織を id で参照し、作成には独自のルートがある {#r-4-a-video-write-references-people-and-organizations-by-id-creation-has-its-own-route}

**決定**: `WorkInfoInput` は人物や組織を `id` だけで指定する。動画のページは、`PUT` を送る前に、ない人物や組織を `POST /api/people` か `POST /api/organizations` で作成する。外部の一括操作も id を参照し、クライアントは一覧のルートで名前か外部 ID から id を見つける ([contracts/screen-api.md、人物と組織](contracts/screen-api.md#people-and-organizations))。

| 案 | 判定 |
| --- | --- |
| **id だけ。別の作成のルート** | 採用 |
| 動画の書き込みに `{ id }` か `{ name }` を置き、`name` なら作成する (`TagInput` の形) | 不採用: タグ名はそのタグの同一性なので、名前で作成しても冪等である。人物の名前はそうではなく (R-3)、送り直した `PUT` が 2 人目の人物を作成してしまう |
| ちょうど 1 つのエンティティがその表記を持つとき、`{ name }` を既存のエンティティに一致させる | 不採用: その名前の 2 人目の人物が黙って 1 人目に統合され、これは Edge Case が分けておくよう求める重複である |

**理由**: 要件 8 (「既存のものを選ぶか、その場で作成する」) は画面の流れであり、書き込みの形ではない。ページからの 2 つの呼び出しは動画の書き込みを冪等に保ち、一覧画面、外部 API、ページに 1 つの作成のルートを与える。

## R-5: 作品情報のオブジェクト全体を置き換える 1 つの `PUT` {#r-5-one-put-that-replaces-the-whole-work-information-object}

**決定**: `PUT /api/videos/{id}/work-info` はオブジェクト全体を受け取って置き換える。ない、`null`、または前後の空白を除くと空になるフィールドは消え、ない一覧は空であり、すべての項目を消した動画は行を持たない。応答は `Video` である。`domain.NormalizeWorkInfo` は [data-model.md、フィールド](data-model.md#fields) の規則を適用し、フィールドのパスと問題を挙げた 1 つの `InvalidWorkInfoError` を返す ([contracts/screen-api.md、`PUT /api/videos/{id}/work-info`](contracts/screen-api.md#put-apivideosidwork-info))。

| 案 | 判定 |
| --- | --- |
| **オブジェクト全体を置き換える** | 採用 |
| 項目の種類ごとに 1 つのルート (`/work-info/dates`、`/work-info/people`、…) | 不採用: ページは 1 つのフォームを保存し、外部の操作はオブジェクト全体を送る。7 つのルートはエラーの対応付けとテストを 7 倍にする |
| マージの意味を持つ `PATCH` | 不採用: そうすると「この一覧を消す」には「そのままにする」とは別の目印が必要になり、画面はそれを必要としない |

**理由**: エディターは項目をまとめて編集し、スクレイパーはまとめて書くので、変更の単位はオブジェクトである。置き換えは冪等なので、ネットワークの失敗の後の送り直しは安全である。エディター自身はこのルートを呼ばない: 表示名をオブジェクトとともに R-14 のルートで保存する。このルートは、外部の操作の意味と、作品情報だけを書くクライアントに使われる。

## R-6: 検索ボックスは、コードと、結び付いた人物と組織のすべての名前と別名を照合する {#r-6-the-search-box-matches-the-code-and-every-name-and-alias-of-the-linked-people-and-organizations}

**決定**: 所有者の場合、検索の語は、`instr(video_work_info.code_key, term) > 0` のとき、または動画のユーザーキーに結び付いた `people_names` か `organization_names` の行が `instr(search_key, term) > 0` を満たすときに動画に一致する。3 つは `tagNameMatchCondition` が組み立てる OR の分岐である ([data-model.md、検索](data-model.md#search))。説明、日付、URL、外部 ID は照合しない。ゲストはそのどれにも一致しない (R-10)。

| 案 | 判定 |
| --- | --- |
| **各名前の行とコードにキーを置き、タグ名と同じく `instr` で照合する** | 採用 |
| コードと結び付いたすべての名前をつなげた動画ごとの 1 つの `search_key` | 不採用: 人物の名前の変更や統合で、結び付いたすべての動画のキーを書き換える必要が出る。名前の行のキーなら 1 か所で変わる |
| 値に対する FTS5 のトライグラムのテーブル | 不採用: トライグラムには 3 文字が必要であり、`AB-12` のようなコードや 2 文字の名前がよくある場合である。タグ名も同じ理由で `instr` を使う |
| 説明を含める | 不採用: 長い説明は一般的な単語に一致し、検索ボックスをメモの全文検索に変えてしまうが、要件 10 はそれを求めていない |

**理由**: 受け入れ条件 3 はコード、名前、別名を挙げている。別名は名前の行にあるので、そこで照合すれば両方を覆い、名前の行を統合先に移す統合 (R-8) は、キーを書き換えずに受け入れ条件 5 を満たしたままにする。

## R-7: id による `person` と `organization` の一覧のパラメータ。すべての役割に一致する {#r-7-person-and-organization-list-parameters-by-id-matching-every-role}

**決定**: `GET /api/videos`、`GET /api/library`、`GET /api/library/ids` は、それぞれ id を 1 つ取る `person` と `organization` を得て、ほかのすべての条件と AND をとる。どの役割であれリンクの行がその id を挙げていれば、動画は一致する ([contracts/screen-api.md、一覧のパラメータ `person` と `organization`](contracts/screen-api.md#list-parameters-person-and-organization))。ライブラリの URL は `person=<id>` と `organization=<id>` を持つ。動画のページで人物や組織を押すと `/?person=<id>` か `/?organization=<id>` が開く。`listFolderVideos` は、`tag` を取らないのと同じく、これらを取らない。ゲストは `400` を受け取る (R-10)。

| 案 | 判定 |
| --- | --- |
| **一覧の API とライブラリの URL の id のパラメータ、役割は問わない** | 採用 |
| id とともに役割 (`person=3&role=performer`) | 不採用: 要件 11 はクレジットではなく人物を挙げている。ある動画を監督し、別の動画に出演した人物の動画の一覧は 1 つである |
| 独自のページ (`/people/<id>`) | 不採用: ライブラリにはすでに URL、スナップショット、条件の仕組みがあり、絞り込みは検索ボックスやタグと組み合わさる |
| 置き換えられた計画の `performer=<name>` のような、値としての名前 | 不採用: 名前は一意でない (R-3) |

**理由**: 受け入れ条件 4 は「値を押すと、同じ人物の動画を見る」である。URL の id はまさにそれを行い、名前の変更の後も残るが、URL の名前ではそうならない。

## R-8: エンティティの編集は、省略可能なフィールドの `PATCH`、リンクを移す統合、リンクを外す削除である {#r-8-entity-edits-are-a-patch-of-optional-fields-a-merge-that-moves-links-and-a-delete-that-detaches}

**決定**: `PATCH /api/people/{id}` (と `/api/organizations/{id}`) は、`name`、`aliases`、`externalIds`、組織では `parentId` を受け取り、それぞれ省略可能である。与えた一覧は保存した一覧全体を置き換える。`POST …/{id}/merge` は `sourceIds` を受け取り、統合元のすべてのリンクの行、名前の行、外部 ID を 1 つのトランザクションで統合先に移す。統合した統合先が別名か外部 ID の上限を超えるとき、または統合元の組織が統合先の祖先であるときは、何も変えずに拒否する。`DELETE …/{id}` はエンティティとそのリンクの行を削除する。画面は確認の前に `videoCount` を示す。組織自身かその子孫の 1 つである `parentId` は拒否する。外部 ID はある種類のエンティティを最大 1 つしか指さないので、`POST` と `PATCH` は別のエンティティが持つ外部 ID を拒否する ([contracts/screen-api.md、人物と組織](contracts/screen-api.md#people-and-organizations))。

| 案 | 判定 |
| --- | --- |
| **省略可能なフィールドを持つ `PATCH`。別名は 1 つの一覧** | 採用 |
| タグ風の同義語のルート (`POST …/synonyms`、`DELETE …/synonyms?name=`) | 不採用: タグの同義語は、1 つ加えると統合が起こりうるので名前ごとのルートを必要とする。人物の別名は何とも衝突しない (R-3) ので、別名のダイアログは 1 つの一覧を保存する |
| エンティティ全体を置き換える `PUT` | 不採用: 名前の変更のダイアログが読み込んだ別名を送り直し、別のタブで行った別名の編集を上書きしてしまう |
| 結び付いた動画を持つエンティティの削除を拒否する | 不採用: Edge Case は、件数を示してリンクを外すとしている |
| 統合先の祖先である統合元を、連鎖の親を付け替えて統合する (統合先が統合元の位置に入る) | 不採用: 2 つの間の組織は、ループを避けるために独自の規則が必要になる。拒否すれば Edge Case の「循環は拒否する」が保たれ、所有者は先に統合先を移すか、逆向きに統合できる |
| 別名と外部 ID の上限を超えて統合する | 不採用: 統合先は、それらの一覧の次の `PATCH` が拒否する一覧を持つことになる |
| 2 つのエンティティが外部 ID を共有することを許し、照会ですべての持ち主を返す | 不採用: 出演者を探すスクレイパーが、どれを結び付けるべきかわからない。重複は統合すべき誤りであり、保つべき状態ではない |

**理由**: 一覧画面はタグのページ (一覧、名前の変更、統合) に従い、1 つの `PATCH` が 1 つのルートと 1 つのエラーの対応付けで名前の変更、別名の編集、親を扱う。統合は `MergeTags` をなぞるので、受け入れ条件 5 はタグの統合と同じように満たされる。

## R-9: 外部 API は 1 つの一括の置き換えと、エンティティの種類ごとの固定の `POST` のルートを得る。`field` は失敗したプロパティを挙げる {#r-9-the-external-api-gets-one-bulk-replace-and-literal-post-routes-per-entity-kind-field-names-the-failing-property}

**決定**: `POST /api/v1/video-work-info` は R-5 の意味で `{ video, workInfo }` の `items` を受け取る (1 から 20000、1 つのトランザクション、どのエラーでも何も適用しない)。`ExternalVideo` は必須の `workInfo` を得る。人物と組織は、`GET /api/v1/people` (`q`、または照会のための `source` と `externalId` を持つ)、`POST /api/v1/people`、`POST /api/v1/people/update`、`POST /api/v1/people/merge`、`POST /api/v1/people/delete` と、`/organizations` の下の同じ 5 つを得る。MCP は操作ごとに 1 つのツールを得る ([contracts/external-api.md](contracts/external-api.md))。プロパティを挙げるどのエラーも、両方のスキーマで、`people[1].id` や `aliases[0].language` のようなパスである `field` を持つ。`index` は `items` の中の位置という意味を保つ。

| 案 | 判定 |
| --- | --- |
| **1 つの一括の置き換え、種類ごとに 5 つのルート、パスとしての `field`** | 採用 |
| 動画の書き込みの中の `{ source, externalId }` の参照 | 不採用: 照会のルートが同じ参照を解決し、1 つの参照の形 (`id`) なら `WorkInfoInput` は両方のスキーマで同じままである |
| フィールドと問題ごとに 1 つの理由 (`code_too_long`、`date_invalid`、…) | 不採用: 7 つの項目の種類と 4 つの問題で 20 を超える列挙値になり、画面のカタログにはそれぞれのメッセージが必要になる |
| `replace` の隣の `merge` の動作 | 不採用: この機能が対象とする書き手はオブジェクト全体を持っている。`v1` は後でクライアントを壊さずに `action` を得られる |

**理由**: `POST /api/v1/video-display-names` と 039 のタグのルートは、外部の書き手がすでに使う形である。スクレイパーはエンティティを外部 ID で探し、なければ作成し、動画を 1 回書く。

## R-10: ゲストは応答、検索、絞り込みのどこでも作品情報を受け取らない {#r-10-guests-receive-no-work-information-in-responses-search-or-filters}

**決定**: ゲストへの応答は (`displayName` を省くのと同じく) `workInfo` を省き、ゲストの検索の条件に作品情報の分岐はなく、ゲストからの `person` や `organization` は `Audience.CheckVideoQuery` を通じて `400 guest_filter_not_allowed` を返し、人物と組織のどのルートも所有者専用である。

| 案 | 判定 |
| --- | --- |
| **タグと同じく所有者専用** | 採用 |
| 公開した動画でそれを表示し、ゲストに検索と絞り込みを許す | 不採用: ゲストの契約は、タイトル以外の所有者が入力したすべてのフィールドを除いている。所有者が公開するのは動画であり、動画についてのメモではない |

**理由**: 親 Issue のすべての要件は所有者を挙げており、ゲストの面を広げることは Issue が決めていない決定である。

## R-11: 作品情報の書き込みは `VideoWorkInfoChanged` を発行し、エンティティの編集は何も発行しない {#r-11-a-work-information-write-publishes-videoworkinfochanged-entity-edits-publish-nothing}

**決定**: `WorkInfoStore` はコミットの後に、書き込みごとに 1 つのイベントとして `VideoWorkInfoChanged{VideoIDs}` を発行し、`httpapi.Events.Handle` はそれを id ごとの `video` の通知に対応付ける。人物や組織の名前の変更、統合、削除はイベントを発行しない。開いている動画のページは、タグの名前の変更の後と同じく、次の取得で新しい名前を表示する。

| 案 | 判定 |
| --- | --- |
| **動画の書き込みに 1 つのイベント、エンティティの編集にはなし** | 採用 |
| `VideoOverrideChanged` を再利用する | 不採用: 上書きは内容ごとであり 1 つの id を運ぶ。まとまりへの書き込みはすべてのメンバーに影響する |
| エンティティの編集で、結び付いたすべての動画の id を発行する | 不採用: 多作な人物の統合は数千の id を挙げることになり、既存の画面でそれを必要とするものはない |

**理由**: 受け入れ条件 8 (外部 API で書いた値が開いているページに現れる) には動画のイベントが必要であり、ページはそれをすでに扱う。タグのページが、エンティティの管理は通知しないという前例を作っている。

## R-12: 種類ごとに 1 つの日付。`YYYY`、`YYYY-MM`、`YYYY-MM-DD` {#r-12-one-date-per-kind-in-yyyy-yyyy-mm-or-yyyy-mm-dd}

**決定**: `video_work_dates` は種類 (`release`、`recorded`、`published`) ごとに最大 1 行を持ち、`date` はテキスト `YYYY`、`YYYY-MM`、`YYYY-MM-DD` であり、日を持つときは暦の上の日付で、年は 1 から 9999 である。テキストは与えられたとおりに保存し、返す。

| 案 | 判定 |
| --- | --- |
| **精度を落とした ISO のテキスト、種類ごとに 1 つ** | 採用 |
| 完全な日付と `precision` の列 | 不採用: 検証し描画する 2 つ目の列になる。ISO のテキストはすでにその長さで精度を持っている |
| 種類ごとに複数の日付 | 不採用: 要件 2 は種類ごとに 1 つの日付としている。2 つ目の発売日は 2 つ目の発売であり、それはバージョンかシリーズ (`対象外`) である |

**理由**: スクレイパーも所有者も年だけの日付を持っており (要件 2)、テキストの形は SQL で解析なしに文字列として比較し並べ替えられる。

## R-13: 作品情報の書き込みは編集時刻を進め、エンティティの編集は進めない {#r-13-work-information-writes-advance-the-edit-time-entity-edits-do-not}

**決定**: 動画の作品情報を変える `PUT` か一括の項目は、表示名と同じく、そのユーザーキーについて `touchEditedAtForUserKeys` を呼ぶ。何も変えない書き込みはそれをそのままにする。人物や組織の名前の変更、統合、削除は、どの動画の編集時刻にも触れない。

| 案 | 判定 |
| --- | --- |
| **動画の書き込みは進め、エンティティの編集は進めない** | 採用 |
| エンティティの編集は、結び付いたすべての動画の編集時刻を進める | 不採用: 033 R-2 はタグの水準の操作を編集時刻の対象外にしており、人物はタグと同じ水準で編集される |

**理由**: 編集時刻は「所有者がこの動画を最後に編集したのはいつか」に答える。人物の名前の変更は人物の編集である。

## R-14: エディターは表示名と作品情報を、1 つの `PUT /api/videos/{id}/details` で 1 つのトランザクションとして保存する {#r-14-the-editor-saves-the-display-name-and-the-work-information-with-one-put-apivideosiddetails-in-one-transaction}

**決定**: 要件 8 の全画面のエディターは `PUT /api/videos/{id}/details` で保存し、その本文は `{ displayName: string, workInfo: WorkInfoInput }` で、両方とも必須である。`WorkInfoStore.SetDetails` は、表示名を内容の鍵の下に (029 の行と規則)、作品情報をユーザーキーの下に (R-2)、1 つのトランザクションで書く: 規則に反する表示名や規則に反する作品情報のフィールドはすべてをロールバックするので、タイトルが情報なしで保存されることも、その逆もない。表示名を先に確かめ、次に作品情報を `WorkInfo` スキーマの順に確かめ、最初の失敗を `field` (`displayName`、または R-9 と同じく `workInfo` の中のパス) とともに報告する。コミットの後、ストアは名前が変わったときに `VideoOverrideChanged` を、情報が変わったときに `VideoWorkInfoChanged` を発行し (R-11)、どちらかが変わったときに編集時刻を 1 回進める (R-13): 新しい名前では動画の内容の鍵を、新しい情報では、`touchEditedAtForUserKeys` が展開するとおりのユーザーキーのすべてのメンバーの内容の鍵を進める。`PUT /api/videos/{id}/display-name` と `PUT /api/videos/{id}/work-info` は残る: 前者は要件 8 が残すその場でのタイトルの編集に、後者は R-5 と R-9 の外部の意味に使われる ([contracts/screen-api.md、`PUT /api/videos/{id}/details`](contracts/screen-api.md#put-apivideosiddetails))。

| 案 | 判定 |
| --- | --- |
| **両方を受け取る 1 つのルート、1 つのトランザクション** | 採用 |
| エディターが `PUT …/display-name` を送り、その後 `PUT …/work-info` を送る | 不採用: 2 つの間の失敗や閉じたタブは、タイトルを保存し情報を保存しないまま残し、要件 8 はそれを認めない。クライアントは 2 つの要求をアトミックにできない |
| `PUT …/work-info` の `WorkInfoInput` の省略可能なプロパティとしての `displayName` | 不採用: `WorkInfoInput` は外部の一括の操作の項目であり、その操作にはタイトル用の `POST /api/v1/video-display-names` があるので、共有のスキーマは利用者の半分が送ってはならないプロパティを持つことになる。また「ないなら消す」(R-5) はタイトルには誤りである。タイトルでは、ないことは「そのままにする」を意味しなければならない |
| 所有者が書くすべてのフィールドに対してマージの意味を持つ `PATCH /api/videos/{id}` | 不採用: 今そのように書く動画のフィールドはなく、「タイトルを消す」には「そのままにする」とは別の目印が必要になり、エディターは常に両方の値を持つので、省略可能にしても得るものがない |
| `OverrideStore` に置き、`WorkInfoStore` を呼ぶ `SetDetails` | 不採用: ARCHITECTURE.md は、役割の型が別の役割の公開メソッドを呼ぶことを禁じている。表示名の書き込みはすでにパッケージの関数 `registeredContentKeysForVideoIDs`、`displayNamesOf`、`writeDisplayName`、`contentKeysForUserKeys`、`touchEditedAt`、`refreshSearchKeysForContentKeys` であり、`SetDetails` は自身のトランザクションでそれらを呼ぶ。作品情報の SQL は `WorkInfoStore` に属する |

**理由**: メンテナーは、エディターがタイトルと情報を 1 つの単位として保存するよう要件 8 を改めた。API はその画面に従い、その逆ではない。ストアはすでに表示名の書き込みをトランザクションの範囲の関数として持っているので、両方を覆う 1 つのトランザクションは、既存の `SetDisplayName` の形に R-2 の行を加えたものである。残す 2 つのルートは、その場でのタイトルの編集と外部の契約を変えずに保つ。
