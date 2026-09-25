# Implementation Plan: フォルダからグループを作り、ライブラリで1件として扱って連続再生する

**Branch**: `feature/017-folder-groups` | **Parent Issue**: #326

**Input**: The parent Issue. It is this feature's specification.

## Summary

子フォルダを持たず直下に動画が2本以上あるフォルダを、スキャンのたびにフォルダの形だけから
「グループ」にし、ライブラリではグループを1枚のカードとしてふつうの動画と混ぜて並べ、押すと
続きのメンバーから再生画面を開いて、最後のメンバーまで予告つきで自動で続けて再生する。
利用者はフォルダ単位で「まとめを解除」「直下をまとめる」の例外を付け外しでき、グループを
フォルダ名のタグに変えられる。既存のタグ（名前かシノニム）と同じ名前のフォルダの下にある動画には、
そのタグが付いているものとして扱う（親 Issue #326）。

サーバーは、グループの割り当てと動画ごとの祖先フォルダ名を、所在の表から作り直せる索引として
持つ（[data-model.md §1〜§3](data-model.md)）。例外はフォルダの絶対パスで持つ利用者データである。
フォルダ由来のタグは索引のフォルダ名とタグ名を問い合わせの時点で突き合わせて導くので、タグの変更は
すぐ効く（[data-model.md §4](data-model.md#4-フォルダ由来のタグ)）。ライブラリの一覧は、動画と
グループを「項目」として混ぜて返す新しい経路 `GET /api/library` にし、既存の `GET /api/videos` は
1本ずつの一覧として残す（[contracts/library-api.md](contracts/library-api.md)）。再生画面は、動画の
応答と関連動画の応答に載ったグループから並び・前後・次のメンバーを決める
（[contracts/folder-groups-api.md](contracts/folder-groups-api.md)）。

## Technical Context

**Canonical definitions**:

- 境界と依存方向、store の役割の型、索引と利用者データの区別: [ARCHITECTURE.md](../../ARCHITECTURE.md)・
  [internal/store/roles.go](../../internal/store/roles.go)・[.golangci.yml](../../.golangci.yml)
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`task generate`）。変更系の経路の守り:
  [internal/httpapi/router.go](../../internal/httpapi/router.go)・
  [internal/httpapi/openapi_routes_test.go](../../internal/httpapi/openapi_routes_test.go)
- マイグレーションは足すだけで既存を変えない: [internal/store/migrations/](../../internal/store/migrations/)・
  [scripts/migrations-immutable.sh](../../scripts/migrations-immutable.sh)
- 一覧の組み立て（範囲・検索式・chosen・絞り込み・keyset・shuffle）:
  [internal/store/listing.go](../../internal/store/listing.go)・
  [specs/013-library-search/contracts/list-api.md](../013-library-search/contracts/list-api.md)
- 代表の所在（登録フォルダの下にある所在のうちパスの最小）: `videoColumnsTemplate`（[internal/store/videos.go](../../internal/store/videos.go)）
- 同じフォルダの前後の並び: `domain.OrderRelated`・`compareSiblings`（[internal/domain/related.go](../../internal/domain/related.go)）
- フォルダを所在のパスから導く規則と登録フォルダの判定: [internal/domain/folder.go](../../internal/domain/folder.go)
  （`SummarizeFolder`・`LocateVideoFolder`）・`registeredLocationCondition`
- タグの表・名前の規則・絞り込み・検索欄での照合:
  [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)・
  [specs/014-video-tags/contracts/tags-api.md](../014-video-tags/contracts/tags-api.md)・
  `domain.NormalizeTagName`（[internal/domain/tag.go](../../internal/domain/tag.go)）
- 起動時に版の古い派生値だけを作り直す前例: `LibraryStore.RefreshSearchKeys`・`domain.SearchKeyVersion`
  （[cmd/mdm/main.go](../../cmd/mdm/main.go)）
- Web の所有境界・一覧の控え・再生画面の構成と視覚規則: [ARCHITECTURE.md#web-layer](../../ARCHITECTURE.md#web-layer)・
  [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md)・
  [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md)・
  [specs/014-video-tags/ui-design.md](../014-video-tags/ui-design.md)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）

**Feature-specific context**:

- マイグレーションを1つ足す（`00009_folder_groups.sql`、[data-model.md §1](data-model.md#1-マイグレーション)）。
  既存の表は変えない。
- 規模の前提は既存と同じ1万本である。グループの作り直しは索引の全所在を1回読んで Go で割り当てを
  決め、1つの書き込み取引で置き換える。1万本・数千フォルダで1秒未満を目安にし、単位の PR で
  ベンチマークの値を示す。
- 追加する依存は無い。
- `ui` Issue なので、カード・再生画面の並び・自動再生の予告・フォルダ画面のメニューの見た目と
  予告の秒数は、この Plan のあとの design 工程で `ui-design.md` に決める。この Plan は、画面が
  守る API の契約と、状態がどこで決まるかまでを決める。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。グループの割り当て・
  フォルダ名の取り出し・グループの視聴状態と開くメンバーの決め方は `internal/domain` の純粋関数に
  置く（Structural Decisions 2・9）。SQL は `internal/store`、経路は `internal/httpapi`。
  スキャンの後の作り直しは `internal/app` の `Scans` が自分で宣言する interface から呼ぶ。
- **store の役割の型**（ARCHITECTURE.md の `store.DB` の節）: 合格。例外の書き換えとグループの
  タグ化は新しい役割の型 `FolderGroupStore` に置き、作り直しとタグの作成は非公開の関数で
  共有する（Structural Decisions 8）。型どうしは公開メソッドを呼ばない。
- **索引と利用者データの区別**: 合格。例外（`folder_group_overrides`）は利用者データで、
  `videos` にも登録フォルダにも外部キーを張らずパスで持つ。グループとフォルダ名の表は索引で、
  消えても作り直せる（[data-model.md §1](data-model.md#1-マイグレーション)）。
- **「起動時とスキャンの後にライブラリ全体を読まない」**（ARCHITECTURE.md、`internal/app/scans.go` の
  注記）: 条件付きで合格。親 Issue 要件 1 が「スキャンのたびに作り直す」ことを求めるので、スキャンを
  閉じる直前に索引の表（SQLite）を全件読む。ファイルシステムは読まない。起動時は索引の版が古い
  ときだけ作り直す（`RefreshSearchKeys` と同じ扱い）。ARCHITECTURE.md と注記の文言を、最初の単位で
  この形に書き直す。Complexity Tracking に記録する。
- **ドメインイベント**: 合格。作り直しと例外の変更には、生成物の削除やワーカーの起床のような
  副作用が無いので、イベントを足さない。画面の反映は、変更した画面が控えを捨てて読み直す
  （Structural Decisions 10）。
- **API の正本**（AGENTS.md）: 合格。経路・スキーマ・誤りの `code` は `api/openapi.yaml` に足し、
  `task generate` で生成する。本文を持つ経路は `requiresJSONBody` に足す。
- **UI の正本**（library-ui.md・`web/src/theme/tokens.test.ts`）: 合格の見込み。トークンだけを使い、
  構図は design 工程で決める。画面の単位は視覚・操作・支援技術を確かめる。
- **サーバーと話す場所**（ARCHITECTURE.md「Web layer」）: 合格。新しい経路の呼び出しは
  `web/src/api/` に置く。

Phase 1 のあとも判定は同じである。

## Structural Decisions

1. **グループは作り直せる索引の表（`folder_groups`・`folder_group_members`）に持ち、作り直しは
   所在の表から行う。作り直すのは、スキャンを閉じる直前、メディアフォルダの追加・置換・削除の
   取引の中、例外の変更の取引の中、起動時に索引の版が古いときと中断したスキャンを閉じたとき、である**
   （[data-model.md §3](data-model.md#3-作り直す時点)）。
   - 却下: 一覧の問い合わせのたびに SQL でグループを導く案。並びはファイル名の自然順で SQL では
     書けず、「子フォルダを持たない」の判定を毎ページ全所在に掛けることになる。
   - 却下: スキャナーが歩いたファイルシステムの事実から決める案。例外の変更（要件 8）とメディア
     フォルダの削除は歩かずにグループを変えるので、同じ規則を2か所に持つことになる。
2. **割り当ての規則は `internal/domain` の1つの純粋関数に置く。** 入力は登録フォルダ、登録フォルダの
   下にある全所在（動画の id とパス）、例外で、出力はグループ（パス・名前・並んだメンバー）と
   動画ごとの祖先フォルダ名である（[data-model.md §2](data-model.md#2-割り当ての規則)）。代表の所在は
   今の `videoColumnsTemplate` と同じくパスの最小、メンバーの並びは `compareSiblings` と同じ順にする。
   - 却下: store の中で SQL と Go に分けて書く案。要件 2・6・7 の組み合わせと Edge Cases を
     SQLite 無しで網羅して試せなくなる。
3. **例外はフォルダの絶対パスを鍵にした1行に、1つの `mode`（`ungroup`・`group_direct`）で持つ。**
   2つの例外の重なり（Edge Case「例外の重なり」）は表の形で起きない。鍵は登録フォルダの判定と
   同じ規則で比べられる形（Windows では区切りを `\` にそろえ ASCII を小文字）に整える。
   - 却下: 登録フォルダの `id` と相対パスで持つ案。メディアフォルダを外して付け直すと `id` が
     変わり、要件 7 の「メディアフォルダの変更を経ても残る」を満たさない。
   - 却下: 例外ごとに別の列や表を持つ案。両方が付いた状態を検査で防ぐことになる。
4. **フォルダ由来のタグは保存しない。索引に動画ごとの祖先フォルダ名（`video_folder_names`）を持ち、
   タグの名前（`tag_names`）との突き合わせは読み出しの時点で行う**
   （[data-model.md §4](data-model.md#4-フォルダ由来のタグ)）。タグの作成・改名・シノニム・削除・統合は
   次の読み出しからそのまま効く（要件 14）。
   - 却下: フォルダ由来の付与を `video_tags` に書き込む案。利用者データの表に索引から導いた行が混ざり、
     タグの名前の変更のたびに書き直しが要り、手で付けた分だけを外す（要件 13）区別も列で持つことになる。
   - 却下: 読み出しのたびに所在のパスを段に分ける案。段の切り出しと `NormalizeTagName` は SQL で書けない。
5. **動画のタグの応答は、手で付けたかフォルダ由来かを1件ごとに持つ（`VideoTag`）。取り外しは今の
   経路のまま手で付けた分だけに効く。** 選択の要約には、手で付けた本数（`manualCount`）を足す
   （[contracts/folder-groups-api.md §4](contracts/folder-groups-api.md#4-タグの応答の変更)）。
   - 却下: `TagRef` に出所を足す案。`TagRef` は付け外しの応答や要約でも使い、そこでは出所に意味が無い。
6. **ライブラリの一覧は新しい経路 `GET /api/library`（と「すべて選択」用の `GET /api/library/ids`）に
   し、項目を「動画」か「グループ」の `LibraryItem` にする。既存の `GET /api/videos` は1本ずつの一覧の
   まま残す**（[contracts/library-api.md](contracts/library-api.md)）。フォルダ画面のルートの検索結果
   （`web/src/folders/RootSearchResults.tsx`）が今も `GET /api/videos` で1本ずつ出しており、要件 22 は
   フォルダ画面を今のままとする。`GET /api/videos/ids` はライブラリだけが使うので、ライブラリを
   切り替える単位で消す。
   - 却下: `GET /api/videos` の項目の型を変える案。フォルダ画面の検索結果まで項目の形が変わる。
   - 却下: グループを `Video` に付けた欄で表し、代表のメンバーを項目にする案。題名・長さ・大きさ・
     視聴状態が動画の値かグループの値かを欄ごとに読み分けることになる。
   - 却下: グループを別の経路で返して画面で混ぜる案。並び・件数・keyset を2つの一覧で合わせられない（要件 15・18・20）。
7. **一覧の絞り込みはメンバー単位、視聴状態の絞り込みと並べ替えは項目単位で行う。グループの集計値
   （長さ・大きさ・日時・視聴の本数）は問い合わせの時点でメンバーから数える**
   （[data-model.md §5](data-model.md#5-ライブラリの項目)）。keyset とシャッフルの `id` は、動画は
   動画の `id`、グループは残っているメンバーのうち並びで最初のものの `id` で、項目どうしで重ならない。
    - 却下: keyset の `id` に `folder_groups.id` を使う案。作り直しのたびに振り直され、動画の `id` と
      同じ数の空間で重なりうる。
   - 却下: 集計値を作り直しのときに表へ書く案。長さは取り込みの解析で、最後に再生した時刻は再生の
     たびに変わり、作り直しの時点の値はすぐ古くなる。
8. **例外の変更とグループのタグ化は新しい役割の型 `FolderGroupStore`（`db.FolderGroups()`）に置く。
   作り直しはパッケージ内の非公開の関数 `rebuildFolderIndex(tx)` にし、`ScanIndexStore`（スキャンの後と
   起動時）・`SettingsStore`（メディアフォルダの変更の取引）・`FolderGroupStore` がそれぞれ自分の取引の
   中で呼ぶ。タグ化のタグの作成は `tags.go` の名前の引き方と作成を非公開の関数に切り出して共有する。**
   経路は `internal/httpapi` から `FolderGroupStore` を直接呼び、`internal/app` を通さない（タグの
   操作と同じ理由で、組み合わせる相手が無い）。
   - 却下: `TagStore` に置く案。例外はタグではなく、`TagStore` が索引に書くことになる。
   - 却下: `SettingsStore` に置く案。`SettingsStore` は登録フォルダの設定の型で、フォルダごとの
     利用者データを混ぜると型の線が消える。
9. **グループの視聴状態、見終えた本数、押したときに開くメンバー（要件 17・23）はサーバーが
   `internal/domain` の関数で決めて項目に載せる**（[data-model.md §6](data-model.md#6-グループの視聴状態と開くメンバー)）。
   - 却下: 画面がメンバーの再生位置から決める案。カードは全メンバーの再生位置を持たず、そのために
     全メンバーを項目に載せることになる。
10. **例外とタグ化の結果は、操作した画面が一覧の控えを捨てて読み直すことで反映する。
    ほかのタブは再読み込みで一致する**（Edge Case「同時操作」）。SSE のイベントは足さない。
    一覧に残っているグループのカードは、メンバーの再生位置・タグ・`video` イベントの変化で
    `GET /api/folders/{rootId}/group` から1件ずつ取り直す（[contracts/library-api.md §3](contracts/library-api.md#3-グループ1件)）。
    - 却下: 例外の変更を SSE で配る案。親 Issue が求めるのは再読み込みで一致することだけである。
    - 却下: 再生から戻るたびに控えを捨てる案。グループを1本見るたびにスクロールと読み込み済みの
      ページを失う。
11. **再生画面のグループの並び・前後・次のメンバーは、関連動画の応答に載せたグループから決める。**
    グループのメンバーを開いたときは、`nextId`・`prevId` をグループの中の並びにし、関連動画の
    `items` から同じグループのメンバーを除く。題名の近くの「何本目」は `GET /api/videos/{id}` の
    `group` から出す（[contracts/folder-groups-api.md §3](contracts/folder-groups-api.md#3-動画と関連動画のグループ)）。
    - 却下: グループの並びを別の経路で取る案。関連動画の組み立て（`app.Catalog`）が既に同じフォルダの
      前後を決めており、2つの応答の食い違い（前後のつまみと並びが違うメンバーを指す）を画面で合わせることになる。
12. **自動再生は画面の中で行う。予告の間は `/api/events` の `video` イベントのうち次のメンバーを
    名指しするものを見張り、届いたら `GET /api/videos/{id}` で確かめる。予告が終わった時点でも同じく
    確かめる。どちらかで 404 なら予告をやめて再生終了の表示にする**（Edge Case「予告中に次のメンバーが
    消えたら」）。動画の行を消した取引は消えた動画ごとに `domain.VideoIngestChanged` を発行するので
    （`internal/store/sqlite.go` の `changes`）、スキャンでもメディアフォルダの削除でも届く。次のメンバーは
    開いたときの関連動画の応答で決まり、途中で別のメンバーへ差し替えない（Edge Case「再生中にグループが
    変わる」）。移るときは前後のつまみと同じく `state.from` を引き継ぐので、閉じる操作は開く前の一覧へ
    戻る（要件 30）。
    - 却下: 予告の終わりに1回だけ確かめる案。消えたメンバーへの予告が残り秒数のあいだ出続け、
      Edge Case の「予告をやめる」を満たさない。
    - 却下: 予告の間に1秒ごとに `GET` する案。消えたことを伝える `video` イベントが既にあり、要求を足す理由が無い。
13. **Web の一覧の保持（`useVideos`・`listSnapshot`）は、ライブラリとフォルダ画面の両方で項目を
    `LibraryItem` にそろえる。** フォルダ画面の応答は動画の項目に包む。再生位置・タグ・`video` イベントの
    反映は、動画の項目では今のまま、グループの項目ではメンバーの `id` で当てて Structural Decisions 10 の
    取り直しを行う。グループのカードは `web/src/library/` に置く（フォルダ画面には出ないため）。
    - 却下: ライブラリ用に別の保持を作る案。ページング・取り消し・控えの復元（約 500 行）を2つ持つことになる。

14. **スキャンを閉じる前の作り直しに失敗しても、スキャンは失敗にせず閉じる。** 取り込みの結果は
    既に書き込まれており、作り直しは次の時点（次のスキャン・例外の変更・メディアフォルダの変更）で
    やり直せる。失敗は索引の状態に `stale` として残し、次の起動でも作り直す
    （[data-model.md §3](data-model.md#3-作り直す時点)）。
    - 却下: スキャンを失敗として閉じる案。取り込んだ動画は一覧に出ているのに、スキャンの表示だけが
      失敗になり、利用者に直す手段が無い。
15. **グループでないフォルダのタグ化は 409 で断る**（[contracts/folder-groups-api.md §2](contracts/folder-groups-api.md#2-グループをタグに変える)）。
    要件 11 はグループに対する操作で、画面はグループにだけこの操作を出す。
    - 却下: グループでなくてもタグを作って `ungroup` を書く案。効かない例外が残り、画面の状態の
      食い違い（古いタブ）を黙って通すことになる。

## Project Structure

### Documentation (this feature)

```text
specs/017-folder-groups/
├── plan.md
├── data-model.md                   # 表・割り当ての規則・作り直し・フォルダ由来のタグ・項目の集計
└── contracts/
    ├── library-api.md              # GET /api/library・/ids・グループ1件
    └── folder-groups-api.md        # 例外とタグ化、Video.group・関連動画、タグの応答の変更
```

`research.md` は作らない。判断はすべて上の Structural Decisions に代案とともに書いた。
`quickstart.md` は作らない。各単位の受け入れ証拠がテストと画面の確認を名指ししており、それ以外に
走らせる手順が無い。`ui-design.md` は design 工程が作る。

### Source Code

**Affected boundaries**:

- `internal/domain`: グループの割り当て・フォルダ名・例外の値・グループの視聴状態と開くメンバー。
- `internal/store`: マイグレーション、`FolderGroupStore`、`rebuildFolderIndex`、ライブラリの項目の
  問い合わせ、タグの読み出しへのフォルダ由来の分の追加、関連動画のためのグループの読み出し。
- `internal/app`: `Scans` がスキャンを閉じる前に作り直しを呼ぶ。`Catalog` が関連動画にグループを載せる。
- `internal/httpapi`・`api/openapi.yaml`: 新しい経路と、`Video`・`RelatedVideos`・`FolderSummary`・タグの応答の変更。
- `cmd/mdm`: `FolderGroupStore` の組み立てと、起動時の版の古い索引の作り直し。
- `web/src/api`・`web/src/library`・`web/src/player`・`web/src/folders`・`web/src/videoList`: 画面。

**New paths**: `internal/store/migrations/00009_folder_groups.sql`・`internal/store/folder_groups.go`・
`internal/domain/folder_group.go`・`internal/httpapi/folder_groups.go`・`internal/httpapi/library.go`。
Web の新しいファイルの名前は各単位で決める。

**Structure decision**: 既存の層に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。新しい層やパッケージは作らない。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| スキャンの後に索引の全所在を読む | 要件 1 がスキャンのたびのグループの作り直しを求める | 問い合わせのたびに導く案は Structural Decisions 1 の理由で採らない。読むのは SQLite だけで、ファイルシステムは読まない |

## Implementation Work

### フォルダからグループを割り当てる索引と、フォルダごとの例外をサーバーに置く

**Scope**: マイグレーション `00009_folder_groups.sql`、`internal/domain` の割り当ての規則とフォルダ名の
取り出し、`rebuildFolderIndex` と4つの作り直しの時点（スキャンを閉じる前・メディアフォルダの変更・
例外の変更・起動時の版・中断したスキャンの回復）、`FolderGroupStore` の例外の設定と解除。
ARCHITECTURE.md の「スキャンの後に全体を読まない」の記述（と `internal/app/scans.go` の注記）、
「Two kinds of data」の表の区分、store の役割の型の一覧に、この単位が足すものを書く
（[data-model.md §1〜§3](data-model.md)）。

**Dependencies**: None

**Acceptance**: `internal/domain` の表駆動テストが、要件 2〜7 と Edge Cases（深さ、ルート直下、1本だけの
フォルダ、子フォルダと動画の混在、同じ内容の複数の所在、2つの例外の切り替え、例外のパスが無いとき）の
割り当てを確かめて通る。`internal/store` のテストが、スキャンの後・メディアフォルダの削除・例外の変更の
それぞれでグループの表が置き換わり、例外が再スキャンと再オープンの後も残ることを確かめて通る。
1万本のベンチマークの値を PR に示す。`task check` が通る。

### フォルダ名から既存のタグを動画に付け、タグの表示・絞り込み・検索・本数・要約に含める

**Scope**: `video_folder_names` と `tag_names` の突き合わせを、動画のタグの読み出し・タグでの絞り込み・
検索欄のタグ名の照合・タグごとの本数・選択の要約に足す。`Video.tags` を `VideoTag`（出所つき）にし、
要約に `manualCount` を足す（[data-model.md §4](data-model.md#4-フォルダ由来のタグ)・
[contracts/folder-groups-api.md §4](contracts/folder-groups-api.md#4-タグの応答の変更)）。画面は生成した型に
合わせてコンパイルが通るところまで直し、見た目は変えない。

**Dependencies**: フォルダからグループを割り当てる索引と、フォルダごとの例外をサーバーに置く

**Acceptance**: store と httpapi のテストが、受け入れ条件 6・7・8（シノニムの登録で再スキャンなしに付く）と
Edge Cases（タグの削除・統合がすぐ効く、同名のフォルダが別の場所にある、複数の所在の祖先を両方使う）を
確かめて通る。取り外しの経路で外れるのが手で付けた分だけであること（受け入れ条件 9 のサーバー側）を
テストが確かめる。`task check` が通る。

### ライブラリの一覧でグループを1件として動画と混ぜて返す API を足す

**Scope**: `GET /api/library`・`GET /api/library/ids`・`GET /api/folders/{rootId}/group` と、
その下のライブラリの項目の問い合わせ（メンバー単位の絞り込み、項目単位の視聴状態と並べ替え、
集計、keyset、件数）、グループの視聴状態と開くメンバーの決め方
（[contracts/library-api.md](contracts/library-api.md)・[data-model.md §5・§6](data-model.md#5-ライブラリの項目)）。

**Dependencies**: フォルダからグループを割り当てる索引と、フォルダごとの例外をサーバーに置く;
フォルダ名から既存のタグを動画に付け、タグの表示・絞り込み・検索・本数・要約に含める

**Acceptance**: store と httpapi のテストが、受け入れ条件 1・2・10（13 種類の並び順（`VideoSort`）とシャッフルで項目が
要件 18 の値で並び、`total` が項目の数と一致し、ページをまたいで重複と抜けが無い）・11・12（視聴状態と
その絞り込み）・14 の開くメンバー、`ids` がグループの全メンバーを含むことを確かめて通る。
`task check` が通る。

### フォルダの例外の付け外しと、グループをタグに変える操作を API で公開する

**Scope**: `PUT /api/folders/{rootId}/grouping`・`POST /api/folders/{rootId}/grouping/tag` と、
`FolderSummary.grouping`。タグ化は1つの取引で、名前の検査に失敗したら何も書かない
（[contracts/folder-groups-api.md §1・§2](contracts/folder-groups-api.md#1-フォルダのまとめ方)）。

**Dependencies**: フォルダからグループを割り当てる索引と、フォルダごとの例外をサーバーに置く;
フォルダ名から既存のタグを動画に付け、タグの表示・絞り込み・検索・本数・要約に含める

**Acceptance**: httpapi のテストが、受け入れ条件 3・4・5 のサーバー側（例外の直後の `GET /api/library` の
項目、タグ化の後にメンバーが単体に戻りそれぞれにタグが付く、同名のタグやシノニムがあればそれを使う）、
タグ名に使えないフォルダ名で 400 になり例外もタグも増えないこと、グループでないフォルダのタグ化が
409 になることを確かめて通る。`openapi_routes_test.go` が通る。`task check` が通る。

### 動画と関連動画の応答にグループを載せ、前後をグループの中の並びにする

**Scope**: `GET /api/videos/{id}` の `group`、`GET /api/videos/{id}/related` の `group` と、メンバーの
ときの `nextId`・`prevId`・`items` の変更（[contracts/folder-groups-api.md §3](contracts/folder-groups-api.md#3-動画と関連動画のグループ)）。
`VideoFolder` の説明（「GET /api/videos/{id} には入らない」）を、`group.folder` として入る形に直す。

**Dependencies**: フォルダからグループを割り当てる索引と、フォルダごとの例外をサーバーに置く

**Acceptance**: `internal/app` と httpapi のテストが、メンバーの応答にグループの全メンバーが並びの順で
入ること、最初のメンバーに `prevId` が無く最後のメンバーに `nextId` が無いこと、`items` に同じグループの
メンバーが無いこと、グループに属さない動画の応答が変わらないことを確かめて通る。`task check` が通る。

### 画面の一覧の保持の項目を、動画とグループの両方を運べる形にそろえる

**Scope**: `web/src/api/useVideos.ts`・`listSnapshot.ts` の項目を `LibraryItem` にそろえ（Structural
Decisions 13）、フォルダ画面の応答と、切り替え前のライブラリの `GET /api/videos` の応答を動画の項目に包む。
グループの項目への再生位置・タグ・`video` イベントの反映と `GET /api/folders/{rootId}/group` による取り直しを
足す。画面の見た目は変えない。

**Dependencies**: ライブラリの一覧でグループを1件として動画と混ぜて返す API を足す

**Acceptance**: Vitest が、動画の項目で今の再生位置・タグ・`video` イベント・控えの復元の振る舞いが
変わらないことと、グループの項目がメンバーの変化で取り直され、404 で一覧から外れることを確かめて通る。
`task check` が通る。

### ライブラリでグループを1枚のカードで出し、押すと続きのメンバーから再生する

**Scope**: ライブラリを `GET /api/library` に切り替え、グループのカード、グループのカードの選択（全メンバー）、
「すべて選択」の `GET /api/library/ids`、件数を足す。使われなくなる `GET /api/videos/ids` を
`api/openapi.yaml`・`internal/httpapi`・`internal/store`（`VideoIDs`）とそのテストから消し、ARCHITECTURE.md の
その記述を直す。`ui-design.md` の該当する節に従う。

**Dependencies**: 画面の一覧の保持の項目を、動画とグループの両方を運べる形にそろえる

**Acceptance**: 画面が変わる単位で、実装で視覚・操作・支援技術を確かめる。Vitest が、受け入れ条件 1・2・
12（再生から戻るとカードの視聴状態と本数が変わる）・13・14（押すと開くメンバーの再生画面へ移る）と、
カードの支援技術向けの名前にグループであることと本数が入ることを確かめて通る。フォルダ画面の一覧と
ルートの検索結果が今のまま1本ずつ出ること（受け入れ条件 18）をテストが確かめる。`GET /api/videos/ids` を
消したあとの Go のテストと `openapi_routes_test.go` を含めて `task check` が通る。

### 再生画面にグループの並びを出し、次のメンバーを予告して自動で再生する

**Scope**: 題名の近くのグループ名と何本目、関連動画の列の上位のメンバーの並びと境目、前後のつまみ、
再生が終わったときの予告・取り消し・自動再生と、予告の終わりの確かめ（Structural Decisions 12）。
`ui-design.md` の該当する節に従う。

**Dependencies**: 動画と関連動画の応答にグループを載せ、前後をグループの中の並びにする

**Acceptance**: 画面が変わる単位で、実装で視覚・操作・支援技術を確かめる。Vitest が、受け入れ条件 15・16・
17・19 と、予告の終わりに次のメンバーが無ければ再生終了の表示になること、自動で開いたメンバーが再生に
失敗したら先へ進まないこと、予告の開始が一度だけ読み上げられ取り消しがキーボードで押せることを
確かめて通る。数百本のグループで今のメンバーが開いた時点で見える位置にあることを確かめる。
`task check` と `task test-e2e` が通る。

### フォルダ画面と再生画面から、まとめの解除・直下をまとめる・グループをタグに変えるを操作する

**Scope**: フォルダ画面のメニューと、再生画面のグループの見出しのメニューに、例外の付け外しとタグ化を
置く。操作の後は一覧の控えを捨て、今の画面を読み直す（Structural Decisions 10）。`ui-design.md` の
該当する節に従う。

**Dependencies**: フォルダの例外の付け外しと、グループをタグに変える操作を API で公開する;
再生画面にグループの並びを出し、次のメンバーを予告して自動で再生する;
ライブラリでグループを1枚のカードで出し、押すと続きのメンバーから再生する

**Acceptance**: 画面が変わる単位で、実装で視覚・操作・支援技術を確かめる。Vitest が、受け入れ条件 3・4・5 の
画面側（操作の直後にライブラリへ戻ると再スキャンなしにカードが変わる）と、タグ化の失敗を伝えることを
確かめて通る。`task check` が通る。

### フォルダ由来のタグを手で付けたタグと区別して出し、選択バーでは手で付けた分だけを外す

**Scope**: カード・グループのカード・再生画面のタグの表示で出所を区別し、フォルダ由来のタグには外す操作を
出さない。選択バーの取り外しの候補と件数を `manualCount` から作る。`ui-design.md` の該当する節に従う。

**Dependencies**: フォルダ名から既存のタグを動画に付け、タグの表示・絞り込み・検索・本数・要約に含める;
ライブラリでグループを1枚のカードで出し、押すと続きのメンバーから再生する

**Acceptance**: 画面が変わる単位で、実装で視覚・操作・支援技術を確かめる。Vitest が、受け入れ条件 6 の
区別した表示と受け入れ条件 9（手でも付けていた場合は手の分が外れてフォルダ由来の表示が残る）を
確かめて通る。区別が文字の大きさではなく形か色であることを `ui-design.md` と照らして確かめる。
`task check` が通る。
