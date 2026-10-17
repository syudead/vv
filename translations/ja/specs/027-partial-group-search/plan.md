---
source: specs/027-partial-group-search/plan.md
sourceHash: 1d6e53cf49f77b4244d7cddb03228f7f57030b89ac04e007509395b36e17de39
---

# 実装計画: ライブラリの検索がグループの一部のメンバーだけに一致するとき、一致した各動画を示す {#implementation-plan-show-each-matching-video-when-a-library-search-matches-only-some-members-of-a-group}

**ブランチ**: `feature/027-partial-group-search` | **親 Issue**: #523

**入力**: 親 Issue。これがこの機能の仕様である。

## 概要 {#summary}

`GET /api/library` が項目を組み立てる方法を変える。検索語かタグの絞り込みがあり、グループの一部のメンバーだけが一致するとき、グループのカードは示さず、一致した各メンバーがそれぞれ動画の項目になる。すべてのメンバーが一致するときだけ、これまでどおりグループのカードを示す。`GET /api/library/ids` も同じ項目に従う。

この判定は、`internal/store` で項目を組み立てる SQL (`libraryItemsCTE`) の中で行う ([research.md R-1](research.md#r-1-the-decision-is-made-in-the-sql-stage-that-builds-items))。判定に使うのは検索語とタグだけで、再生できるかどうかと視聴状態はこれまでどおり項目に適用する ([R-2](research.md#r-2-only-the-search-query-and-tags-decide-playability-still-applies-to-items-as-before))。応答のスキーマは変わらず、規則の差分は [contracts/library-api.md](contracts/library-api.md) にある ([R-3](research.md#r-3-017s-artifacts-stay-unchanged-the-current-rule-lives-in-this-features-contract-and-architecturemd))。画面のコードは変わらない。単独で示すメンバーは `kind: video` の項目であり、ライブラリの画面は項目の種類だけで描画の仕方を選ぶ (`web/src/api/libraryItems.ts`、`web/src/library/LibraryPage.tsx`)。

## 技術的な文脈 {#technical-context}

**正本の定義**:

| 項目 | 出典 |
| --- | --- |
| 境界、依存の方向、`LibraryStore` の役割、今のライブラリの一覧の規則 | [ARCHITECTURE.md](../../ARCHITECTURE.md) ("Intended topology" の `GET /api/library` の段落と `LibraryStore` の項目) |
| 今の項目の組み立て方と、各閲覧者に見えるもの | [specs/017-folder-groups/data-model.md の Library items から Visibility per audience](../017-folder-groups/data-model.md#library-items)、[specs/017-folder-groups/contracts/library-api.md](../017-folder-groups/contracts/library-api.md) |
| 実装 | [internal/store/library_items.go](../../internal/store/library_items.go) (`libraryItemsCTE`、`ListLibrary`、`LibraryIDs`)、[internal/store/listing.go](../../internal/store/listing.go) (`chosenLocationsCTE`、`listSpec`)、[internal/httpapi/library.go](../../internal/httpapi/library.go) |
| API の正本と生成コード | [api/openapi.yaml](../../api/openapi.yaml) (`listLibrary`、`listLibraryIds`)、`task generate` ([Taskfile.yml](../../Taskfile.yml)) |
| 画面が一覧を保持し項目を描画する方法 | [web/src/api/libraryItems.ts](../../web/src/api/libraryItems.ts)、[web/src/api/useVideos.ts](../../web/src/api/useVideos.ts)、[web/src/library/LibraryPage.tsx](../../web/src/library/LibraryPage.tsx) |
| ブラウザテスト | `web/e2e/guest.e2e.ts` (削除済み) (`task test-e2e`。CI は `main` への push でだけ実行する) |

**この機能に固有の文脈**:

- 依存、マイグレーション、テーブル、応答のスキーマは追加も変更もしない。変わるのは、`GET /api/library` と `GET /api/library/ids` が項目を組み立てる方法 (SQL) と、それを説明する文書である。
- `web/e2e/guest.e2e.ts` の所有者のシナリオは、`ゲスト` の検索で `非公開だけ` (7 本の動画のうち D だけが一致する) がグループのカードとして示される (`ownerGroups`) ことを前提にしている。新しい規則では D は動画のカードとして示されるので、実装の単位がこの前提を直す。e2e は `main` への push でだけ実行されるので、統合の後に気付いたのでは遅すぎる。

## Constitution Check {#constitution-check}

- **依存の方向** (ARCHITECTURE.md の "Intended topology"、`.golangci.yml` の depguard): 合格。変わるのは `internal/store` の読み出し (`libraryItemsCTE`) だけで、`internal/domain` と `internal/httpapi` の型とインターフェースは変わらない。
- **API の正本と生成コード** (AGENTS.md、ARCHITECTURE.md): 合格。`api/openapi.yaml` の説明が変わり、生成コードは `task generate` で作り直す。スキーマは変わらない。
- **閲覧者ごとの条件は `visibleLocationCondition` を通す** (ARCHITECTURE.md の `LibraryStore` の項目、[017 data-model.md の Visibility per audience](../017-folder-groups/data-model.md#visibility-per-audience)): 合格。「すべてのメンバー」は今の `gm` (閲覧者が見てよいメンバー) について数え、ゲストには公開メンバーだけについて判定する ([contracts/library-api.md の How `GET /api/library` builds items](contracts/library-api.md#how-get-apilibrary-builds-items))。
- **制約はテストで確かめる** (core-beliefs.md): 合格。ストアのテストが規則を固定し、e2e が画面での見え方を確かめる。
- **文書はコードと同じ PR で変える** (core-beliefs.md、AGENTS.md): 合格。実装の単位が ARCHITECTURE.md の段落と `api/openapi.yaml` の説明を更新する (R-3)。

判定はフェーズ 1 の後も同じである。複雑度の追跡に挙げる違反はない。

## プロジェクトの構成 {#project-structure}

### 文書 (この機能) {#documentation-this-feature}

```text
specs/027-partial-group-search/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1 to R-3
└── contracts/
    └── library-api.md    # Delta in how GET /api/library and /ids build items (replaces items 1–2 of 017 data-model.md "Library items")
```

`data-model.md` はない (実体やテーブルの変更がない)。`quickstart.md` はない (ストアのテストと `task test-e2e` が受け入れ条件を確かめ、この機能に固有の手作業の手順はない)。

### ソースコード {#source-code}

**影響する境界**:

- `internal/store` (`library_items.go` の `libraryItemsCTE` とそのテスト)
- `internal/httpapi` (テストだけ。ハンドラーは変わらない)
- `api/openapi.yaml` (説明) と生成コード、`ARCHITECTURE.md`
- `web/e2e` (古い規則を前提にしたシナリオを直し、受け入れ条件のシナリオを足す)

**新しいパス**: なし

**構成の決定**: 既存の配置に従う ([ARCHITECTURE.md](../../ARCHITECTURE.md))。

## 実装作業 {#implementation-work}

### ライブラリの検索が一部のメンバーだけに一致するとき、グループの代わりに一致した各動画を示す {#show-each-matched-video-instead-of-the-group-when-a-library-search-matches-only-some-members}

**範囲**: `libraryItemsCTE` (`internal/store/library_items.go`) を [contracts/library-api.md の How `GET /api/library` builds items から `GET /api/library/ids`](contracts/library-api.md#how-get-apilibrary-builds-items) の規則に従わせ、`ListLibrary` と `LibraryIDs` の両方に効かせる。古い規則を固定するストアと httpapi のテスト (`TestListLibraryFiltersPerMember`、`TestLibraryIDsIncludeAllMembersOfMatchedGroups` など) を新しい規則で置き換える。`api/openapi.yaml` の `listLibrary` と `listLibraryIds` の説明と `task generate`、ARCHITECTURE.md の `GET /api/library` の段落、コードのコメントにある [017 data-model.md の Library items](../017-folder-groups/data-model.md#library-items) への参照 ([Documents and tests to update](contracts/library-api.md#documents-and-tests-to-update))。`web/e2e/guest.e2e.ts` の所有者の `ゲスト` の検索のシナリオを、`公開あり` がグループのカードとして、`非公開だけ` が D の動画のカードとして示されるように直す (すべてのメンバーに一致する語で検索してグループを選ぶ手順は残す)。

**依存**: なし

**受け入れ**: `task check` が通る。フィクスチャ `show` (ep1、ep2、ep10) に対するストアのテスト:

| ケース | 期待 | 受け入れ条件 |
| --- | --- | --- |
| `ep10` を検索 | ep10 の動画の項目が 1 つ。`total` は 1。グループの項目はない | 1、5 |
| フォルダ名 `show` を検索 | メンバー 3 本のグループの項目が 1 つ | 2 |
| そのうち 2 本だけに手で付けたタグで絞り込む | その 2 本が動画の項目になる | 3 |
| フォルダ由来のタグで絞り込む | グループの項目 | 3 |
| 検索語もタグもない | 今と同じ項目 | 4 |
| 一部のメンバーだけが一致するときの `WatchUnwatched` | 一致したメンバーのうち未視聴のものだけ | 6 |
| `ep10` の検索 / `show` の検索での `LibraryIDs` | ep10 の id だけ / 3 本すべて | 7 |
| ゲスト (公開: ep1 と ep10)、`ep10` の検索 / `show` の検索 | 動画の項目 / 2 本のグループ | 8 |

httpapi のテストでは、`GET /api/library?query=ep10` の項目の `kind` が `video` であり、`GET /api/library/ids?query=ep10` が `ids` に 1 つの id を返す。ローカルで `task test-e2e` を実行し、所有者の `ゲスト` の検索で `ゲスト非公開D` の動画のカードの隣に `公開あり, group of 5 videos` のカードが示されることを確かめ、結果を PR 本文に書く。

### 単独で示したメンバーの再生、"Select all"、ゲストの表示を e2e で確かめる {#check-playback-select-all-and-the-guest-view-of-members-shown-on-their-own-with-e2e}

**範囲**: 既存のゲストのフィクスチャ (`公開あり`、5 本の動画のうち 4 本が公開) を使い、受け入れ条件 1、2、7、8 のシナリオを `web/e2e` に足す。画面のコードは変わらない (単独で示すメンバーは、動画のカードと同じ `kind: video` の項目である)。

**依存**: ライブラリの検索が一部のメンバーだけに一致するとき、グループの代わりに一致した各動画を示す

**受け入れ**: `task test-e2e` が通り、次を確かめる。

| シナリオ | 期待 | 受け入れ条件 |
| --- | --- | --- |
| 所有者が 1 本の動画のタイトルだけに一致する語で検索する | `article[data-video-id]` が 1 つで `article[data-group-root]` はない。クリックするとその動画の `/videos/{id}` が開く | 1 |
| フォルダ名で検索する | グループのカードが 1 つ。メンバーは単独で示されない | 2 |
| 一部のメンバーだけが一致した状態で "Select all" してタグを付ける | タグは一致した動画の `GET /api/videos/{id}` にだけ現れる | 7 |
| ゲストが 1 本の公開メンバーだけに一致する語で検索する | その 1 本が動画のカードとして示される | 8 |
