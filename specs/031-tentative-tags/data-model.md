# Data model: 仮のタグと却下した名前

親 Issue: #589。

既存の表の定義は [internal/store/migrations/](../../internal/store/migrations/) が正本で、タグの表と
名前の規則・書き換えの規則は [specs/014-video-tags/data-model.md](../014-video-tags/data-model.md)、
フォルダ由来のタグは [specs/017-folder-groups/data-model.md §4](../017-folder-groups/data-model.md#4-フォルダ由来のタグ)、
データの区分は [ARCHITECTURE.md](../../ARCHITECTURE.md)「Rebuildable and user data」にある。
ここには、この feature が足す列と表、それを読み書きする規則だけを書く。書いていない表は変えない
（`tag_names`・`video_tags` はそのまま。付け外し・絞り込み・検索・本数の SQL は変えない）。

## 1. マイグレーション

`00022_tentative_tags.sql` として足す（`main` の最後は `00021_video_overrides.sql`）。

```sql
-- 仮のタグ（specs/031-tentative-tags/research.md R-1）。自動の付与で新しく作られたタグは 1、
-- 手で作ったタグと導入前のタグは 0。確定は 0 に書き換えるだけで、名前と付与は変えない。
alter table tags add column tentative integer not null default 0 check (tentative in (0, 1));

-- 却下した名前（R-2）。仮のタグを却下したとき、そのタグの元の名前を覚え、以後の仮の作成で
-- 飛ばす。タグでも付与でもない利用者データで、名前の照合は tag_names.name と同じ完全一致。
create table rejected_tag_names (
    name       text    primary key,
    created_at integer not null
) without rowid;
```

Down は `rejected_tag_names` を落とし、`tags.tentative` を `alter table … drop column` で落とす。

不変条件（`internal/store/invariants_test.go` に足す）:

- `tentative = 1` のタグに `canonical = 0` の `tag_names` は無い（仮のタグはシノニムを持たない。
  [R-4](research.md#r-4-仮のタグへの手入れ改名シノニム統合先は同じ取引で確定にする)）。
- `rejected_tag_names.name` と `tag_names.name` に同じ値は無い
  （[R-3](research.md#r-3-名前を-tag_names-に書く取引は同じ名前を-rejected_tag_names-から外す)）。

区分: どちらも作り直せない利用者データ。ARCHITECTURE.md のタグの表の一覧に `rejected_tag_names` を足す。

## 2. `rejected_tag_names`

- 1 行は却下した名前 1 つ。`name` は `domain.NormalizeTagName` を通した形（却下したタグの元の名前は
  常にその形で保存されている）。
- 引くのは `tentative` が真の作成（§3）と、管理画面の一覧（[contracts/screen-api.md §3](contracts/screen-api.md#3-却下した名前)）だけ。
  照合用の鍵（`search_key`）は持たない。名前の検索の対象にしない。
- 一覧の並びは名前の自然順（`domain.SortTagNames`）。

## 3. 書き換えの規則

014 の §4 の表に足す。どの操作も 1 つの取引で行い、途中で失敗したら何も残さない。

| 操作 | 書き換え |
| --- | --- |
| 仮の作成（`tentative` が真の `add`・`replace` で、名前がどのタグの名前にもシノニムにも当たらないとき） | 名前が `rejected_tag_names` にあれば、その名前を飛ばす（作らず、付けず、飛ばした名前として返す）。無ければ `tags` に `tentative = 1` で 1 行、`tag_names` に `canonical = 1` で 1 行。1 つの要求で同じ名前が複数の動画に付くときも作るのは 1 つ（Edge Case） |
| 手での作成（画面の作成、名前での付与での作成、`tentative` が偽の API、グループのタグ化） | 今と同じ（`tentative = 0`）。同じ取引で `rejected_tag_names` からその名前を消す |
| 既存のタグに当たる名前の付与（`tentative` の真偽を問わず） | 今と同じくそのタグを付ける。`tentative` は変えない（要件 2・3、仮のタグへの手での付与も仮のまま） |
| 改名 | 今と同じ。名前が変わるときは、同じ取引で `tentative = 0` にし、新しい名前を `rejected_tag_names` から消す。今と同じ名前なら何も変えない |
| シノニム登録（名前 n をタグ T に） | 今と同じ。n を足すとき（承諾した統合を伴うときとも）は T を `tentative = 0` にし、n を `rejected_tag_names` から消す。n が既に T のシノニムなら何も変えない |
| 統合（元 X → 先 Y） | 今と同じ。Y を `tentative = 0` にする。X は消える（仮でも確定でも） |
| 確定（`ConfirmTag`） | `tentative = 0`。既に 0 なら何も変えない。`tag_names`・`video_tags` は変えない |
| 却下（`RejectTag`） | `tentative = 1` でなければ `domain.ErrTagNotTentative`。元の名前を読み、`tags` の行を消し（`tag_names`・`video_tags` は連鎖して消える）、`rejected_tag_names` に元の名前を `insert or ignore` |
| 削除（`DeleteTag`） | 今と同じ。仮でも確定でも名前を覚えない（画面は仮のタグに削除を出さない。要件 10） |
| 却下した名前の取り外し（`ForgetRejectedTagName`） | `delete from rejected_tag_names where name = ?`。無ければ何も変えない（Edge Case） |

- 「仮の作成」の判定と却下した名前の照合は、名前の引き当て（`lookupTagName`）と同じ取引の中で行う。
  書き込みの取引は 1 つずつ走るので、却下との競合は起きない
  （[R-6](research.md#r-6-却下と同じ名前の仮の付与はsqlite-の書き込みの直列化に任せる)）。
- `tag_names` に名前を書く 2 つの入口（`insertTagName`、改名の `update`）が `rejected_tag_names` の
  削除を持つ。仮の作成もこの入口を通るが、飛ばす判定の後なので消すものは無い（R-3）。
- `remove` はどのタグにも当たらない名前を今と同じく何もしない。`tentative` は受け付けるが読まない。

## 4. `domain` に足す値

| 値 | 中身 |
| --- | --- |
| `TagRef.Tentative` | `bool`。`VideoTag`（`TagRef` を埋め込む）と `TagSummaryItem.Tag` にもこれで載る |
| `Tag.Tentative` | `bool`。管理画面の 1 件 |
| `ErrTagNotTentative` | 仮でないタグを却下しようとした。API では `tag_not_tentative`（409） |
| `VideoTagsOutcome{Items []VideoTagsResult, SkippedNames []string}` | 一括操作の結果。`SkippedNames` は却下した名前に当たって飛ばした名前（整えた形、`tags` の順、重複なし。無ければ空の配列） |

`NormalizeTagName`・`SortTag*` は変えない。仮のタグの名前の規則は確定したタグと同じである。

## 5. 保存層の操作

`TagStore` に足す。すべて共有する SQLite 接続だけを使い、ドメインイベントは発行しない（タグの変更は
副作用を持たない。ARCHITECTURE.md の `TagStore` の段落のまま）。

| 操作 | 1 つの取引で行うこと |
| --- | --- |
| `ApplyVideoTags(ctx, videos, action, names, tentative bool) (VideoTagsOutcome, error)` | 今の `ApplyVideoTags` に `tentative` を足す。`resolveTagNames` が `tentative` を受け、無い名前を §3 の「仮の作成」か「手での作成」で作り、飛ばした名前を集める。飛ばした名前は置き換え後の集合に入らない（Edge Case「`replace` と `tentative`」） |
| `ConfirmTag(ctx, id) (Tag, error)` | §3 の確定。無ければ `ErrTagNotFound` |
| `RejectTag(ctx, id) (name string, error)` | §3 の却下。無ければ `ErrTagNotFound`、仮でなければ `ErrTagNotTentative` |
| `ListRejectedTagNames(ctx) ([]string, error)` | 名前の自然順 |
| `ForgetRejectedTagName(ctx, name) error` | §3 の取り外し。`name` は `NormalizeTagName` で整えてから照合し、整えられない入力はそのまま照合する（`RemoveSynonym` と同じ扱い） |

既存の操作の変更:

- `ListTags`、`tagByID`、`lookupTagName`（`nameLookup` に仮かどうかを足す）、`canonicalNameByTagID` を
  使う `AttachTagByID`・`DetachTag`・`AttachTagByName`、`TagsByContentKeys`（`tags` を結ぶ）、
  `Summary`、外部連携の一覧（`ListTags` を共有）は `tentative` を読んで `TagRef.Tentative`・
  `Tag.Tentative` に載せる。
- `RenameTag`・`AddSynonym`・`mergeTagInto` は §3 の確定を行う。
- `insertTagName` と `RenameTag` の `update` は §3 の `rejected_tag_names` の削除を行う。
  `FolderGroupStore` のグループのタグ化は `findOrCreateTag` → `insertTag` → `insertTagName` を通るので、
  変更なしで手での作成の規則になる。
- `internal/httpapi` が宣言する `Tags` の interface（`router.go`）に上の操作を足し、`ApplyVideoTags` の
  署名を変える。配線は `cmd/mdm`（変更なし。同じ `store.TagStore` が満たす）。

## 6. 変えないもの

- 動画への付け外し（`AttachTagByID`・`DetachTag`・`applyManualTags`）、タグでの絞り込み（014 §6）、
  検索欄でのタグ名の照合（014 §7）、本数（014 §5）、フォルダ由来のタグ（017 §4）。どれも
  `tags.tentative` を読まない（要件 4）。
- ゲストへの応答（`Video.tags` は空の配列のまま。[guest-api.md](../016-single-account-auth/contracts/guest-api.md)）。
- `SearchKeyVersion`。`rejected_tag_names` は照合用の鍵を持たない。
