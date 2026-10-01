# Research: 仮のタグと却下した名前

技術スタック、境界と依存方向、索引と利用者データの区分、認証の境界、タグの表と名前の規則は正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)、
[ARCHITECTURE.md](../../ARCHITECTURE.md)、
[specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
[specs/017-folder-groups/data-model.md §4](../017-folder-groups/data-model.md#4-フォルダ由来のタグ)、
[specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)）。
ここにはこの feature が足す決定だけを書く。

## R-1: 仮かどうかは `tags` の 1 列で持つ

- Decision: `tags` に `tentative integer not null default 0 check (tentative in (0, 1))` を足す
  （[data-model.md §1](data-model.md#1-マイグレーション)）。既存の行は既定値で確定したタグになる（要件 16、
  Edge Case「既存データの移行」）。確定は `tentative = 0` に書き換えるだけで、`tag_names`・`video_tags` は
  触らない（要件 8.1「付いている動画は変わらない」）。
- Rationale: 仮かどうかはタグ 1 件の状態で、タグを返す読み出し（一覧・1 件・動画のタグ・要約）はどれも
  `tags` を起点に結んでいる。列にすれば、その読み出しに 1 列足すだけで要件 5 の全応答に出る。付け外し・
  絞り込み・検索・本数は `video_tags`・`tag_names` を読むので、列を足しても今の動きのまま（要件 4）。
- Alternatives considered: 仮のタグの `id` を持つ別の表（読み出しごとに `exists` を足すことになり、
  「確定」が行の削除になって、列の書き換えより見通しが悪い。状態が 2 値で、行の有無で表す利点が無い）。
  `tag_names.canonical` に第 3 の値を足す（名前の行の種類と、タグの状態は別の事柄で、
  `tag_names_canonical_idx` の部分索引と 014 の不変条件を崩す）。

## R-2: 却下した名前は名前だけの表 `rejected_tag_names` に置き、タグの名前と同じ完全一致で引く

- Decision: 却下は「タグを消し、その元の名前を `rejected_tag_names (name primary key, created_at)` に
  入れる」の 1 つの取引で行う（[data-model.md §2](data-model.md#2-rejected_tag_names)）。照合は
  `tag_names.name` と同じ既定の BINARY で、綴りが完全に一致するときだけ当たる（要件 13）。
  `tentative` が真の作成は、名前が無く、かつこの表に無いときだけ仮のタグを作る（要件 11）。
- Rationale: 却下した名前は、タグでも動画への付与でもない。「次から作らない」だけの事実で、
  `tag_names` に置くと `tags` への外部キーと 014 の不変条件（どのタグにも元の名前が 1 行）に合わない。
  要件 14 の一覧と取り外し、要件 15 の「手で決めたら外す」は、名前を主キーにした表の
  `select`・`delete` で足りる。
- Alternatives considered: タグの行を消さず「却下」の状態を足す（要件 8.2 はタグの削除と動画からの取り外しを
  求めている。残った行は `id` での絞り込みや `GET /api/tags` に出さないための条件を全読み出しに足すことになる）。
  却下した名前を `NormalizeTagName` 以上に正規化して照合する（要件 13 が完全一致と決めている。表記揺れは
  統合で吸収する）。

## R-3: 名前を `tag_names` に書く取引は、同じ名前を `rejected_tag_names` から外す

- Decision: `tag_names` に名前の行を足す（作成、シノニム登録、名前での付与での作成、グループのタグ化）と
  改名で名前を書き換える取引は、同じ取引でその名前を `rejected_tag_names` から消す。仮の作成は、却下した
  名前をその前に飛ばしているので、この規則をそのまま通しても何も消さない
  （[data-model.md §3](data-model.md#3-書き換えの規則)）。
- Rationale: 要件 15 は「手で決めたことを優先し、却下した名前の一覧から外す」を、画面の作成・改名・
  シノニムと `tentative` が偽の API のすべてに求めている。名前を書く入口は `insertTagName` と改名の 1 文に
  集まっているので、そこに置けば経路ごとの呼び忘れが起きない。この規則で「同じ名前が `tag_names` と
  `rejected_tag_names` の両方にある」状態が生まれず、それを不変条件として検査できる。
- Alternatives considered: 各 API のハンドラで消す（`internal/httpapi` に SQL の都合が漏れ、グループの
  タグ化のような別の役割の経路を見落とす）。両方にある状態を許し、読み出しで `tag_names` を優先する
  （要件 14 の一覧に、もう名前として使われている名前が残る）。

## R-4: 仮のタグへの手入れ（改名・シノニム・統合先）は同じ取引で確定にする

- Decision: `RenameTag`（名前が変わるとき）、`AddSynonym`（名前を足すとき、承諾した統合を伴うときとも）、
  `MergeTag` の統合先は、対象が仮なら同じ取引で `tentative = 0` にする（要件 9）。
  改名で今と同じ名前を送ったときは、今の契約どおり何も変えない（仮のままにする）。仮のタグどうしの統合は、
  統合先が確定し統合元が消える（Edge Case）。「確定する」は独立の操作 `ConfirmTag` としても置く（要件 8.1）。
- Rationale: 仮のタグに名前やシノニムを与えた時点で、利用者はそのタグを使うと決めている。同じ取引で書けば
  「改名したのに仮のまま」の中間状態が無い。仮のタグがシノニムを持たない（要件 9）ことは、この規則から
  導かれ、不変条件として検査できる。
- Alternatives considered: 画面が改名のあとに `confirm` を続けて送る（2 要求の間で別のタブの却下と
  競合し、確定したはずのタグが消える。API の利用者にも同じ手順を求めることになる）。

## R-5: 「却下する」は仮のタグにだけ効き、確定したタグには `409 tag_not_tentative` で何もしない

- Decision: `RejectTag` は取引の中でタグが仮であることを確かめ、仮でなければ
  `domain.ErrTagNotTentative`（API は `409 tag_not_tentative`）を返して何も変えない。`ConfirmTag` は既に
  確定したタグにも `200` で今の状態を返す
  （[contracts/screen-api.md §2](contracts/screen-api.md#2-仮のタグの操作)）。
- Rationale: 却下はタグの削除を伴う。別のタブや API で先に確定されたタグ（Edge Case「操作の競合」）を、
  古い画面の「却下する」で消してはならない。確定したタグの削除は今の `DELETE`（名前を覚えない）にだけ
  任せる（要件 10）。確定は状態を進めるだけで、重ねても害が無いので、`tag_not_found` と同じ扱いを
  求めない。画面は `tag_not_tentative` を `tag_not_found` と同じく一覧の取り直しにする。
- Alternatives considered: 確定したタグへの「却下」を削除として受け付ける（要件 10 に反し、競合で
  名前を覚える削除が起きる）。`confirm` も `409` にする（画面は取り直すだけで、要求者に見せる違いが無い。
  API の利用者には冪等な方が扱いやすい）。

## R-6: 却下と、同じ名前の仮の付与は、SQLite の書き込みの直列化に任せる

- Decision: 却下の取引（仮の確認 → `tags` の削除 → `rejected_tag_names` への挿入）と、仮の付与の取引
  （名前を引く → 却下した名前を確かめる → 仮のタグを作る → 付ける）は、どちらも書き込みの取引として
  始める（`internal/store` の即時ロックのプール）。特別なロックや再試行は足さない。
- Rationale: 書き込みの取引は 1 つずつ走る。却下が先なら付与は却下した名前を見て飛ばし、付与が先なら
  却下はそのタグを消して名前を覚える。どちらの順でも「却下した名前なのにタグが残る」状態にならない
  （Edge Case「操作の競合」）。
- Alternatives considered: 却下の取引の後に同じ名前のタグを探して消し直す（直列化で起き得ないことへの
  対処で、読み手に競合があると誤解させる）。

## R-7: 外部連携 API は `tentative` を要求の 1 項目、飛ばした名前を応答の 1 項目として足す

- Decision: `POST /api/v1/video-tags` の本文に `tentative`（真偽値、省略時は偽）を、応答に
  `skippedTags: string[]`（整えた名前、`tags` の順、重複なし。無ければ空）を足す。`add`・`replace` で
  却下した名前を飛ばす。`remove` は名前が無ければ元から何もしないので、`tentative` は受け付けるが
  何も変えず、`skippedTags` は空にする。`Tag`・`ExternalVideoTag` に `tentative` を足す。MCP の
  `update_video_tags` は同じ本文の型から入力の形を導いているので、`tentative` は説明を足すだけで入る
  （[contracts/external-api.md](contracts/external-api.md)）。
- Rationale: 要件 1・11・12 の形をそのまま契約にする。応答の項目の追加と本文の任意の項目の追加は、
  026 の互換の方針（項目と操作の追加だけ）に収まる。`remove` で「飛ばした」と返すと、無い名前を
  外そうとした（今も何も起きない）ときと区別が付かず、利用者に意味の無い項目を読ませる。
- Alternatives considered: 却下した名前を含む要求を `400` にする（要件 11 が失敗にしないと決めている）。
  仮の付与を別の操作にする（同じ本文に 1 項目足す方が、スクレイパーの変更が最小で、`replace` との組み合わせも
  1 つの規則で済む）。

## R-8: 仮のタグだけの絞り込みと却下した名前の一覧は、画面の側で持つ

- Decision: 「仮のタグだけ」の絞り込みは、`GET /api/tags` に引数を足さず、共有の一覧
  （`web/src/api/tags.ts`）を管理画面が `tentative` で絞る。却下した名前の一覧は
  `GET /api/tags/rejected-names` で別に読み、管理画面だけが使う。
- Rationale: タグの一覧は全件を 1 回で持ち、名前の検索も画面の側で絞っている（014 の管理画面）。
  引数を足すと共有の保持が条件ごとに分かれる。却下した名前はタグではなく、候補（combobox）や
  チップには要らないので、共有の一覧に混ぜない。
- Alternatives considered: `GET /api/tags` の応答に `rejectedNames` を同居させる（タグの一覧を読む
  すべての画面が、使わない配列を受け取る。取り直しの単位も別で済む）。

## R-9: 画面の API は仮のタグに 2 つの `POST`、却下した名前に `GET` と `DELETE` を足す

- Decision: `POST /api/tags/{id}/confirm`、`POST /api/tags/{id}/reject`、`GET /api/tags/rejected-names`、
  `DELETE /api/tags/rejected-names?name=…` の 4 経路（[contracts/screen-api.md](contracts/screen-api.md)）。
  取り外しの名前はシノニムの解除と同じくクエリで渡す。
- Rationale: 確定と却下は「そのタグへの操作」で、統合・シノニムと同じ `POST /api/tags/{id}/…` の形に
  そろえる。却下は削除を伴うが `DELETE /api/tags/{id}` とは結果（名前を覚える）が違うので、別の経路に
  して混同を防ぐ。名前をパスに置かないのは 014 の決定（`/` や `%` を含む名前）のまま。
- Alternatives considered: `PATCH /api/tags/{id}` に `tentative: false` を足す（改名の要求と混ざり、
  `tentative: true` を送れる形になる。仮に戻す操作は要件に無い）。`DELETE /api/tags/{id}?reject=true`
  （同じ経路で結果が変わり、古い画面の削除が名前を覚える事故につながる）。
