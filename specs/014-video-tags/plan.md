# Implementation Plan: 動画にタグを付けて分類し、タグで絞り込む

**Branch**: `feature/014-video-tags` | **Parent Issue**: #193

**Input**: The parent Issue. It is this feature's specification.

## Summary

利用者が動画にタグを付け外しし、動画に付いたタグを押してタグの AND でライブラリ一覧を
絞り込み、検索欄でもタグ名から探せるようにする。タグそのものは1つの管理画面で作成・
改名・削除・統合・シノニム登録できるようにする（親 Issue #193）。

サーバーは、タグと名前（元の名前とシノニムを1つの名前空間で持つ）と、動画の中身
（`content_key`）とタグの対応を、利用者データの表として持つ
（[data-model.md](data-model.md)）。付与・取り外しは1本でも複数本でも同じ1つの経路で、
動画の `id` の集合を受けて1つのトランザクションで反映する。一覧の項目にタグを載せ、
一覧の問い合わせにタグの AND を足す（[contracts/tags-api.md](contracts/tags-api.md)）。
画面は、押したタグを URL に `id` で載せ、絞り込み中のタグを一覧の上に並べて1つずつ
外せるようにする（[contracts/list-url.md](contracts/list-url.md)）。
検索欄の語は、題名と場所に加えてタグの名前とシノニムにも照合する（要件 10）。
一覧の条件・URL・問い合わせの組み立ては、#195 が作った形の上に足す。

## Technical Context

**Canonical definitions**:

- 境界と依存方向: [ARCHITECTURE.md](../../ARCHITECTURE.md)
- API の正本と生成: [api/openapi.yaml](../../api/openapi.yaml)（`task generate`）。誤りの形と
  変更系の経路の守り: [internal/httpapi/router.go](../../internal/httpapi/router.go)・
  [internal/httpapi/openapi_routes_test.go](../../internal/httpapi/openapi_routes_test.go)
- スキーマと利用者データの区別: [internal/store/migrations/](../../internal/store/migrations/)
  （[00002_core.sql](../../internal/store/migrations/00002_core.sql) の冒頭）。既存ファイルは
  変えない（[scripts/migrations-immutable.sh](../../scripts/migrations-immutable.sh)）
- store の役割ごとの型と、型どうしで共有する非公開の関数の決まり:
  [ARCHITECTURE.md](../../ARCHITECTURE.md) の `store.DB` の節と
  [internal/store/roles.go](../../internal/store/roles.go)
- 層（`internal/domain`・`internal/app`・アダプタ）と depguard の検査:
  [ARCHITECTURE.md#intended-dependency-direction](../../ARCHITECTURE.md#intended-dependency-direction)・
  [.golangci.yml](../../.golangci.yml)。副作用のためのドメインイベント:
  [internal/domain/event.go](../../internal/domain/event.go)
- 利用者データを `content_key` で持つ前例: `playback_progress` と `PlaybackStore`
  （[internal/store/progress.go](../../internal/store/progress.go)）、一覧の項目へ載せる前例は
  [internal/httpapi/videos.go](../../internal/httpapi/videos.go) の `progressFor`
- いまライブラリにある動画の条件: `registeredVideoCondition`（[internal/store/videos.go](../../internal/store/videos.go)）
- 一覧の問い合わせ・条件の URL: #195 の
  [plan.md](../013-library-search/plan.md)・
  [contracts/list-api.md](../013-library-search/contracts/list-api.md)・
  [contracts/list-url.md](../013-library-search/contracts/list-url.md)・
  [ui-design.md](../013-library-search/ui-design.md)。条件と URL の変換、カード、一致なしの
  表示は、ライブラリとフォルダ画面が共有する [web/src/videoList/](../../web/src/videoList/) にある
- Web の所有境界と視覚規則: [ARCHITECTURE.md#web-layer](../../ARCHITECTURE.md#web-layer)・
  [library-ui.md](../../docs/design-docs/library-ui.md)
- 一覧のページング・復元: [web/src/api/useVideos.ts](../../web/src/api/useVideos.ts)・
  [web/src/api/listSnapshot.ts](../../web/src/api/listSnapshot.ts)・
  [web/src/api/progressEvents.ts](../../web/src/api/progressEvents.ts)
- 検査入口: [Taskfile.yml](../../Taskfile.yml)（`task check`・`task test-e2e`）

**Feature-specific context**:

- この feature は、#195（統合 PR #219 で `main` に入った）の一覧の組み立て
  （`internal/store/listing.go`・`VideoQuery` の視聴状態と再生可否）と、条件の URL
  （`web/src/videoList/listCriteria.ts`・`useListCriteria.ts`）の上に作る
  （Structural Decisions 1・15）。
- この Plan の承認の後に `main` へ入った構成の変更（#252〜#263: `internal/app` の新設、
  store の役割ごとの型への分割、一覧の部品の `web/src/videoList/` への移動）に合わせて
  改訂した。改訂で足した判断は Structural Decisions 13〜15 で、テーブル・API・URL の
  契約は変えていない。同じ改訂で、名前の検証で制御文字を空白の除去より先に調べる順に
  直した（[data-model.md §2](data-model.md#2-名前の規則)）。前後に改行やタブを持つ名前は、
  API でも誤りになる。
- マイグレーションを1つ足す（`00008_tags.sql`、[data-model.md §1](data-model.md#1-マイグレーション)）。
  既存の表には触れない。
- 規模の前提は既存と同じ1万本である。一括の付け外しは、1万本を1回の要求と1つの
  トランザクションで扱える形にする（[contracts/tags-api.md §4](contracts/tags-api.md#4-付与と取り外し)）。
  タグの数は数百までを想定し、タグの一覧は1回で全件を返す（ページングしない）。
- 追加する依存は無い。候補の選択（combobox）も確認の窓も、既存の部品と手書きの ARIA で
  作る（Structural Decisions 9）。
- `ui` Issue なので、見た目と操作の配置は、この Plan のあとの design 工程で
  `ui-design.md` に決める。この Plan は、画面が守る API と URL の契約までを決める。

## Constitution Check

- **依存方向**（ARCHITECTURE.md「Intended dependency direction」）: 合格。名前の整え方、
  タグと付与の値の型、誤りの種類は `internal/domain` に置く。SQL は `internal/store` の
  役割の型（Structural Decisions 13）、経路は `internal/httpapi` で、`httpapi` は自分が使う
  interface を宣言し、組み立ての根の `cmd/mdm` が役割の型を渡す。タグの操作は
  `internal/app` を通さない（Structural Decisions 14）。兄弟のパッケージを import しない
  ことは depguard が検査する。
- **store の役割の型**（ARCHITECTURE.md の `store.DB` の節）: 合格。業務の操作は役割の型の
  メソッドに置き、型どうしは互いの公開メソッドを呼ばない。複数の型が使う SQL は
  パッケージ内の非公開の関数で共有する（Structural Decisions 13）。
- **ドメインイベント**（ARCHITECTURE.md・`internal/domain/event.go`）: 合格。タグの変更には
  副作用（生成物の削除、ワーカーの起床、`/api/events`）が無いので、イベントを発行しない
  （Structural Decisions 7・13）。
- **API の正本**（AGENTS.md・ARCHITECTURE.md）: 合格。経路・スキーマ・誤りの `code` は
  `api/openapi.yaml` に足し、`task generate` で生成する。生成物は手で編集しない。
  本文を持つ経路は `requiresJSONBody` にも足し、`openapi_routes_test.go` が対応を検査する。
- **索引と利用者データの区別**（00002_core.sql・ARCHITECTURE.md）: 合格。足す表は
  すべて利用者データで、`videos` に外部キーを張らず `content_key` で結ぶ。再スキャンと
  メディアフォルダの変更の経路は、これらの表に書かない。マイグレーションは新しい
  ファイルとして足す。
- **UI の正本**（library-ui.md・`web/src/theme/tokens.test.ts`）: 合格の見込み。色と大きさは
  トークンだけを使う。構図は design 工程で決め、実装では `ui-design.md` の観点で視覚・操作・
  支援技術を確かめる。
- **裏側の無い入口を増やさない**（library-ui.md §7）: 合格。サイドバーの「タグ」は、
  管理画面ができる単位で初めて足す。
- **サーバーと話す場所**（ARCHITECTURE.md「Web layer」）: 合格。取得と変更の関数、
  タグの一覧の共有は `web/src/api/` に置き、画面は `fetch` しない。

Phase 1 のあとも判定は同じで、正当化の要る違反は無い。

## Structural Decisions

1. **タグ絞り込みは、#195 の一覧の組み立てと条件の URL に1つの条件として足す。入口は
   タグを押す操作で、絞り込み中のタグは一覧の上に並べる。** サーバーは `listing.go` の
   条件にタグの AND を足し、画面はライブラリの条件に `tag` を足す（置き場所は
   Structural Decisions 15）。控えの鍵・履歴・「条件を解除」も #195 の仕組みをそのまま使う。
   - 却下: タグの絞り込みを別の経路や別の URL の読み書きとして持つ案。検索語・視聴状態・
     再生可否との AND（要件 5）、件数、控えの鍵を2か所で合わせることになる。
   - 却下: タグを押すとタグ名を検索欄の語として足す案。検索欄は部分一致の全文検索なので、
     題名にその言葉を含むだけの動画も出る。親 Issue はタグ絞り込みを検索欄と独立させ、
     そのタグが付いた動画だけにすることを求める（要件 5）。
2. **名前とシノニムを1つの表 `tag_names` に置き、名前を主キーにする。** 「元の名前と
   シノニムを通して同じ名前は1つ」を SQLite の一意性で保証する
   （[data-model.md §1](data-model.md#1-マイグレーション)）。
   - 却下: `tags.name` と別表 `tag_synonyms.name` に分ける案。2つの表にまたがる一意性を
     アプリの検査で守ることになり、作成・改名・シノニム登録の3か所で同じ検査が要る。
3. **付与は `content_key` とタグの組で持つ。** 再生位置と同じく、動画の行が消えても
   付与は残り、同じ中身が戻れば付与も戻る（要件 9、Edge Case「ファイルが見えなくなった
   動画」）。
   - 却下: `videos.id` で持つ案。最後の所在が消えると動画の行が消え、戻ったときは別の
     `id` になるので、要件 9 を満たさない。
4. **付け外しは1本でも複数本でも `POST /api/video-tags` の1つの経路にし、動画の `id` の
   集合を受ける。** 「すべて選択」は、同じ条件の全件の `id` を返す `GET /api/videos/ids`
   で集合に変える（[contracts/tags-api.md §4・§5](contracts/tags-api.md#4-付与と取り外し)）。
   選択は画面の中で常に `id` の集合として持つ。
   - 却下: 選択を「一覧の条件 + 除いた `id`」で表し、サーバーが実行時に条件を評価する案。
     選択バーに出した件数と、実際に付いた本数が、その間の取り込みで食い違う
     （受け入れ条件 4）。付け外し・要約・件数表示のすべてが条件の解釈を持つことになる。
   - 却下: 「すべて選択」で画面が残りのページを全部読む案。1万本では 60 件ずつ 167 回の
     要求になり、#195 が消す「全ページを読みに行く処理」を戻すことになる。
   - 却下: 再生画面用の `PUT /api/videos/{id}/tags/{tagId}` を別に置く案。1本は
     集合の特別な場合で、同じ意味の経路を2つ保守することになる。
5. **動画のタグは `Video.tags` として、名前ごと一覧と1件の応答に載せる。** サーバーは
   ページの項目の `content_key` でまとめて引き、`progressFor` と同じ位置で項目に足す。
   - 却下: 項目には `id` だけを載せ、画面がタグの一覧から名前を引く案。統合で消えた `id`
     を持つ項目が残り、それを直すために変更を配る仕組みが要る。
   - 却下: タグをページごとの別の要求で取る案。一覧の1ページが2回の往復になり、
     復元の控えも2つの応答を合わせることになる。
6. **絞り込みの URL にはタグの `id` を載せる**（[contracts/list-url.md](contracts/list-url.md)）。
   - 却下: 名前を載せる案。改名で古い URL が効かなくなり、シノニムの名前と元の名前で
     同じ条件が2つの URL を持つ。
7. **画面の更新は、付け外しは読み込み済みの項目と控えの差し替え、管理画面の変更は控えの
   破棄とタグの一覧の取り直しで行う。** 付け外しの結果は、再生位置の
   `progressEvents` と同じ形の通知で、読み込み済みの一覧の項目と
   `listSnapshot` の控えに反映する。改名・削除・統合・シノニムの変更は、メディア
   フォルダの変更と同じく `clearListSnapshot()` を呼ぶ。SSE のイベントは足さない。
   - 却下: 付け外しの後に一覧を読み直す案。選択バーで付けるたびにスクロールと
     読み込み済みのページを失う。
   - 却下: タグの変更を SSE で全タブへ配る案。親 Issue が別のタブに求めるのは、古い
     タグを使ったときに「もう無い」と伝えて取り直すことだけで
     （Edge Case「ほかの画面での並行した変更」）。付け外しと管理の操作では
     `tag_not_found`、絞り込みでは一覧の応答の `missingTagIds` で、古いタグを使った
     その時点に分かる（[contracts/tags-api.md §5](contracts/tags-api.md#5-一覧の絞り込みとすべて選択)）。
   付け外しで一覧の絞り込みに合わなくなった項目は、#195 の視聴状態と同じく、その場では
   一覧から外さず、次の読み込みで反映する。
8. **タグの一覧は `web/src/api/` の1つの共有の保持で持ち、候補・絞り込み中のタグの
   確かめ・管理画面が同じものを読む。** 画面が開くとき、タグを変えたとき、`tag_not_found` か
   `missingTagIds` を受けたときに取り直す。
   - 却下: 画面ごとに `GET /api/tags` を持つ案。管理画面で作ったタグが、開いたままの
     再生画面の候補に出ないなど、同じタブの中で食い違う（受け入れ条件 9）。
9. **候補の選択（combobox）は依存を足さずに `web/src/ui/` に手で作る。確認の窓は
   `settings/FolderPicker.tsx` の `ModalFrame` を `web/src/ui/` へ移して使う。**
   - 却下: combobox のライブラリ（downshift・cmdk など）を足す案。使う部品は1つで、
     入っている Radix には combobox が無い。矢印キーと Enter の操作（UI品質の
     「操作の優先順位」）は ARIA 1.2 の combobox の型で足りる。
   - 却下: 管理画面に確認の窓を別に作る案。フォーカスの戻し方と Esc の扱いが2つになる。
10. **共有の動画カード（`web/src/videoList/VideoCard.tsx`）の題名の下の行は、呼び出し側が
    選ぶ。ライブラリはタグの行、フォルダ画面は今のメタ情報の行のままにする。** 親 Issue は要件 3 でライブラリ一覧のカードだけを変え、
    「フォルダ画面でのタグ表示」を対象外にしている。`Video.tags` はどの経路でも返るが
    （[contracts/tags-api.md §1](contracts/tags-api.md#1-スキーマ)）、フォルダ画面は描かない。
    カードは今、全体が再生画面への1つのリンクなので、押せるタグはリンクの外に置く
    （リンクの中に押せる要素を入れ子にしない）。ライブラリの表形式の行（`VideoRow`）は、
    親 Issue がカードだけを挙げているので変えない。
    - 却下: 共有の `VideoCard` ごと、フォルダ画面のカードにもタグを出す案。カードの形は
      1つで済むが、親 Issue が対象外とした表示を足すことになる。
    - 却下: フォルダ画面のために別のカードの部品を作る案。hover プレビューと選択の
      振る舞いを2か所で保守することになる。
11. **統合では、統合元の元の名前も統合先のシノニムにする**
    （[data-model.md §4](data-model.md#4-書き換えの規則)）。統合は同じものを指す2つの
    タグを1つにする操作なので、統合元の名前で付けたり絞り込んだりしても統合先として
    扱われるのが自然である。シノニム登録に伴う統合（受け入れ条件 17）も、同じ1つの
    書き換えで済む。統合元は一覧からは消える（受け入れ条件 12）。
    - 却下: 統合元の元の名前を消し、シノニム登録に伴う統合だけがその名前をシノニムにする案。
      名前をふさがない利点は、あとからシノニムを解除すれば得られる。一方で、統合元の名前を
      覚えている利用者がその名前で付けると、統合したはずの分類が別のタグとして作り直される。
12. **検索欄のタグ名の照合は、語ごとの条件に「タグ名の鍵を持つ行がある」の OR を足して
    行う。タグ名の照合形の鍵は `tag_names` に持つ**
    （[data-model.md §7](data-model.md#7-検索欄でのタグ名の照合)）。
    - 却下: 動画のタグ名を、所在ごとの `search_key` に書き足す案。付け外し・改名・統合の
      たびに、その中身のすべての所在の鍵と全文索引を書き直すことになる。選択バーから
      1万本へ付けると、1回の操作で1万行以上の索引を書き換える。
    - 却下: タグ名にも全文索引を張る案。タグ名は数百までで、`instr` でなめても間に合う。
13. **タグの書き換えと付与は、新しい役割の型 `TagStore`（`db.Tags()`）に置く。タグで
    絞った一覧・全件の `id`・検索欄のタグ名の照合は、一覧の問い合わせの一部として
    `LibraryStore` に置く。**
    - `TagStore` が持つ操作: タグの作成・改名・削除・統合・シノニムの登録と解除・本数つきの
      一覧、動画の `id` の集合への付け外し、選んだ動画の要約、`content_key` の集合から項目の
      タグを引く操作（`PlaybackStore.ProgressByContentKeys` と同じ形）、起動時のタグ名の鍵の
      作り直し。`PlaybackStore` と同じく、索引の役割の型にも知らせの発行にも依存しない。
      本数・要約・`id` からの引き直しのために `videos` を SQL で読む（下の共有の関数）。
    - `LibraryStore` が持つ操作: `listing.go` のタグの AND、全件の `id`、`search.go` の語ごとの
      条件のタグ名の OR。どれも1つの問い合わせの中の条件なので、ほかの型を呼ばずに
      `video_tags` と `tag_names` を SQL で読む（視聴状態の条件が `playback_progress` を読むのと同じ）。
    - 両方が使う SQL（`id` からいまライブラリにある動画の `content_key` を引く、ライブラリに
      ある動画だけを数える）は、`registeredVideoCondition` のようなパッケージ内の非公開の
      関数で共有する（`internal/store/roles.go` の決まり）。
    - 起動時は、`cmd/mdm` が `LibraryStore.RefreshSearchKeys` の隣で `TagStore` のタグ名の
      鍵の作り直しを呼ぶ。所在の鍵と違ってメディアフォルダに依らないので、`folderMu` は取らない。
    - 却下: タグの操作を `LibraryStore` に足す案。`LibraryStore` は作り直せる索引の読み出しの型で、
      作り直せない利用者データの書き換えを混ぜると、型が何を持つのかの線が消える。
    - 却下: `PlaybackStore` に足す案。再生位置とタグは別の利用者データで、どちらの
      変更も他方に関わらない。
14. **タグの操作は `internal/app` を通さず、`internal/httpapi` が宣言する interface から
    `TagStore` と `LibraryStore` を直接呼ぶ。** どの操作も1つのトランザクションで済み、規則は
    `internal/domain` の名前の整え方と `internal/store` の SQL にある。ファイル・`ffmpeg`・
    イベントの発行など、アプリケーション層が組み合わせる相手が無い。ARCHITECTURE.md は
    `httpapi` が store を直接呼ぶことを認めている（再生位置と同じ）。`Video.tags` を項目に
    足すのも、`progressFor` と同じく `httpapi` で行う（Structural Decisions 5）。
    - 却下: `internal/app` に `Tags` のユースケースを置く案。どのメソッドも store の1回の
      呼び出しをそのまま渡すだけになり、同じ interface を `app` と `httpapi` の2か所で
      宣言して保守することになる。
15. **Web のタグ絞り込みの条件はライブラリが持ち、共有の `web/src/videoList/` の
    `ListCriteria` には `tag` を足さない。** `web/src/library/` に `tag` の読み書き
    （並びの正規化・16 個の上限、[contracts/list-url.md §1](contracts/list-url.md#1-パラメータ)）と、
    `ListCriteria` に `tag` を組み合わせる関数を置く。共有の部品には、画面の固有の条件を
    運ぶための次の口だけを足す。
    - `useListCriteria` は、呼び出し側が指定した画面の固有のパラメータの名前を受け取り、
      その値を読んで返す。中身は解釈しない。URL を書くすべての経路（`apply`、`sort=random` の
      `seed` を補う置き換え、並べ替えの push の前の置き換え）で、その値を残す。`apply` は
      その値の差し替えを任意で受け取り、ライブラリはこれで `tag` を書く（タグを足す・外す
      ときの push、`missingTagIds` を取り除く replace、「条件を解除」での消去）。
    - 「条件を解除」を出すか、0 件のときに「該当なし」と「ライブラリが空」のどちらを出すか、
      「条件を解除」で何を外すかは、ライブラリでは `tag` を含めて決める。`tagCriteria.ts` が
      共有の `hasConditions`・`clearConditions` に `tag` を足した版を持ち、`LibraryPage` はそれを使う。
    - 一致なしの条件のチップは、フォルダ画面が範囲のチップを足すのと同じく、ライブラリが
      `conditionLabels` の結果にタグのチップを足す。
    - 一覧の控えの鍵（`web/src/api/listSnapshot.ts` の `ListKey`）には、任意の `tags` を足す。
    - 却下: 共有の `ListCriteria` に `tag` を足す案。フォルダ画面も `tag` を読み、条件を
      書き換えるたびに URL へ残すことになり、フォルダ画面の URL を変えない約束
      （contracts/list-url.md）と、フォルダ画面にタグ絞り込みを置かない対象外の範囲に反する。
      `hasConditions`・`clearConditions` の意味も画面ごとに分かれる。
    - 却下: ライブラリだけの URL の読み書きの hook を別に作る案。`sort=random` の `seed` の
      補い方と、並べ替えの履歴の扱い（#195 の list-url.md §3）を2か所で保守することになる。

## Project Structure

### Documentation (this feature)

```text
specs/014-video-tags/
├── plan.md
├── data-model.md
└── contracts/
    ├── tags-api.md
    └── list-url.md
```

`ui-design.md` は、この Plan のあとの design 工程で足す。判断は上の Structural Decisions で
閉じており、別に調べて決めることが無いので `research.md` は作らない。検証は各実装単位の
自動テストと e2e で行い、それ以外に手で走らせる手順が無いので `quickstart.md` も作らない。
この workflow は `tasks.md` を作らない。下の Implementation Work を子 Issue にする。

### Source Code

**Affected boundaries**:

- `internal/domain/`: 名前の整え方、`Tag`・`TagRef`・付与の値、誤り、`VideoQuery` のタグ
- `internal/store/`: マイグレーション、役割の型 `TagStore`（タグの書き換え、本数、付け外し、
  要約、項目のタグ、タグ名の鍵の作り直し）、`LibraryStore` の一覧の条件と全件の `id`
  （Structural Decisions 13）
- `api/openapi.yaml` と生成物: [contracts/tags-api.md](contracts/tags-api.md) の差分
- `internal/httpapi/`: タグの経路、`Video.tags`、一覧の `tag`、`/api/videos/ids`
- `cmd/mdm/`: 新しい interface への `TagStore` の受け渡し、起動時のタグ名の鍵の作り直しの呼び出し
- `web/src/api/`: 取得と変更の関数、タグの一覧の共有、付け外しの通知、控えの差し替え
- `web/src/ui/`: combobox、`ModalFrame` の移設
- `web/src/player/`: タグの表示と付け外し、タグを押したときの絞り込み
- `web/src/videoList/`: 共有のカードの題名の下の行を選ぶ口、`useListCriteria` の画面の固有の
  パラメータの口（Structural Decisions 10・15）
- `web/src/library/`: カードのタグの行、タグ絞り込みの条件と絞り込み中のタグの並び、選択バーの操作、「すべて選択」
- `web/src/tags/`（新設）: タグ管理画面
- `web/src/shell/`・`web/src/app/`: サイドバーの入口と経路
- `web/e2e/`: 実ブラウザでの検証
- `ARCHITECTURE.md`: 利用者データの表、`store.DB` の節の役割の型の一覧、API の経路の一覧、
  Web のディレクトリ・経路・サイドバーの入口
- `docs/design-docs/library-ui.md`: §6 のカードと選択バー、§8 の再生画面の構成要素

**New paths**:

- `internal/domain/tag.go` と対応 test
- `internal/store/migrations/00008_tags.sql`
- `internal/store/tags.go` と対応 test（`TagStore`。型の宣言と `db.Tags()` は `roles.go` に足す）
- `internal/httpapi/tags.go` と対応 test
- `web/src/api/tags.ts` と対応 test（取得・変更の関数、共有の保持、付け外しの通知）
- `web/src/library/tagCriteria.ts` と対応 test（URL の `tag` の読み書きと、`ListCriteria` との組み合わせ）
- `web/src/ui/Combobox.tsx` と対応 test、`web/src/ui/ModalFrame.tsx`
- `web/src/tags/`（管理画面）
- `web/e2e/tags.e2e.ts`

**Structure decision**: 既存の配置に従う（[ARCHITECTURE.md](../../ARCHITECTURE.md)）。
タグ管理画面は、設定やフォルダ画面と同じく経路ごとのディレクトリ `web/src/tags/` に置く。

## Implementation Work

### タグを保存し、作成・改名・削除・統合・シノニムの規則をサーバーに置く

**Scope**: `00008_tags.sql` を足す（[data-model.md §1](data-model.md#1-マイグレーション)）。
`internal/domain` に名前の整え方と誤り（[data-model.md §2](data-model.md#2-名前の規則)）、
新しい役割の型 `TagStore`（`internal/store/roles.go` に宣言と `db.Tags()`、Structural Decisions 13）の
メソッドとして、`internal/store/tags.go` に作成・改名・削除・統合・シノニムの登録と解除・本数つきの一覧を
置く（[data-model.md §3〜§5](data-model.md#3-名前の引き方)）。名前の行を書くときに照合用の鍵を
書き、起動時に古い版の鍵を作り直す処理を `TagStore` に置いて、`cmd/mdm` が
`LibraryStore.RefreshSearchKeys` の隣で呼ぶ
（[data-model.md §7](data-model.md#7-検索欄でのタグ名の照合)）。不変条件の検査を
`invariants_test.go` に足す。`ARCHITECTURE.md` の利用者データの記述と `store.DB` の節の
役割の型の一覧、`docs/design-docs/tech-stack-selection.md` §3.2 の表の一覧に3つの表を足す。

**Dependencies**: None

**Acceptance**: `go test ./internal/domain/... ./internal/store/...` で次が通る。前後の空白は
取れ、空白だけの名前、改行やタブを含む名前（`"旅行\n"`・`"\t旅行"` のように前後にあるものを
含む）、101 符号位置の名前は誤りになる。`Anime` があるとき `anime` は別の
タグとして作れる。既存の名前・シノニムへの改名は `ErrTagNameTaken` になる。X を Y へ統合
すると、X の付いていた中身すべてに Y が1つだけ付き、X の元の名前とシノニムが Y のシノニムに
なり、X が一覧から消える。その後 X の名前で付けると Y が付く。
`anime`（10 本）を `Anime` のシノニムにすると、統合元の `id` の承諾が無いか `anime` の `id` と
違えば `ErrTagMergeRequired` で何も変わらず、`anime` の `id` なら 10 本に `Anime` が付いて
`anime` が `Anime` のシノニムになる。別の
タグのシノニムを登録すると `ErrTagNameTaken` になり、そのタグが分かる。本数は、所在が
消えた動画を数えず、削除・統合はその動画の付与にも及ぶ。統合の途中で失敗させると何も
変わっていない。作成・改名・シノニム登録の後、その行の `search_key` が `FoldForMatch` の
結果になっている。`search_version` を 0 に戻した行が、起動時の作り直しで埋まる。
00007 の状態から Up・Down が通る。depguard を含む `task lint` が通る。`scripts/migrations-immutable.sh` と
`task check` が通る。

### 動画へのタグの付け外しと、タグでの一覧の絞り込み・検索をサーバーに置く

**Scope**: `TagStore`（`internal/store/tags.go`）に、動画の `id` の集合への付与・取り外し（名前での付与は
引いて無ければ作る）、選んだ動画のタグの要約、`content_key` の集合から項目のタグを引く
操作を置く（[data-model.md §4](data-model.md#4-書き換えの規則)）。次の条件と全件の `id` は
`LibraryStore` に置き、`id` から `content_key` を引く SQL は2つの型で非公開の関数として共有する
（Structural Decisions 13）。`internal/store/search.go` の
語ごとの条件に、タグ名の照合の OR を足す（[data-model.md §7](data-model.md#7-検索欄でのタグ名の照合)）。#195 の
`internal/store/listing.go` にタグの AND の条件を足し（[data-model.md §6](data-model.md#6-タグでの絞り込み)）、
同じ条件で全件の `id` を返す関数を足す。`VideoQuery` にタグを足す。`ARCHITECTURE.md` の
`store.DB` の節の `LibraryStore` の記述に、タグの条件を足す。

**Dependencies**: タグを保存し、作成・改名・削除・統合・シノニムの規則をサーバーに置く

**Acceptance**: `internal/store` のテストで次が通る。3 本に付けると 3 本すべてに付き、
既に付いていた動画があっても誤りにならない。付いていないタグを外しても誤りにならない。
シノニムの名前で付けると元のタグが付く。途中で失敗させると1本にも付いていない。要約で、
1 本だけに付いたタグの数が 1、全体が 3 になる。タグ A と B で絞ると両方を持つ動画だけが
返り、`Total` もその数になり、検索語・視聴状態と組み合わさる。無い `tag_id` は無視され、
どれを無視したかが返る。
全件の `id` が、同じ条件の全ページの `id` と一致する。所在を移して再スキャンした動画に
同じタグが付いている。題名に「旅行」を含まない動画に「旅行」タグを付けると、検索語 `旅行` で
ライブラリ・フォルダ直下・フォルダ配下のどの範囲でも返り、`-旅行` では返らない。シノニムの名前
でも返り、改名の後は新しい名前で返って古い名前では返らない。`ﾘｮｺｳ` と `りょこう` のように
照合形が同じ語は同じ結果になる。既存の検索のテストが通る。`task check` が通る。

### タグの管理 API を公開する

**Scope**: `api/openapi.yaml` に `Tag` と誤りの `code`、[contracts/tags-api.md §1〜§3](contracts/tags-api.md#3-タグの管理)
の経路を足し、`task generate` で生成する。`internal/httpapi/tags.go` に経路を置き、
`requiresJSONBody` に本文を持つ経路を足す（`PATCH` の分岐は今は無いので足す）。
`internal/httpapi` がタグの管理の interface を宣言し、`cmd/mdm` が `db.Tags()` を渡す
（`internal/app` は通さない。Structural Decisions 14）。`ARCHITECTURE.md` の API の経路の一覧に `/api/tags*` を足す。`web/src/api/tags.ts`
に取得・変更の関数と、タグの一覧の共有の保持（Structural Decisions 8）を置き、管理の
変更で `clearListSnapshot()` を呼ぶ（画面はまだ使わない）。

**Dependencies**: タグを保存し、作成・改名・削除・統合・シノニムの規則をサーバーに置く

**Acceptance**: `internal/httpapi` のテストで次が通る。`POST /api/tags` が 201 を返し、同じ
名前で 409 `tag_name_taken` を返す。無いタグの `PATCH`・`DELETE`・`merge` が 404
`tag_not_found` を返す。`mergeTagId` 無しで、または別のタグの `id` で既存のタグ名を
シノニムにすると 409 `tag_merge_required` を返す。別オリジンからの変更は 403 になる。`openapi_routes_test.go`
が通る。Vitest で、変更の成功後に共有の一覧が取り直され、控えが消える。`task generate` を
走らせ直しても差分が出ない。`task check` が通る。

### 動画のタグ・付け外し・タグの絞り込みを API で公開する

**Scope**: `Video.tags`、`POST /api/video-tags`、`POST /api/video-tags/summary`、`listVideos`
の `tag`、`GET /api/videos/ids` を `api/openapi.yaml` に足して生成する
（[contracts/tags-api.md §1・§4・§5](contracts/tags-api.md#4-付与と取り外し)）。`Video` を返す5つの経路
すべてに、`progressFor` と同じく `internal/httpapi` で `TagStore` から引いた項目のタグを載せ
（Structural Decisions 5・14）、一覧の応答に `missingTagIds` を載せる。
`ARCHITECTURE.md` の API の経路の一覧に `/api/video-tags*` と `/api/videos/ids` を足す。`web/src/api/` に付け外し・要約・
全件の `id` の関数と、付け外しの結果を読み込み済みの項目と控えへ反映する通知を足す
（Structural Decisions 7）。`useVideos` に `tag` の条件を渡せるようにする（画面はまだ使わない）。

**Dependencies**: 動画へのタグの付け外しと、タグでの一覧の絞り込み・検索をサーバーに置く。
タグの管理 API を公開する

**Acceptance**: `internal/httpapi` のテストで次が通る。`GET /api/videos?tag=1&tag=2` が両方を
持つ項目と `total` を返し、各項目の `tags` が名前の自然順になる。無い `id` を混ぜると
`missingTagIds` にそれが入る。`GET /api/videos?query=旅行` と
`GET /api/folders/{rootId}/videos?scope=subtree&query=旅行` が、題名に「旅行」を含まず
「旅行」タグを持つ動画を返す。関連動画と読み取りのやり直しの応答にも `tags` があり、タグの
無い動画では空の配列になる。17 個の `tag` は 400 になる。
`GET /api/videos/ids` が同じ条件の全件の `id` を返し、`/api/videos/{id}` に取られない。無い
`tag.id` での付与は 404 `tag_not_found` になる。Vitest で、付け外しの後に読み込み済みの項目と
控えの `tags` が変わり、続きのページの取得条件に `tag` が載る。`task generate` を走らせ直しても
差分が出ない。`task check` が通る。

### 再生画面で動画のタグを見て、付けたり外したりできるようにする

**Scope**: 再生画面に、その動画のタグと、外す操作と、「タグを追加」の combobox を置く。
combobox は `web/src/ui/Combobox.tsx` に作る（Structural Decisions 9）。候補は共有の
タグの一覧から、元の名前とシノニムの両方で引き、どれにも一致しない名前は新しいタグとして
付ける。`tag_not_found` では、もう無いことを伝えて一覧を取り直す。
`docs/design-docs/library-ui.md` §8 の構成要素にタグを足す。配置と見た目は
`ui-design.md` による。

**Dependencies**: 動画のタグ・付け外し・タグの絞り込みを API で公開する。design 工程の
`ui-design.md` が feature ブランチに入っていること

**Acceptance**: Vitest と `web/e2e/tags.e2e.ts` で、受け入れ条件 1・2・6・13 が再生画面で
確かめられる（13 のシノニムは API で登録しておく）。キーボードだけで「タグを追加」に届き、矢印キーで候補を選んで Enter で
付けられる。外すボタンの名前が「<タグ名>をこの動画から外す」の形で読み上げられる。空白だけの入力、改行を
含む入力、101 文字の入力は確定できず、理由が出る。`task check` と `task test-e2e` が通る。`ui-design.md` の観点で視覚・操作・支援技術を確かめる。

### 一覧のカードにタグを出し、タグを押してタグで一覧を絞り込めるようにする

**Scope**: ライブラリのカード（共有の `web/src/videoList/VideoCard.tsx`）の題名の下の「追加日時 · ファイルサイズ · コーデック」の
行を、その動画のタグの1行に置き換え、各タグを押せるようにする。フォルダ画面のカードは今の
行のままにする（Structural Decisions 10）。`docs/design-docs/library-ui.md` §6 のカードの
一次情報を改める。[contracts/list-url.md](contracts/list-url.md) の `tag` を、ライブラリの
`web/src/library/tagCriteria.ts` と `listSnapshot` の鍵に足し、`useListCriteria` に画面の固有の
パラメータを残す口を足す。共有の `ListCriteria` には足さない（Structural Decisions 1・15）。カードのタグを押すと
今の条件にそのタグを加え、再生画面のタグを押すとそのタグ1つで絞り込んだ一覧を開く。
一覧の上に絞り込み中のタグを並べ、それぞれ1回で外せるようにする。件数と「条件を解除」に
タグ絞り込みを含める。選択中はカードのタグを押しても絞り込まない。16 個を超えるときは
加えずに伝える。一覧の応答の `missingTagIds` と、控えから戻したときの突き合わせで、もう
無いことを伝え、タグの一覧を取り直し、URL から取り除く。配置と見た目は `ui-design.md` による。

**Dependencies**: 再生画面で動画のタグを見て、付けたり外したりできるようにする（再生画面の
タグの表示に押す操作を足し、共有のタグの一覧を使う）

**Acceptance**: Vitest で、URL の `tag` の解釈（並びの正規化・重複・数でない値・17 個目）と、
押したときの足し方（同じタグで変わらない・17 個目は足さずに伝える・ほかの条件を残す）、
外したときにその `id` だけが消えること、`missingTagIds` を受けたときの通知・タグの一覧の
取り直し・URL からの取り除きが通る。`/?tag=1` から再生画面へ移って戻ると、`/` の控えでは
なく `/?tag=1` の控えが使われる。再生画面にいる間に別のタブでタグ 1 を消してから戻ると、
もう無いことが伝わり、URL から取り除かれて一覧が取り直される。`web/e2e/tags.e2e.ts` で、
受け入れ条件 5・7・8・19・20 が確かめられる。カードのタグを押しても再生画面へ移らず、
選択中に押すとそのカードの選択が切り替わる。タグを押す操作は「<タグ名> で絞り込む」、
絞り込み中のタグを外す操作は「<タグ名> の絞り込みを外す」の形で読み上げられ、どちらも
キーボードで届く。フォルダ画面のカードと表形式の行は今のままである。フォルダ画面の URL に
`tag` を書いて開き、検索語や絞り込みを変えると、URL から `tag` が消える（フォルダ画面は
`tag` を読まない）。`/?tag=1&sort=random` を開くと `seed` が補われ、`tag=1` は残る。タグだけで
絞って 0 件のとき、「該当なし」の状態にタグのチップが出て、「条件を解除」でタグが外れる。カードのタグが1行に
収まらないとき、省略されていることが分かる。`web/src/theme/tokens.test.ts` を含む
`task check` と `task test-e2e` が通る。`ui-design.md` の観点で視覚・操作・支援技術を確かめる。

### 選択バーから、選んだ動画へタグを一括で付けたり外したりできるようにする

**Scope**: 選択バーの件数の直後に、タグを付ける操作と外す操作を置く。外す候補は
`POST /api/video-tags/summary` から作り、一部にだけ付いたタグを文言かアイコンで示す。
「すべて選択」を `GET /api/videos/ids` による全件の選択に変え、読み込み済みにない `id` を
選択から落とさないようにする（Structural Decisions 4）。応答に `missingTagIds` があれば
選択を作らず、一覧と同じく伝えて直す（[contracts/tags-api.md §5](contracts/tags-api.md#5-一覧の絞り込みとすべて選択)）。失敗したときと `tag_not_found` を受けたときは、選択を保ったまま
伝え、`tag_not_found` ではタグの一覧を取り直す。`docs/design-docs/library-ui.md` §6 の
選択バーの記述を改める。配置と見た目は `ui-design.md` による。

**Dependencies**: 一覧のカードにタグを出し、タグを押してタグで一覧を絞り込めるようにする

**Acceptance**: Vitest と `web/e2e/tags.e2e.ts` で、受け入れ条件 3・4 が確かめられる。
付けたあと、読み込み済みのカードにタグがすぐ出る。要求を失敗させると、選択が残ったまま
失敗が伝わり、もう一度押すと付く。絞り込み中のタグを別のタブで消してから「すべて選択」
すると、何も選ばれず、もう無いことが伝わる。`task check` と `task test-e2e` が通る。`ui-design.md` の観点で視覚・操作・支援技術を確かめる。

### サイドバーから開くタグ管理画面で、タグを一覧し、作成・改名・削除する

**Scope**: `web/src/tags/` に `/tags` の管理画面を置き、`shell/navigation.ts` に「タグ」の
入口を、`app/App.tsx` に経路を足す。一覧（本数つき・本数 0 を含む）、一覧の中のタグの検索
（共有のタグの一覧を画面の中で絞る。API は足さない）、新規作成、改名、
削除（外れる本数の確認つき）、タグを選んで `/?tag=<id>` へ移る操作を置く
（[contracts/list-url.md §2](contracts/list-url.md#2-タグを押したときと外したとき)）。確認の窓のため
`ModalFrame` を `web/src/ui/` へ移す（Structural Decisions 9）。タグが無いときの空の状態を
置く。検索で一致が無いときの表示も置く。`ARCHITECTURE.md` の Web のディレクトリ・経路・サイドバーの入口の記述を改める。
配置と見た目は `ui-design.md` による。

**Dependencies**: 一覧のカードにタグを出し、タグを押してタグで一覧を絞り込めるようにする
（移った先のタグ絞り込みを使う）

**Acceptance**: Vitest と `web/e2e/tags.e2e.ts` で、受け入れ条件 9・10・11・14・16・18 が
確かめられる。キーボードだけで検索の入力に届く。検索で一致が無いときは、タグが無いときの空の状態と別の表示になり、そこから
入力を消せる。既存の名前やシノニムへの改名で、理由が画面に出る。別のタブで消したタグを
改名しようとすると、もう無いことが伝わり一覧が取り直される。削除は作成と改名より
目立たない。`web/src/settings/` のフォルダ選択と削除の確認が、移した `ModalFrame` で
今までどおり動く。`task check` と `task test-e2e` が通る。`ui-design.md` の観点で視覚・操作・支援技術を確かめる。

### タグ管理画面で、タグを統合し、シノニムを登録・解除する

**Scope**: 管理画面に、統合元と統合先を選ぶ統合（影響する本数の確認つき）と、シノニムの
登録と解除を足す。登録しようとした名前が既存のタグの名前なら、統合することを本数と一緒に
示して確認をとり、承諾したときだけ確認に出したタグの `id` を `mergeTagId` に入れて送る。
`tag_merge_required` を受けたら、タグの一覧を取り直して確認をやり直す
（[contracts/tags-api.md §3](contracts/tags-api.md#3-タグの管理)）。別のタグのシノニムとの
衝突は、どのタグのシノニムかを示す。配置と見た目は `ui-design.md` による。

**Dependencies**: サイドバーから開くタグ管理画面で、タグを一覧し、作成・改名・削除する

**Acceptance**: Vitest と `web/e2e/tags.e2e.ts` で、受け入れ条件 12・17 が
確かめられる。統合すると、統合元の名前が統合先のシノニムとして並ぶ。取り消すと何も変わらない。別のタグのシノニムと同じ名前の登録で、そのタグの
名前が画面に出る。確認の後に別のタブでその名前が別のタグへ移っていると、統合されずに
確認がやり直しになる。統合は作成と改名より目立たない。`task check` と `task test-e2e` が通る。
`ui-design.md` の観点で視覚・操作・支援技術を確かめる。
