# Implementation Plan: フォルダ階層をたどって動画を見るフォルダ画面

**Branch**: `claude/repository-skill-check-35jcfm` | **Parent Issue**: #145

**Input**: The parent Issue. It is this feature's specification.

## Summary

サイドバーに「フォルダ」の入口を足し、登録済みメディアフォルダから階層をたどって、
各フォルダ直下の子フォルダ（中身のサムネイルが透けるフォルダカード）と直下の動画を
見せる画面を追加する（親 Issue #145）。

フォルダは新しく保存せず、取り込み済みの所在（`video_locations.path`）から要求の
たびに導く。サーバーは「登録フォルダの id と相対パス」で指されたフォルダについて、
子フォルダの集計と直下の動画のページを返す新しい読み取り専用 API を持つ
（[contracts/folders.md](contracts/folders.md)）。画面は既存の動画カード・ページング・
復元の仕組みを使い回し、フォルダカードとパンくずを足す。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`task generate`）
- 論理動画と所在の分離、登録フォルダの判定:
  [internal/store/videos.go](../../internal/store/videos.go)
  （`registeredLocationCondition`・`ListVideos` の keyset ページング）
- 登録フォルダの重複・包含の拒否:
  [internal/store/media_folders.go](../../internal/store/media_folders.go)
- Web の所有境界と視覚規則: [ARCHITECTURE.md#web-layer](../../ARCHITECTURE.md#web-layer)・
  [library-ui.md](../../docs/design-docs/library-ui.md)
- 一覧のページング・復元: [web/src/api/useVideos.ts](../../web/src/api/useVideos.ts)・
  [web/src/api/listSnapshot.ts](../../web/src/api/listSnapshot.ts)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）

**Feature-specific context**:

- 新しい経路は3つ（`GET /api/folders`・`GET /api/folders/{rootId}`・
  `GET /api/folders/{rootId}/videos`）で、既存経路の意味は変えない
  （[contracts/folders.md](contracts/folders.md)）。
- 登録フォルダ同士は入れ子にならない（設定 API が `overlapping_media_directories` で
  拒否する）。1つの所在はちょうど1つの登録フォルダに属するので、集計で二重に数えない。
- 規模の前提は既存と同じ1万件。1つの登録フォルダに所在が1万件あっても、1回の要求で
  それを1度読むだけで済む形にする（Structural Decisions 1）。
- 追加の依存・マイグレーション・設定値は無い。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。パスの分解・
  自然順・子フォルダの集計は `internal/domain` の純粋な関数に置き、`net/http`・
  `database/sql` に依存しない。`internal/store` は所在の行を読むだけ、`internal/httpapi` は
  自分が使う操作をインターフェースとして宣言し、`cmd/mdm` が `*store.DB` を渡す。
- **API の正本**（AGENTS.md・ARCHITECTURE.md）: 合格。3経路と新しい型は
  `api/openapi.yaml` に足し、`task generate` で Go と TypeScript を生成する。生成物は手で
  編集しない。
- **データの扱い**（ARCHITECTURE.md の再構築可能な索引と利用者データの区別）: 合格。
  読み取り専用で、テーブル・マイグレーション・`playback_progress` に触れない。
- **UI の正本**（library-ui.md 1・4・5）: 合格。色と大きさは `web/src/index.css` の
  トークンだけを使う。幅による出し分けは CSS だけで行う。構図は人が実機で確かめ、
  PR に画像を添える。
- **スクロールの所有**（ARCHITECTURE.md「The shell does not take ownership of
  scrolling」）: 合格。フォルダ画面も `window` のスクロールと既存の無限スクロールの
  観測点を使い、スクロール容器を作らない。
- **裏側の無い入口を増やさない**（library-ui.md 7）: 合格。「フォルダ」の入口は、
  裏側（この画面）と同時に足す。

Phase 1 のあとも判定は同じで、正当化の要る違反は無い。

## Structural Decisions

1. **フォルダは保存せず、所在のパスから要求のたびに導く。** 子フォルダの一覧と件数・
   プレビューは、登録フォルダ配下の所在の行（パス・動画 id・サムネイル状態・内容鍵）を
   1回読み、`internal/domain` の関数で次の段ごとにまとめて作る。
   - 却下: フォルダ表を持ってスキャン時に保守する案。再構築可能な索引が増え、
     取り込み・移動・設定変更（`syncLocationsUnder`）のすべてで整合を保つ必要が出る。
     1万件を1回読む費用に見合わない。
   - 却下: 集計を SQL の文字列関数だけで書く案。次の段の切り出しと Windows の区切り
     （`registeredLocationCondition` が既に両方を扱っている）を SQL に重ねて書くことに
     なり、自然順も SQL では書けない。
2. **直下の動画は SQL で keyset ページングする。** 「接頭辞 + 区切りの後ろに区切りが
   無い」所在を動画ごとに1件へまとめ（パスの昇順で先の所在を代表にする）、既存の
   `addedDesc`・`titleAsc` とカーソルの形をそのまま使う。
   - 却下: 直下の動画も Go 側で全件をまとめて返す案。1フォルダに数千本あると応答が
     青天井になり、要件 12 のページ単位の読み込みにならない。
3. **フォルダの識別子は「登録フォルダの id + `/` 区切りの相対パス」にする。** 画面の
   URL は `/folders/{rootId}/{段}/{段}…` で、各段を `encodeURIComponent` で包む。
   - 却下: 絶対パスを URL と API に載せる案。登録フォルダの移動（設定画面での変更）で
     URL が壊れ、`..` を含む任意の絶対パスを API が受け取る入口にもなる。
   - 却下: react-router の splat 引数の復号に任せる案。`%` や `#` を含む段で復号が
     一度多く走ると別のフォルダを指すため、`location.pathname` を段ごとに自分で復号する
     （Edge Case「パスに特殊な文字」）。
4. **フォルダ一覧と直下の動画を別の経路にする。** 子フォルダ（全件・並びは自然順で固定）と
   動画（ページング・並び順の選択あり）は、取り方も読み直す契機も違う。並び順を変えたとき
   に読み直すのは動画だけで、子フォルダの並びは変わらない（受け入れ条件 8）。
   - 却下: 既存の `GET /api/videos` にフォルダの条件を足す案。ライブラリの検索・総件数と
     混ざり、所在ごとの題名という別の意味を同じ応答に持ち込む。
5. **一覧の取得・中断・復元は既存の仕組みを一般化して共有する。** `useVideos` は取得元
   （ライブラリか、フォルダか）を受け取る形にし、`listSnapshot` の鍵にフォルダを含める。
   フォルダ画面の控えは子フォルダの一覧も一緒に持ち、戻ったときに往復なしで同じ位置へ
   スクロールを戻す（要件 10）。
   - 却下: フォルダ画面用に同じフックを複製する案。古い応答の拒否と画面離脱時の中断
     （Edge Case「利用者が去る」）を2か所で保守することになる。
6. **フォルダ画面は `web/src/folders/` が持つ。** ページ・フォルダカード・パンくず・
   ツールバー・URL と識別子の相互変換をここに置き、動画カードは `web/src/library/` の
   ものを使う。`web/src/api/` だけがサーバーと話す規則は変えない。
   - 却下: `web/src/library/` の中に置く案。`library/` は検索・絞り込み・選択を持つ
     ライブラリ一覧の流れを所有しており（ARCHITECTURE.md「Web layer」は画面の流れごとに
     ディレクトリを分ける）、それらを持たないフォルダ画面を混ぜると、どの部品がどちらの
     流れに属するかが読めなくなる。
   - 却下: フォルダカードとパンくずを `web/src/ui/` に置く案。`ui/` は画面の意味を持たない
     汎用の部品の置き場で、フォルダの件数や `rootId` を知る部品は入らない。

## Project Structure

### Documentation (this feature)

```text
specs/011-folder-browser/
├── plan.md
├── ui-design.md         # Design 工程で追加する
├── quickstart.md
└── contracts/
    └── folders.md
```

新しく保存するエンティティが無いので `data-model.md` は作らない。判断は上の
Structural Decisions で閉じており、別に調べて決めることが無いので `research.md` も
作らない。この workflow は `tasks.md` を作らず、下の Implementation Work を子 Issue にする。

### Source Code

**Affected boundaries**:

- `api/openapi.yaml` と生成物: 3経路と `FolderSummary`・`FolderPreview`・
  `FolderListing`・`RootFolderListing`
- `internal/domain/`: 相対パスの検証、次の段の切り出し、自然順、子フォルダの集計
- `internal/store/`: 登録フォルダ配下の所在の読み出しと、直下の動画の keyset ページング
- `internal/httpapi/`: 3経路の入口、誤りの対応付け、サムネイル URL の組み立て
- `cmd/mdm/`: 新しいインターフェースへ `*store.DB` を渡す
- `web/src/api/`: 取得関数、`useVideos` の一般化、`listSnapshot` の鍵
- `web/src/shell/`: 「フォルダ」の入口（`navigation.ts`・`Sidebar.tsx`）
- `web/src/app/App.tsx`: `/folders` と `/folders/:rootId/*` の経路
- `web/e2e/`: フォルダ階層の実ブラウザ検証
- `ARCHITECTURE.md`: 公開する API とシェルが持つ経路の記述
- `README.md`: `web/src/` の構成一覧に `folders/` を足す

**New paths**:

- `internal/domain/folder.go` と対応 test
- `internal/store/folders.go` と対応 test
- `internal/httpapi/folders.go` と対応 test
- `web/src/folders/`（`FolderPage.tsx`・`FolderCard.tsx`・`Breadcrumbs.tsx`・
  `FolderToolbar.tsx`・`folderPath.ts` と対応 test）
- `web/e2e/folders.e2e.ts`

## Implementation Work

### フォルダ閲覧 API（子フォルダの集計と直下の動画）

**Scope**: [contracts/folders.md](contracts/folders.md) の3経路と4つの型を
`api/openapi.yaml` に足して生成し、`internal/domain` の相対パス検証・自然順・子フォルダ
集計、`internal/store` の所在の読み出しと直下の動画の keyset ページング、
`internal/httpapi` の入口と誤りの対応付け、`cmd/mdm` の組み立てを実装する。
`ARCHITECTURE.md` の API の記述を更新する。画面は変えない。

**Dependencies**: なし。

**Acceptance**: `root/A/x.mp4`・`root/A/B/y.mp4`・`root/A/B/C/z.mp4` の索引に対し、
`GET /api/folders/{rootId}?path=A` が子フォルダ `B`（`videoCount` 1・`folderCount` 1・
`previews` は `y` だけ）を返し、`/videos?path=A` が `x` だけを返すことを store と httpapi
の test が示す。自然順（`1`・`2`・`10`）、プレビュー最大4件、同じ動画の同じフォルダ内の
重複が1件になること、所在ごとの題名、`..` 等の 400、存在しないフォルダと未登録 id の
404、61件以上でのカーソル続き取得が test で確認できる。`task check` が成功する。

### フォルダ画面の階層表示とサイドバーの入口

**Scope**: 親 Issue の要件 1〜9・11〜13 と `ui-design.md`（Design 工程で追加する）に従い、
サイドバーの「フォルダ」入口、`/folders` と `/folders/:rootId/*` の経路、最上位の
登録フォルダ一覧、フォルダカード（中身が透けるプレビュー・件数・読み上げ名）、パンくず、
並び順と表示倍率だけのツールバー、直下の動画の格子と続きの読み込み、空・見つからない・
読み込み失敗の状態、画面を離れたときの要求の取り消し（Edge Case「利用者が去る」）を
`web/src/folders/` と `web/src/api/` に実装する。並び順と表示倍率は
ライブラリと同じ `web/src/preferences/viewPreferences.ts` の値を読み書きする（要件 13）。
`README.md` の `web/src/` 構成一覧に `folders/` を足し、`ARCHITECTURE.md` の Web layer に
フォルダ画面の経路と所有を書き足す。`useVideos` を
取得元で一般化し、ライブラリ一覧の振る舞いは変えない。単体 test を足す。

**Dependencies**: フォルダ閲覧 API（子フォルダの集計と直下の動画）。

**Acceptance**: 単体 test で、フォルダカードの読み上げ名、プレビューが4件までで
サムネイルが無いときは外形だけになること、`%`・`#`・日本語を含む段の URL 往復、
並び順の変更で子フォルダの並びが変わらないこと、存在しないフォルダで「見つかりません」と
最上位への導線が出ることを確かめる。既存のライブラリ・シェルの test が変更なしで通り、
`task check` が成功する。画面が変わる単位なので、PR に 360px・768px・1280px の画像と、
`ui-design.md`（Design 工程で追加する）の観点による見た目・キーボード操作・支援技術の確認結果を添える。

### フォルダ画面の再生からの復帰と取り込み後の再表示

**Scope**: 要件 10 と Edge Case「取り込み中」に従い、`listSnapshot` の
鍵にフォルダを含め、控えに子フォルダの一覧を持たせて、再生画面から戻ったときに同じ
フォルダの同じスクロール位置へ戻す。取り込みが終わったら控えを捨てて読み直す。
`web/e2e/folders.e2e.ts` で、[quickstart.md](quickstart.md) のフォルダ構成に対し、親 Issue
の受け入れ条件 1〜13 を実ブラウザで通す。

**Dependencies**: フォルダ画面の階層表示とサイドバーの入口。

**Acceptance**: 単体 test で、フォルダ画面の控えが別のフォルダやライブラリ一覧の鍵と
取り違えられないこと、取り込み完了で読み直されることを確かめる。`task test-e2e` の
`folders.e2e.ts` が、深い階層での再読み込み・戻る、再生から戻ったときのスクロール位置、
61件以上の続き取得、キーボードだけの一連の操作を含めて成功する。`task check` が成功する。
画面の見た目が変わる場合は PR に画像を添え、変わらなければ「UI 変更なし」と書く。
