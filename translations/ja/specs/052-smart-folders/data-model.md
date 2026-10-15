---
source: specs/052-smart-folders/data-model.md
sourceHash: b19acacfd44b809fe9b187ecf947050c46e7a4b8c48be4322adecf023aafdc18
---

# データモデル: 中身が保存した条件に従うスマートフォルダ {#data-model-smart-folders-whose-contents-follow-a-saved-condition}

親 Issue: #1139。モデルの残りは変わらない: 既存のテーブルは [internal/store/migrations/](../../internal/store/migrations/) に、グループは [048 データモデル](../048-groups/data-model.md) にある。

## マイグレーション {#migration}

マイグレーションは `000NN_smart_folders.sql` の 1 つで、単位が入るときに `main` で次に空いている番号を取る (今は `00047` が最後で、#1136 は `00048` を予定している)。

```sql
-- User data (research.md R-1). No foreign key to videos.
create table smart_folders (
    id         integer primary key autoincrement,
    parent_id  integer references smart_folders (id) on delete cascade,
    name       text    not null check (length(name) between 1 and 100),
    title_key  text    not null,          -- domain.NaturalSortKey(name)
    rule       text    not null,          -- JSON, domain.SmartRule
    sort       text    not null,          -- a VideoSort other than random
    owner_kept integer not null default 1 check (owner_kept in (0, 1)),
    created_at integer not null,          -- Unix ms
    updated_at integer not null,          -- Unix ms
    check (parent_id is null or parent_id <> id)
);
create index smart_folders_parent_idx on smart_folders (parent_id);

create table smart_folder_favorites (
    smart_folder_id integer primary key references smart_folders (id) on delete cascade,
    favorited_at    integer not null
);

-- playlist_items is rebuilt with smart_folder_id integer references smart_folders (id)
-- on delete cascade; exactly one of content_key, group_id and smart_folder_id is set.
```

`smart_folders` と `smart_folder_favorites` はユーザーデータである: ARCHITECTURE.md (「User data survives a rebuild」) と running-vv.md の表にそれらが加わる。down は両方を落とし、`playlist_items` をそのスマートフォルダの行なしで作り直す。

## `smart_folders.rule` {#smart_foldersrule}

JSON のオブジェクト。どのメンバーも省略できる。ないメンバーは何も制約せず、空のオブジェクトはすべての動画に合う。`domain.SmartRule` がそれを解析し、検証する ([R-3](research.md#r-3-a-rule-is-the-library-filters-fields-contents-are-the-filtered-library))。

| メンバー | 型 | 等しいライブラリの絞り込みの項目 |
| --- | --- | --- |
| `q` | 文字列、500 文字まで | 検索の文言、所有者の意味で ([R-11](research.md#r-11-the-rule-reads-the-owners-facts-visibility-is-the-viewers)) |
| `watch` | `unwatched`、`inProgress`、`watched` | 視聴の状態 |
| `playable` | `true` | 再生できるものだけ |
| `favorite` | `true` | お気に入りだけ |
| `tags` | タグの id の配列、1 から 16 個 | すべてのタグが付いている (手動か、フォルダから) |
| `tagKind` | タグの種類の id | その種類のいずれかのタグ (#1136) |
| `person`、`organization` | id | いずれかの役割でクレジットされている |
| `addedWithin` | 整数、1 から 36,500 | その日数以内に追加された ([R-4](research.md#r-4-the-library-filter-gains-added-within-and-length)) |
| `minDuration`、`maxDuration` | 整数の秒、0 から 360,000 | 長さの範囲、両端を含む。`min` ≤ `max` |

`SmartRule.Query()` は、絞り込みの項目が規則のものと等しい `VideoQuery` を返す。`SmartRule.Refs()` は、存在しない参照の確認と統合のために、その参照を `(kind, id)` として挙げる。

## 中身 {#contents}

| 読み取り | 規則 |
| --- | --- |
| 実効の規則 | フォルダの規則と各祖先の規則。ストアはそれぞれの絞り込みの条件を適用する ([R-5](research.md#r-5-a-childs-effective-rule-is-every-rule-on-its-path-anded)) |
| 項目 | 実効の規則での `ListLibrary` を、フォルダの `sort` で、閲覧者に対して返したもの ([R-3](research.md#r-3-a-rule-is-the-library-filters-fields-contents-are-the-filtered-library))。最初のページでは、`title_key` 順の子フォルダが先に来る |
| 存在しない参照 | 経路上の規則が存在しない id を挙げる: 子フォルダのほかに項目はない ([R-9](research.md#r-9-merges-rewrite-rules-a-deleted-reference-makes-the-rule-match-nothing)) |
| 再生の順序 | 順序どおりの項目。グループのカードはその再生の順序 (048) に置き換え、各動画は最初に現れた位置に置く。子フォルダは再生しない |
| カードの値 | 再生の順序に対する `LibraryGroup` の値: 本数、合計、日付、視聴の状態、プレビュー、開く動画 |
| 示すこと | 所有者には常に示す。ゲストには再生の順序が空でないときに示す ([R-8](research.md#r-8-an-empty-folder-stays-and-is-shown-to-the-owner-only)) |

## ライブラリの項目 {#library-items}

ライブラリの項目のクエリーは、クエリーに検索がなく、お気に入りだけ以外の絞り込みもないとき、最上位のフォルダ (`parent_id is null`) を種類 `smartFolder` の項目として持つようになる。お気に入りだけのときは、任意の深さのお気に入りにしたフォルダを持つ ([R-7](research.md#r-7-root-smart-folders-are-cards-in-the-unfiltered-library))。その並びの値は、グループのカードと同じようにカードの値から来る。カーソルは種類を持つ。動画やグループの項目は変わらない。

## 編集 {#edits}

| 操作 | 規則 |
| --- | --- |
| 作成 | 名前 (048 の名前の規則)、規則、並び順 (既定は `addedDesc`)、省略できる親。`owner_kept = 1` |
| 変更 | 名前、規則、並び順、親のいずれか。`updated_at` と `owner_kept = 1` |
| 移動 | フォルダ自身かその子孫である親は `ErrSmartFolderCycle` で、何も書かない。親がなければ最上位になる |
| 削除 | フォルダと、外部キーを通してその子孫、そのお気に入り、そのプレイリストの項目を削除する |
| 存在しない対象 | 存在しない親の id は `ErrSmartFolderNotFound` である |

存在しない id を挙げる規則は受け付け、存在しないものとして報告する。

## 統合 {#merges}

| 変更 | 規則への影響 |
| --- | --- |
| タグの統合 (`mergeTagsInto`) | 各規則の統合元のタグの id が、統合のトランザクションの中で、重複を除いて統合先の id になる |
| 人物か組織の統合 (`CreditStore.Merge`) | 同じように、統合元の id が統合先の id になる |
| タグ、タグの種類、人物、組織の削除 | 規則は書かない。参照は存在しなくなる |

## 通知 {#notices}

コミットの後、下の各書き込みは `SmartFoldersChanged` を 1 つ発行する ([R-12](research.md#r-12-writes-that-can-change-a-match-publish-smartfolderschanged))。

| 書き込み | 挙げるフォルダ |
| --- | --- |
| フォルダの作成、変更、移動、削除、そのお気に入りの設定、または統合による規則の書き換え | そのフォルダとその子孫 (移動では、さらに元の親と新しい親) |
| 動画へのタグの付け外し (手で、一括で、または自動タグで) | すべてのフォルダ |
| タグの名前の変更、削除、統合、同義語の変更、またはタグの種類の設定か解除 (#1136) | すべてのフォルダ |
| 人物か組織のクレジットの追加、解除、名前の変更、削除、統合 | すべてのフォルダ |
| 動画のお気に入りの設定か解除 | すべてのフォルダ |
| `domain.WatchState` が保存済みのものと異なる進捗の保存 | すべてのフォルダ |
| 動画が一覧に載る、ライブラリを離れる、まとめられるか、まとまりから外される、またはグループが変わる (`GroupsChanged`) | すべてのフォルダ |

フォルダが 1 つもないときは何も発行しない。

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| `rule` は有効な `domain.SmartRule` として解析できる | すべての書き込みでの `domain.ParseSmartRule`。`internal/store/invariants_test.go` |
| `sort` は `random` 以外の有効な `VideoSort` である | `domain.ValidateSmartSort` |
| どのフォルダも自身の祖先ではない | 書き込みの中の `domain.ValidateSmartFolderParent`。`internal/store/invariants_test.go` |

## 変わらないもの {#what-does-not-change}

`groups`、`group_items`、`group_exclusions`: スマートフォルダはどの動画もライブラリの最上位から外さず、項目を持たない ([R-1](research.md#r-1-a-smart-folder-is-its-own-entity-not-a-group))。
