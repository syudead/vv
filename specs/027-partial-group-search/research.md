# Research: ライブラリの検索で一部のメンバーだけが当たったグループは、当たった動画を1本ずつ出す

親 Issue: #523。

受け継ぐ技術の決定は [docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
と [ARCHITECTURE.md](../../ARCHITECTURE.md) にある（Go の単一バイナリ、SQLite、`api/openapi.yaml` を正本に
した生成、`LibraryStore` の読み出し）。今の項目の作り方は
[specs/017-folder-groups/data-model.md §5〜§7](../017-folder-groups/data-model.md#5-ライブラリの項目) にある。
ここには、この feature が足す決定だけを書く。

## R-1: 判定は項目を作る SQL の段に置く

- **Decision**: 「全メンバーが当たったか、一部か」は `internal/store` の `libraryItemsCTE`（`items` を作る
  with 句）の中で決め、`ListLibrary` と `LibraryIDs` が同じ句を使う。規則は
  [contracts/library-api.md §1](contracts/library-api.md#1-get-apilibrary-の項目の作り方)。
- **Rationale**: `total`・並べ替え・keyset のカーソル・「すべて選択」はどれも項目に対して働く
  （017 §5 の 4〜7）。項目が決まる前にそれらを掛けることはできないので、判定は項目を作る段より前に
  無ければならず、その段は今も SQL の中にある。`LibraryIDs` が同じ句を使えば、要件 8（「すべて選択」を
  一覧の項目に合わせる）は別の実装を持たずに満たせる。
- **Alternatives considered**:
  - 1ページを取ってから Go で分ける: `total` とカーソルはグループ1件で数えられているので、分けた後の
    項目の数と食い違い、ページをまたいで重複と抜けが起こる。
  - 画面（`useVideos`）で分ける: ゲストには非公開のメンバーの本数を返さないので（017 §7）、画面は
    「全メンバー」を知らず判定できない。`total` と `GET /api/library/ids` も合わなくなる。

## R-2: 決め手は検索語とタグだけにし、再生可否は項目に今までどおり掛ける

- **Decision**: 「当たったメンバー」は、範囲と検索式（`chosen`）にタグの AND を掛けたものとする。
  再生可否（`playable`）はこの判定に入れず、視聴状態と同じく、できた項目に掛ける。グループの項目は
  メンバーのどれか1本が再生できれば残り（今の結果と同じ）、動画の項目はその動画で判定する。
- **Rationale**: 要件 6 が視聴状態と再生可否をこの判定に使わないと決めている。今の実装は再生可否を
  メンバー単位の条件に入れているが、「全メンバーが当たった」グループについては「どれか1本が
  （検索語 ∧ タグ ∧ 再生可）」と「どれか1本が再生可」は同じ集合になるので、項目に掛け直しても
  今の一覧は変わらない（要件 4）。
- **Alternatives considered**:
  - 再生可否も決め手にする: 「再生できるものだけ」を付けただけで、絞り込み無しでは1枚だった
    グループが分かれる。要件 4 と 6 に反する。
  - グループの項目に全メンバーの再生可を求める: 再生できないメンバーを1本含むグループが今は出て
    いるのに消える。要件 6 の「今までどおり」に反する。

## R-3: 017 の成果物は直さず、現行の規則は本 feature の契約と ARCHITECTURE.md に置く

- **Decision**: `specs/017-folder-groups/` の `data-model.md` と `contracts/library-api.md` は変えない。
  置き換える規則は [contracts/library-api.md](contracts/library-api.md) に書き、実装の単位で
  ARCHITECTURE.md の `GET /api/library` の段落、`api/openapi.yaml` の説明文、コードの注釈の参照を
  そちらへ向ける。
- **Rationale**: 完成した feature の成果物は履歴であり、現行の設計は実装・API スキーマ・試験と
  ARCHITECTURE.md から読む（`specs/README.md`）。017 も 013 の `list-api.md` を書き換えず、差分の文書を
  足して ARCHITECTURE.md から参照している。同じ形にすれば、どの feature で規則が変わったかが履歴に残る。
- **Alternatives considered**: 017 の §5 を書き換える。承認済みの成果物を後の feature が改訂すると、
  その文書がどの時点の判断かが分からなくなる。ARCHITECTURE.md が現行を指すので、書き換える必要も無い。
