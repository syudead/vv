# Implementation Plan: 一覧とフォルダ画面の検索を、複数語・表記揺れ・全件絞り込み・並べ替えまで揃える

**Branch**: `feature/013-library-search` | **Parent Issue**: #195

**Input**: The parent Issue. It is this feature's specification.

## Summary

ライブラリ一覧の検索を、1つの文字列の部分一致から「空白で AND・`"…"`・`-`・`OR`」の
書き方に置き換える。照合は、表記の揺れを畳んだ所在ごとの鍵に対して行う。視聴状態と
再生可否の絞り込み、7種の並べ替えは、画面での後処理をやめてサーバーの一覧の問い合わせに
入れる。同じ検索・絞り込み・並べ替えをフォルダ画面にも置き、条件はすべて URL に載せる
（親 Issue #195）。

サーバーは、ライブラリ・フォルダ直下・フォルダ配下の3つの一覧を、1つの問い合わせの
組み立てで返す。「対象の所在を選ぶ → 動画ごとに1件へまとめる → 絞る → 並べる → keyset で
区切る」という流れである。API の差分は [contracts/list-api.md](contracts/list-api.md)、
保存する鍵は [data-model.md](data-model.md)、URL は [contracts/list-url.md](contracts/list-url.md)
にある。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`task generate`）
- 今の検索と一覧: [internal/store/search.go](../../internal/store/search.go)・
  [internal/store/videos.go](../../internal/store/videos.go)（`ListVideos`・
  `registeredLocationCondition`・カーソル）・
  [internal/store/folders.go](../../internal/store/folders.go)（`ListFolderVideos`）
- スキーマ: [internal/store/migrations/](../../internal/store/migrations/)。既存ファイルは
  変えない（[scripts/migrations-immutable.sh](../../scripts/migrations-immutable.sh)）
- 自然順: `domain.CompareNatural`（[internal/domain/folder.go](../../internal/domain/folder.go)）
- Web の所有境界と視覚規則: [ARCHITECTURE.md#web-layer](../../ARCHITECTURE.md#web-layer)・
  [library-ui.md](../../docs/design-docs/library-ui.md)
- 一覧のページング・復元: [web/src/api/useVideos.ts](../../web/src/api/useVideos.ts)・
  [web/src/api/listSnapshot.ts](../../web/src/api/listSnapshot.ts)
- フォルダ画面: [specs/011-folder-browser/plan.md](../011-folder-browser/plan.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）

**Feature-specific context**:

- 規模の前提は既存と同じ1万件である。1〜2文字の語は、除外語も含めて、範囲内の所在を
  `instr` でなめる。1万件ではこれで間に合うので、そのための索引は足さない。
- 追加する依存は無い。`golang.org/x/text/unicode/norm` は既に使っている。小文字化は
  `CompareNatural` と同じ `unicode.ToLower` を使い、`cases` は足さない。
  `modernc.org/sqlite` の `RegisterDeterministicScalarFunction` を初めて使う
  （Structural Decisions 5）。
- マイグレーションを1つ足す（`00007_location_search.sql`、[data-model.md §2](data-model.md#2-全文索引の付け替え)）。
  再生位置などの利用者データには触れない。
- `ui` Issue なので、見た目と操作の配置は、この Plan のあとの design 工程で
  `ui-design.md` に決める。この Plan は、画面が守る URL と API の契約までを決める。

## 親 Issue の食い違いの扱い

要件 5 と受け入れ条件 5 は、`-` と `OR` だけを打つと「その文字を含む動画（無ければ一致
なし）」を出すとしている。一方で Edge Case は、`-` だけ・`|` だけの入力を「検索語が無いのと
同じ（全件）」としている。この Plan は、検査できる形で書かれた受け入れ条件 5 に従う。
単独の `-` と、先頭・末尾・単独の `OR` は字面の語として扱う
（[contracts/list-api.md §1](contracts/list-api.md#1-検索語の書き方)）。受け入れ条件 5 が
挙げていない `|` は Edge Case に従い、演算子にならない `|` を捨てる。空白だけの入力と
`""` だけの入力も、Edge Case のとおり全件を出す。食い違うのは「`-` だけの入力」だけで、
この扱いは保守者の判断を受けてから plan-to-issues に進み、Edge Case の記述は `issue-spec` で
親 Issue 側を直す。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。検索語の解釈、
  照合形への変換、自然順の鍵、視聴状態の定義、ランダムの値の関数は `internal/domain` の
  純粋な関数に置く。`golang.org/x/text` は depguard の禁止対象ではない。SQL の組み立てと
  SQLite への関数の登録は `internal/store` に置く。
- **API の正本**（AGENTS.md・ARCHITECTURE.md）: 合格。パラメータ・`VideoSort` の値・
  `VideoFolder` は `api/openapi.yaml` に足し、`task generate` で生成する。生成物は手で
  編集しない。
- **索引と利用者データの区別**（ARCHITECTURE.md）: 合格。足す列と FTS 表は再構築できる
  索引で、`playback_progress` は読むだけである。マイグレーションは新しいファイルとして
  足す。
- **UI の正本**（library-ui.md・`web/src/theme/tokens.test.ts`）: 合格の見込み。色と大きさは
  トークンだけを使う。構図は design 工程で決め、実装 PR に画像を添える。
- **スクロールの所有**（ARCHITECTURE.md「The shell does not take ownership of
  scrolling」）: 合格。`window` のスクロールと既存の無限スクロールの観測点を使う。
  絞り込みで全ページを読みに行く今の処理は消す。
- **サーバーと話す場所**（ARCHITECTURE.md「Web layer」）: 合格。条件の受け渡しは
  `web/src/api/` の取得関数と `useVideos` に足し、画面は `fetch` しない。

Phase 1 のあとも判定は同じで、正当化の要る違反は無い。

## Structural Decisions

1. **照合用の鍵は Go で作って所在ごとに保存し、起動時に版を見て埋め直す。** 表記の揺れを
   畳む変換（NFKC・小文字化・かなの統一）は SQLite の SQL では書けない。そこで Go が作った
   `search_key` を `video_locations` に持ち、その1列に trigram 索引を張る。既存の
   ライブラリは、起動時に `search_version` の古い行を作り直して対応する
   （[data-model.md §5](data-model.md#5-鍵を作る時点と-search_version)）。
   - 却下: 変換を SQLite の関数として登録し、トリガと FTS の中で呼ぶ案。登録していない
     接続（`sqlite3` コマンドでの調査や復旧）では、所在への書き込みがトリガで失敗する。
     相対パスが `media_folders` に依存するため、トリガが別の表を引くことにもなる。
   - 却下: goose の Go マイグレーションで一度だけ埋める案。変換の規則を変えるたびに
     マイグレーションを足すことになり、規則の版とスキーマの版が絡む。
   - 却下: 検索語だけを正規化して、今の `videos_fts`（生の `title`・`path`）を引く案。
     題名側の全角やかなの揺れを吸収できず、要件 7 を満たさない。登録フォルダのパスも
     索引に残るので、要件 8 も満たさない。
2. **検索式は FTS5 の式にせず、所在1行に対する SQL の論理式に組み立てる。** 3文字以上の
   語は `location_search_fts MATCH`、1〜2文字の語は `instr(search_key, ?)` とし、AND・OR・
   NOT は SQL の `and`・`or`・`not` で結ぶ。式全体を所在の条件として評価し、動画は
   「条件を満たす所在が1つでもあるか」で当てる（要件 9）。
   - 却下: 式全体を1つの FTS5 の `MATCH` 式に訳す案。trigram は2文字以下の語に一致せず
     （要件 6）、FTS5 は除外語だけの式（要件 3 の `-京都`）を受け付けない。
   - 却下: すべての語を `instr` で調べる案。1万件でも間に合う。ただし3文字以上の語で今
     効いている索引を捨てる理由が無い。
   - 却下: 語ごとに動画の単位で集合演算する案。受け入れ条件 10（語ごとに別の所在で
     満たしたら当てない）を満たさない。
3. **3つの一覧を1つの問い合わせの組み立てにまとめる。** ライブラリ（登録フォルダの下
   すべて）、フォルダ直下、フォルダ配下を「所在の範囲」の違いとして表す。範囲と検索式を
   満たす所在を動画ごとにパスの最小の1件へまとめ、その所在の題名・大きさ・更新日時と
   動画の属性で、絞り込み・並べ替え・keyset を掛ける
   （[contracts/list-api.md §4](contracts/list-api.md#4-一覧に出す所在と-videofolder)）。
   `ListVideos`・`ListFolderVideos`・`searchFilter` はこれで置き換える。
   - 却下: 今の `ListVideos` と `ListFolderVideos` に、それぞれ検索式・絞り込み・13 通りの
     並べ替えを足す案。同じ keyset の条件を2か所で保守することになる。フォルダ配下の
     検索という3つ目の形も増える。
   - 却下: 検索中も一覧の項目を「登録フォルダの下で最初の所在」のままにする案。当たった
     所在とは別の所在の題名と置き場所が出て、要件 19 の手がかりが食い違う。
4. **並び順は、並べ替えと向きを組にした1つの `VideoSort` の値で表す。** `addedDesc`・
   `titleAsc` を同じ名前で残し、昇順・降順の対を足す。`random` は向きを持たない
   （[contracts/list-api.md §3](contracts/list-api.md#3-videosort-の値)）。
   - 却下: `sort` と `order` の2つのパラメータに分ける案。今の URL と端末の設定にある
     `addedDesc`・`titleAsc` を読み替える表が、画面とサーバーの両方に要る。
     `random` と向きのような意味の無い組み合わせも生まれる。
5. **ランダムの並びは、`seed` と `id` から作る値での keyset にする。** 値は
   `internal/domain` の混ぜ合わせの関数で作る。`internal/store` がそれを決定的な
   スカラー関数として SQLite に登録し、`order by` とカーソルの条件で使う。`seed` は画面が
   作って URL に載せる。関数は `internal/store` のパッケージ初期化で一度だけ登録する。
   登録は以後に開く接続にだけ効き、同じ名前の二重登録は誤りになるため、`store.Open` の
   中では登録しない。
   - 却下: `order by random()` で並べ、並びをサーバーで覚える案。再読み込みや URL の共有で
     同じ並びにならない（要件 15）。サーバーに状態を持つことにもなる。
   - 却下: 同じ混ぜ合わせを SQL の算術だけで書く案。SQLite には排他的論理和の演算子が
     無く、64 ビットを超えた掛け算は実数に変わる。偏りの無い並びを SQL の中で保証
     しにくい。登録する関数は `select` の中でしか使わず、スキーマに残らないので、1 の
     却下理由（関数の無い接続での書き込み失敗）は当たらない。
6. **題名の自然順は、保存した `title_key` のバイト順で並べる。**
   （[data-model.md §4](data-model.md#4-title_key-の規則)）。
   - 却下: 自然順の照合順序を SQLite に登録して `order by title collate natural` とする案。
     keyset の比較にも同じ照合順序を付け忘れなく掛ける必要がある。鍵は 1 の埋め直しで
     一緒に作れるので、保存する方が単純である。
7. **視聴状態と再生可否の絞り込みは、サーバーの一覧の条件にする。** 画面の後処理と、
   絞り込み中に全ページを読みに行く処理は消す。視聴状態の定義は
   [data-model.md §6](data-model.md#6-視聴状態の導き方) の1つにする。一覧から再生して
   戻ったときは、復元した項目の再生位置だけを差し替え、その場で一覧から外さない
   （Edge Case「再生位置の更新と視聴状態の絞り込み」）。
   - 却下: 画面の後処理を残し、件数だけをサーバーに数えさせる案。スクロールで続きを
     読むたびに、表示される件数とページの区切りが食い違う（要件 12）。
8. **フォルダ画面の検索範囲は、API の `scope` として明示する。** 検索語があるときに
   配下すべてを探すのは画面の決めごとなので、`listFolderVideos` は `scope` と `query` を
   独立に受ける（[contracts/list-api.md §2](contracts/list-api.md#2-追加するパラメータ)）。
   最上位の画面の検索は `listVideos` を使い、置き場所の手がかりは `Video.folder` と
   登録フォルダの一覧から作る。
   - 却下: `listFolderVideos` が「`query` があれば配下すべて」と暗黙に切り替える案。
     画面の決めごとがサーバーの意味に紛れ込む。直下だけを検索したい将来の画面で、
     意味を変えることになる。
9. **一覧の条件は1つの URL の形で持ち、ライブラリとフォルダ画面が同じ部品で読み書きする。**
   URL の解釈・既定への丸め・履歴の扱い（[contracts/list-url.md](contracts/list-url.md)）と、
   検索欄・絞り込み・並べ替えの部品は、`web/src/library/` に置いてフォルダ画面から使う。
   フォルダ画面は、今も動画カードと並べ替えの選択肢を `library/` から使っている。
   - 却下: 共通の部品を新しいディレクトリに移す案。一覧の流れを持つのは `library/` で、
     フォルダ画面はそれを借りる側という今の分担で足りる。移すと、この feature と関係の
     無い import の書き換えが PR に混ざる。
   - 却下: フォルダ画面に同じ部品を複製する案。受け入れ条件 21・23 のキー操作と、
     「同じ製品の同じ部品に見えるか」という確認点を2か所で保守することになる。

## Project Structure

### Documentation (this feature)

```text
specs/013-library-search/
├── plan.md
├── data-model.md
└── contracts/
    ├── list-api.md
    └── list-url.md
```

`ui-design.md` は、この Plan のあとの design 工程で足す。判断は上の Structural Decisions で
閉じており、別に調べて決めることが無いので `research.md` は作らない。検証は各実装単位の
自動テストと e2e で行い、それ以外に手で走らせる手順が無いので `quickstart.md` も作らない。
この workflow は `tasks.md` を作らない。下の Implementation Work を子 Issue にする。

### Source Code

**Affected boundaries**:

- `internal/domain/`: 検索語の解釈、照合形、自然順の鍵、視聴状態、ランダムの値、
  `VideoQuery`・`FolderVideoQuery`・`VideoSort` の拡張
- `internal/store/`: マイグレーション、鍵の作成と埋め直し、一覧の問い合わせの組み立て、
  カーソル、SQLite への関数の登録
- `api/openapi.yaml` と生成物: パラメータ、`VideoSort`、`VideoFolder`
- `internal/httpapi/`: パラメータの受け渡し、`Video.folder` の組み立て
- `cmd/mdm/`: 起動時の鍵の埋め直し
- `web/src/api/`: 取得関数、`useVideos` の条件、`listSnapshot` の鍵
- `web/src/library/`: URL の条件、ツールバー、検索欄、書き方の手引き、一致なしの状態
- `web/src/folders/`: 検索欄と絞り込み、配下の検索結果と置き場所の手がかり
- `web/src/preferences/`: 保存できる並べ替えの値
- `web/e2e/`: 実ブラウザでの検証
- `ARCHITECTURE.md`: 全文索引の名前と一覧 API の記述

**New paths**:

- `internal/domain/search.go` と対応 test（検索語の解釈・照合形・自然順の鍵）
- `internal/store/migrations/00007_location_search.sql`
- `internal/store/listing.go` と対応 test（一覧の問い合わせの組み立て。`search.go` を置き換える）
- `web/src/library/listCriteria.ts` と対応 test（URL と条件の相互変換）
- `web/e2e/search.e2e.ts`

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。
共通の部品を置く場所は Structural Decisions 9 のとおり。

## Implementation Work

### 検索語の書き方を解釈し、表記の揺れを畳む照合形を作る

**Scope**: `internal/domain` に、検索語を式へ解釈する関数、照合形への変換（`fold`）、
題名の自然順の鍵を作る関数、`SearchKeyVersion` を足す。書き方と上限は
[contracts/list-api.md §1](contracts/list-api.md#1-検索語の書き方)、鍵は
[data-model.md §3・§4](data-model.md#3-search_key-の規則) による。まだどこからも呼ばない。

**Dependencies**: None

**Acceptance**: `go test ./internal/domain/...` で次が通る。受け入れ条件 1〜5 の各入力が
期待する式になる。`ＡＢＣ１２３`・`ｶﾀｶﾅ`・`たび`・NFD の「が」の照合形が、対応する入力の
照合形と一致する。`2話` の鍵が `10話` の鍵より、`２話` の鍵が `10話` の鍵より、`ア` の鍵が `い` の鍵より
バイト順で前に来る。既存の `TestCompareNatural` の名前と、かな・全角数字を含む名前の組に
ついて、鍵の順序が `CompareNatural(fold(a), fold(b))` と同じ向きになる。
17 個以上の語を並べた入力は、先頭 16 個の語の式になる。`task check` が通る。

### 所在ごとの照合用の鍵を保存し、既存のライブラリを起動時に埋め直す

**Scope**: マイグレーション `00007_location_search.sql` を足す
（[data-model.md §1・§2](data-model.md#1-video_locations-に足す列)）。取り込みとメディア
フォルダの追加・変更で鍵を作り、起動時に古い版の行を埋め直す処理を
`internal/store` と `cmd/mdm` に足す（[data-model.md §5](data-model.md#5-鍵を作る時点と-search_version)）。
今の `searchFilter` は、新しい索引を引くように最小限だけ書き換える。`videos_fts` を直接
調べている `internal/store/fts_test.go` を新しい索引に合わせる。`ARCHITECTURE.md` と
`docs/design-docs/tech-stack-selection.md` の `videos_fts` の記述を更新する。

**Dependencies**: 検索語の書き方を解釈し、表記の揺れを畳む照合形を作る

**Acceptance**: `internal/store` のテストで次が通る。00006 の状態で所在を入れた DB を
移行して開くと、再取り込みなしで全所在の `search_key` と `title_key` が埋まる。登録フォルダの
パスは `search_key` に入らない。メディアフォルダを差し替えると、新しい登録の下の所在の鍵が
新しい相対パスで作り直される。Down で `videos_fts` が戻る。`scripts/migrations-immutable.sh`
と `task check` が通る。

### 一覧の問い合わせを1つにまとめ、検索式・視聴状態・再生可否・フォルダ配下で絞る

**Scope**: `internal/store/listing.go` に、所在の範囲（ライブラリ・直下・配下）と検索式を
受けて動画のページと総件数を返す組み立てを置く。`ListVideos`・`ListFolderVideos`・
`searchFilter` をこれに置き換える（Structural Decisions 2・3・7）。`VideoQuery` と
`FolderVideoQuery` に検索式・視聴状態・再生可否・範囲を足す。並べ替えはこの単位では
今の2種のままとする。一覧に出す所在の選び方は
[contracts/list-api.md §4](contracts/list-api.md#4-一覧に出す所在と-videofolder) による。

**Dependencies**: 所在ごとの照合用の鍵を保存し、既存のライブラリを起動時に埋め直す

**Acceptance**: `internal/store` のテストで、受け入れ条件 1〜7・9・10 の題名と所在の組に
対して期待する動画だけが返る。受け入れ条件 17・18 の配置について、直下と配下の範囲で
期待する動画が返る。視聴状態と再生可否を指定したとき、`Total` が条件に合う全件の数になる。
ページを送る途中で別の動画の所在を足しても、1ページ目の時点で条件に合い並べ替えの値が
変わらない動画に重複と取りこぼしが無い（[contracts/list-api.md §5](contracts/list-api.md#5-カーソルと誤り)）。既存の一覧とフォルダの
テストが通る。`task check` が通る。

### 一覧の並べ替えを7種に増やし、昇順と降順を選べるようにする

**Scope**: `VideoSort` に [contracts/list-api.md §3](contracts/list-api.md#3-videosort-の値) の値を
足し、並べ替えごとの `order by` と keyset の条件、並び順の名前を含むカーソルを
`internal/store/listing.go` に足す。`internal/domain` にランダムの値の関数を置き、
`internal/store` が SQLite に登録する（Structural Decisions 4〜6）。`titleAsc` を自然順に
変える。

**Dependencies**: 一覧の問い合わせを1つにまとめ、検索式・視聴状態・再生可否・フォルダ配下で絞る

**Acceptance**: `internal/store` のテストで次が通る。13 の値すべてについて、ページを
またいで全件が1度ずつ、期待する順で返る。`titleAsc` で `2話` が `10話` より前に来る。
長さの無い動画と再生記録の無い動画が、昇順と降順のどちらでも末尾に来る。`random` は同じ
`seed` で同じ並びになり、別の `seed` で並びが変わる。途中で所在を足しても同じ動画が2度
出ない。別の並び順のカーソルは `ErrInvalidCursor` になる。`task check` が通る。

### 一覧 API で検索式・絞り込み・並べ替え・フォルダ配下の検索を受け付ける

**Scope**: `api/openapi.yaml` に [contracts/list-api.md §2〜§5](contracts/list-api.md#2-追加するパラメータ) の
差分を足し、`task generate` で生成する。`internal/httpapi` でパラメータを
`VideoQuery`・`FolderVideoQuery` に渡し、一覧の項目に `folder` を組み立てる。`web/src/api/client.ts`
の取得関数に新しい条件を渡せるようにする（画面はまだ使わない）。`ARCHITECTURE.md` の
一覧 API の記述を更新する。`specs/011-folder-browser/contracts/folders.md` の並び順・直下
だけ・`total` の記述に、この feature の契約へのリンクを添える。

**Dependencies**: 一覧の並べ替えを7種に増やし、昇順と降順を選べるようにする

**Acceptance**: `internal/httpapi` のテストで次が通る。`GET /api/videos?query=京都 -2023&watch=unwatched&sort=durationDesc`
が条件どおりの項目と `total` を返す。`GET /api/folders/{rootId}/videos?scope=subtree&query=京都`
が受け入れ条件 17 の `x`・`y` を返し、`y` の `folder.path` が `A/B` になる。無いフォルダは
`404`、未知の `sort` と `watch` は `400` になる。`task generate` を走らせ直しても差分が
出ない。`task check` が通る。

### ライブラリ一覧の条件を URL に載せ、絞り込みと並べ替えをサーバーに任せる

**Scope**: [contracts/list-url.md](contracts/list-url.md) の解釈と履歴の扱いを
`web/src/library/listCriteria.ts` に置く。`useVideos` と `listSnapshot` の鍵に条件を足し、
画面での絞り込みと全ページの読み込みを消す（Structural Decisions 7・9）。続きのページは
`id` で重複を捨ててから足す（[contracts/list-api.md §5](contracts/list-api.md#5-カーソルと誤り)）。ライブラリの
ツールバーに、並べ替えの7種・向きの切り替え・並べ直す・視聴状態・再生可否・条件を解除を
置き、件数を全件の数で出す。配置と見た目は `ui-design.md` による。端末に保存できる並べ
替えの値を増やす。

**Dependencies**: 一覧 API で検索式・絞り込み・並べ替え・フォルダ配下の検索を受け付ける。
design 工程の `ui-design.md` が feature ブランチに入っていること

**Acceptance**: Vitest で、URL の解釈（古い形式・未知の値・`seed` の補い）と履歴の増え方の
テストが通る。`web/e2e/search.e2e.ts` で、受け入れ条件 11〜15 と 22 がライブラリで確かめ
られる。「未視聴」で絞った一覧から再生して戻ると、その動画が同じ位置に残っている。
Vitest で、要求の途中で条件（検索語・視聴状態・再生可否・並べ替え・`seed`）を変えると、
遅れて届いた古い応答が捨てられる。続きのページに既に出た `id` が含まれていても、一覧に
2度出ない。`web/src/theme/tokens.test.ts` を含む `task check` と `task test-e2e` が通る。
画面が変わるので、実装 PR に幅ごとの画像と、視覚・操作・支援技術の確認を添える。

### 検索欄から検索の書き方の手引きを開けるようにする

**Scope**: 検索欄の横に、書き方（空白・`"…"`・`-`・`OR`）を例つきで示す手引きを開く操作を
置く。検索欄の部品の一部として `web/src/library/` に置き、この単位ではライブラリで
確かめる。フォルダ画面には、検索欄を置く単位がこの部品ごと持ち込む。
中身と配置は `ui-design.md` による。

**Dependencies**: ライブラリ一覧の条件を URL に載せ、絞り込みと並べ替えをサーバーに任せる

**Acceptance**: Vitest と e2e で、手引きを開くと4つの書き方が例つきで出ること、閉じても
検索語が残ること（受け入れ条件 16）が確かめられる。キーボードだけで、検索欄 → 手引き →
絞り込み → 並べ替えと向き → 結果、の順にフォーカスが移る（受け入れ条件 23）。`task check`
が通る。実装 PR に画像と、視覚・操作・支援技術の確認を添える。

### フォルダ画面と最上位の画面で検索と絞り込みを使えるようにする

**Scope**: フォルダ画面と `/folders` のツールバーに、ライブラリと同じ検索欄・絞り込み・
並べ替えを置く。検索語があるときは子フォルダのカードを出さず、配下の検索結果を1つの格子に
並べ、各動画に置き場所の手がかりを添える。検索語が無く絞り込みだけのときは、直下の動画
だけを絞る（[contracts/list-url.md §2](contracts/list-url.md#2-フォルダ画面での意味)）。
フォルダ画面の控えの鍵に条件を足す。`/` と Esc のキー操作、一致なしと「条件を解除」も
同じにする。配置と見た目は `ui-design.md` による。

**Dependencies**: 検索欄から検索の書き方の手引きを開けるようにする（手引きを含む検索欄を
そのまま使う）

**Acceptance**: `web/src/folders/FolderPage.test.tsx` と `web/e2e/folders.e2e.ts` で、
受け入れ条件 17〜22 がフォルダ画面で確かめられる。検索中にフォルダが無くなると「この
フォルダは見つかりません」が出る。`task check` と `task test-e2e` が通る。実装 PR に
幅ごとの画像と、視覚・操作・支援技術の確認を添える。
