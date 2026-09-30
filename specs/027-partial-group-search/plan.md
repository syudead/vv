# Implementation Plan: ライブラリの検索で一部のメンバーだけが当たったグループは、当たった動画を1本ずつ出す

**Branch**: `feature/027-partial-group-search` | **Parent Issue**: #523

**Input**: The parent Issue. It is this feature's specification.

## Summary

`GET /api/library` の項目の作り方を変える。検索語かタグの絞り込みがあるとき、グループのメンバーの
うち当たったのが一部だけなら、グループのカードを出さずに当たったメンバーを1本ずつ動画の項目にし、
全メンバーが当たったときだけ今までどおりグループのカードを出す。`GET /api/library/ids` も同じ項目に
合わせる。

判定は `internal/store` の項目を作る SQL（`libraryItemsCTE`）の中で行い
（[research.md R-1](research.md#r-1-判定は項目を作る-sql-の段に置く)）、決め手にするのは検索語とタグだけで、
再生可否と視聴状態は今までどおり項目に掛ける（[R-2](research.md#r-2-決め手は検索語とタグだけにし再生可否は項目に今までどおり掛ける)）。
応答のスキーマは変えず、規則の差分は [contracts/library-api.md](contracts/library-api.md) に書く
（[R-3](research.md#r-3-017-の成果物は直さず現行の規則は本-feature-の契約と-architecturemd-に置く)）。
画面のコードは変えない。1本ずつ出したメンバーは `kind: video` の項目であり、ライブラリの画面は
項目の種類だけで描き分けている（`web/src/api/libraryItems.ts`、`web/src/library/LibraryPage.tsx`）。

## Technical Context

**Canonical definitions**:

- 境界・依存方向・`LibraryStore` の役割・ライブラリの一覧の今の規則: [ARCHITECTURE.md](../../ARCHITECTURE.md)
  （「Intended topology」の `GET /api/library` の段落と `LibraryStore` の項）
- 今の項目の作り方と見る人ごとの見え方: [specs/017-folder-groups/data-model.md §5〜§7](../017-folder-groups/data-model.md#5-ライブラリの項目)、
  [specs/017-folder-groups/contracts/library-api.md](../017-folder-groups/contracts/library-api.md)
- 実装: [internal/store/library_items.go](../../internal/store/library_items.go)（`libraryItemsCTE`・`ListLibrary`・
  `LibraryIDs`）、[internal/store/listing.go](../../internal/store/listing.go)（`chosenLocationsCTE`・`listSpec`）、
  [internal/httpapi/library.go](../../internal/httpapi/library.go)
- API の正本と生成物: [api/openapi.yaml](../../api/openapi.yaml)（`listLibrary`・`listLibraryIds`）、
  `task generate`（[Taskfile.yml](../../Taskfile.yml)）
- 画面の一覧の保持と項目の描き分け: [web/src/api/libraryItems.ts](../../web/src/api/libraryItems.ts)、
  [web/src/api/useVideos.ts](../../web/src/api/useVideos.ts)、[web/src/library/LibraryPage.tsx](../../web/src/library/LibraryPage.tsx)
- ブラウザの試験: [web/e2e/guest.e2e.ts](../../web/e2e/guest.e2e.ts)（`task test-e2e`。CI は `main` への push でだけ回す）

**Feature-specific context**:

- 依存・移行・表・応答のスキーマは足さず変えない。変わるのは `GET /api/library` と `GET /api/library/ids` の
  項目の作り方（SQL）と、それを説明する文書だけである。
- `web/e2e/guest.e2e.ts` の所有者の場面は、「ゲスト」の検索で「非公開だけ」（7本のうち D だけが当たる）が
  グループのカードで出ることを前提にしている（`ownerGroups`）。新しい規則では D が動画のカードで出るので、
  この前提は実装の単位で直す。e2e は `main` への push でだけ回るため、統合の後に気づくのでは遅い。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended topology」、`.golangci.yml` の depguard）: 合格。変えるのは
  `internal/store` の読み出し（`libraryItemsCTE`）だけで、`internal/domain`・`internal/httpapi` の型と
  インターフェースは変えない。
- **API の正本と生成物**（AGENTS.md、ARCHITECTURE.md）: 合格。`api/openapi.yaml` の説明文を直し、生成物は
  `task generate` で作り直す。スキーマの変更は無い。
- **見る人ごとの条件は `visibleLocationCondition` を通す**（ARCHITECTURE.md の `LibraryStore` の項、017 §7）:
  合格。「全メンバー」は今の `gm`（見せてよいメンバー）で数え、ゲストには公開のメンバーだけで判定する
  （[contracts/library-api.md §1](contracts/library-api.md#1-get-apilibrary-の項目の作り方)）。
- **制約は試験で確かめる**（core-beliefs.md）: 合格。規則は store の試験が固定し、画面での見え方は e2e が確かめる。
- **文書は変更と同じ PR で直す**（core-beliefs.md、AGENTS.md）: 合格。ARCHITECTURE.md の段落と
  `api/openapi.yaml` の説明を実装の単位で直す（R-3）。

Phase 1 のあとも判定は同じである。Complexity Tracking に載せる違反は無い。

## Project Structure

### Documentation (this feature)

```text
specs/027-partial-group-search/
├── plan.md               # This file
│                         # No spec.md — the parent Issue is the specification
├── research.md           # R-1〜R-3
└── contracts/
    └── library-api.md    # GET /api/library・/ids の項目の作り方の差分（017 §5 の 1〜2 を置き換える）
```

`data-model.md` は作らない（エンティティも表も変えない）。`quickstart.md` は作らない（store の試験と
`task test-e2e` が受け入れ条件を確かめ、feature 固有の手順は無い）。

### Source Code

**Affected boundaries**:

- `internal/store`（`library_items.go` の `libraryItemsCTE` と、その試験）
- `internal/httpapi`（試験だけ。ハンドラは変えない）
- `api/openapi.yaml`（説明文）と生成物、`ARCHITECTURE.md`
- `web/e2e`（旧規則を前提にした場面の修正と、受け入れ条件の場面の追加）

**New paths**: None

**Structure decision**: Follows the existing layout（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。

## Implementation Work

### ライブラリの一覧で、一部のメンバーだけが当たったグループを当たった動画ごとに出す

**Scope**: `libraryItemsCTE`（`internal/store/library_items.go`）を
[contracts/library-api.md §1〜§2](contracts/library-api.md#1-get-apilibrary-の項目の作り方) の規則にし、
`ListLibrary` と `LibraryIDs` の両方に効かせる。旧規則を固定している store と httpapi の試験
（`TestListLibraryFiltersPerMember`・`TestLibraryIDsIncludeAllMembersOfMatchedGroups` など）を新しい規則に
置き換える。`api/openapi.yaml` の `listLibrary`・`listLibraryIds` の説明文と `task generate`、ARCHITECTURE.md の
`GET /api/library` の段落、コードの注釈の 017 §5 への参照（[§4](contracts/library-api.md#4-直す文書と試験)）。
`web/e2e/guest.e2e.ts` の所有者の「ゲスト」検索の場面を、「公開あり」はグループのカード、「非公開だけ」は
D の動画のカードで出る形に直す（グループを選ぶ手順は、全メンバーが当たる語で検索して残す）。

**Dependencies**: None

**Acceptance**: `task check` が通る。store の試験で、fixture の `show`（ep1・ep2・ep10 の3本）について:
`ep10` の検索は ep10 が動画の項目1件で `total` が 1、グループの項目は無い（受け入れ条件 1・5）;
フォルダ名 `show` の検索はメンバー3本のグループの項目1件（受け入れ条件 2）; 2本だけに手で付けたタグで絞ると
その2本が動画の項目、フォルダ由来のタグで絞るとグループの項目（受け入れ条件 3）; 検索語もタグも無い一覧は
今の項目と同じ（受け入れ条件 4）; 一部だけが当たった状態の `WatchUnwatched` は当たったメンバーのうち未視聴だけ
（受け入れ条件 6）; `LibraryIDs` は `ep10` の検索で ep10 の id だけを返し、`show` の検索で3本を返す
（受け入れ条件 7）; ゲスト（公開は ep1・ep10）で `ep10` の検索は動画の項目、`show` の検索は2本のグループ
（受け入れ条件 8）。httpapi の試験で `GET /api/library?query=ep10` の項目の `kind` が `video`、
`GET /api/library/ids?query=ep10` の `ids` が1件。`task test-e2e` をローカルで通し、所有者の「ゲスト」検索で
「公開あり, group of 5 videos」と「ゲスト非公開D」の動画のカードが並ぶことを確かめ、その結果を PR の本文に書く。

### ライブラリ画面で1本ずつ出たメンバーの再生・すべて選択・ゲストの見え方を e2e で確かめる

**Scope**: `web/e2e` に、既存の guest fixture（「公開あり」の5本、うち4本が公開）を使って受け入れ条件 1・2・7・8 の
場面を足す。画面のコードは変えない（1本ずつ出したメンバーは `kind: video` の項目で、動画のカードと同じ）。

**Dependencies**: ライブラリの一覧で、一部のメンバーだけが当たったグループを当たった動画ごとに出す

**Acceptance**: `task test-e2e` が通り、次を確かめる。所有者が1本の題名にだけ当たる語で検索すると
`article[data-video-id]` が1枚で `article[data-group-root]` が無く、押すとその動画の `/videos/{id}` が開く
（受け入れ条件 1）; フォルダ名で検索するとグループのカードが1枚でメンバーは1本ずつ出ない（受け入れ条件 2）;
一部だけが当たった状態で「すべて選択」してタグを付けると、当たった動画の `GET /api/videos/{id}` にだけそのタグが
出る（受け入れ条件 7）; ゲストで公開メンバー1本にだけ当たる語で検索すると、その1本が動画のカードで出る
（受け入れ条件 8）。
