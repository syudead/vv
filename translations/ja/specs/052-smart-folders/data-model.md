---
source: specs/052-smart-folders/data-model.md
sourceHash: 58b37d2726f87ac4bee30dc177605bff43fd025271495f982794de9b4eb8910d
---

# データモデル: 動画が保存した条件に従うスマートフォルダ {#data-model-smart-folders-whose-videos-follow-a-saved-condition}

親 Issue: #1139。モデルの残りは変わらない: グループのテーブルは [internal/store/migrations/00043_groups.sql](../../internal/store/migrations/00043_groups.sql) と [048 データモデル](../048-groups/data-model.md) にある。

## マイグレーション {#migration}

マイグレーションは `000NN_smart_folders.sql` の 1 つで、単位が入るときに `main` で次に空いている番号を取る (今は `00047` が最後で、#1136 は `00048` を予定している)。

```sql
alter table groups add column rule text;               -- null for a group; JSON for a smart folder
alter table groups add column sort text;               -- null for a group; a VideoSort other than random
alter table groups add column owner_kept integer not null default 0
    check (owner_kept in (0, 1));
update groups set owner_kept = 1;                      -- every existing group (research.md R-10)
-- A smart folder is never automatic, and has a sort exactly when it has a rule.
-- SQLite cannot add a table check with alter table; the invariants test enforces both.
```

down マイグレーションは、`rule` が null でないすべての行を削除した後で 3 つの列を落とす。`groups` はユーザーデータのままである。ARCHITECTURE.md と running-vv.md に変更は要らない。

## `groups.rule` {#groupsrule}

JSON のオブジェクト。どのメンバーも省略できる。ないメンバーは何も制約しない。空のオブジェクトはすべての動画に合う。`domain.SmartRule` がそれを解析し、検証する ([R-3](research.md#r-3-the-rule-is-the-library-filter-combined-with-and-built-by-the-same-sql))。

| メンバー | 型 | 意味 (同じ意味のライブラリの絞り込みの項目と同じ) |
| --- | --- | --- |
| `q` | 文字列 | 検索の文言、所有者の意味で ([R-11](research.md#r-11-the-rule-reads-the-owners-facts-visibility-is-the-viewers)) |
| `watch` | `unwatched`、`inProgress`、`watched` | 視聴の状態 |
| `playable` | `true` | 再生できるものだけ |
| `favorite` | `true` | お気に入りだけ |
| `tags` | タグの id の配列、1 から 16 個 | すべてのタグが付いている (手動か、フォルダから) |
| `tagKind` | タグの種類の id | その種類のいずれかのタグ (#1136) |
| `person` | 人物の id | いずれかの役割でクレジットされている |
| `organization` | 組織の id | いずれかの役割でクレジットされている |
| `addedWithin` | 整数、1 から 36,500 | その日数以内に追加された ([R-4](research.md#r-4-the-library-filter-gains-added-within-and-length)) |
| `minDuration`、`maxDuration` | 整数の秒、0 から 360,000 | 長さの範囲、両端を含む。`min` ≤ `max` |

`domain.SmartRule.Query()` は、絞り込みの項目が規則のものと等しい `VideoQuery` を返す。`domain.SmartRule.Refs()` は、存在しない参照の確認と統合のために、その参照を `(kind, id)` として挙げる。

## メンバー {#members}

閲覧者にとってのスマートフォルダのメンバー ([R-5](research.md#r-5-hand-added-videos-are-items-hand-removed-videos-are-exclusions)):

| 出どころ | 規則 |
| --- | --- |
| 条件に合う動画 | 所有者の情報で `rule` を満たす示される動画 (まとまりの代表) のうち、そのユーザーキーにフォルダの `group_exclusions` の行がないもの。`rule` に存在しない参照があるときはない ([R-9](research.md#r-9-merges-rewrite-rules-a-deleted-reference-makes-the-rule-match-nothing)) |
| 動画の項目 | `content_key` を持つ `group_items`。示されるかどうかは 048 の規則による |
| 子グループ | `child_id` を持つ `group_items`。`position` 順 |
| 閲覧者による切り詰め | 閲覧者が見てはいけない動画はすべて除く (048 の見え方) |

`groupDescendantsCTE` は、条件に合う動画をそのフォルダの直接のメンバーとして持つようになるので、その上に作ったどの読み取り (ライブラリの項目、カードの値、グループのページ、`?group`、プレイリスト、`LibraryIDs`) もそれらを見る。再生の順序 ([R-6](research.md#r-6-child-groups-keep-their-positions-videos-follow-the-folders-sort)): 子グループの項目の `sort_key` は今と同じくその位置である。動画のメンバー (条件に合う動画か項目) はすべて、すべての子グループの後に、フォルダの `sort` で並び、同順位は動画の id で決める。条件に合い、かつ項目でもある動画は 1 回だけ現れる。

グループのページは、項目の行があり、条件に合わない動画のメンバーに `manual` の印を付けるので、所有者は手で加えたものが分かる。除外した動画は独自のルートで読む ([contracts/screen-api.md](contracts/screen-api.md))。

## 編集 {#edits}

| 操作 | スマートフォルダでの規則 |
| --- | --- |
| 作成 | `name`、`rule`、`sort` (既定は `addedDesc`)。`owner_kept = 1`。要求の項目は動画の項目か子グループになる |
| 規則か並び順の変更 | `domain.SmartRule` で検証する。`updated_at` と `owner_kept = 1` |
| 動画の追加 | その除外を削除する。条件に合わないときは項目を挿入する |
| 動画の取り除き | その項目があれば削除し、その除外を書く |
| 最後のメンバーの取り除き、またはそれを空にする整理での移動 | フォルダは残る ([R-8](research.md#r-8-an-empty-smart-folder-stays-and-is-shown-to-the-owner-only)) |
| 動画の移動か整理での並べ替え | `409 smart_folder_sorted` |
| スマートフォルダを `from` とする整理でのグループ作成 | `409 smart_folder_sorted`: 新しいグループには対象の動画の位置が要る |
| スマートフォルダを別のグループに統合する整理 | `409 smart_folder_merge`: 規則が失われる。グループをスマートフォルダに統合すると、その項目を手で加える |
| 解散、タグへの変換、お気に入り、プレイリスト、入れ子 | グループと同じ (048) |

上の画面の API のすべての書き込みと、048 と 050 のすべてのグループの書き込みは、変えたグループに `owner_kept = 1` を設定する。`syncFolderGroups` と `rekeyGroupItems` は設定しない ([R-10](research.md#r-10-owner_kept-marks-every-group-the-owner-made-or-edited))。

## 統合と削除 {#merges-and-deletions}

| 変更 | 規則への影響 |
| --- | --- |
| タグの統合 (`mergeTagsInto`) | 各規則の統合元のタグの id が、統合のトランザクションの中で、重複を除いて統合先の id になる |
| 人物か組織の統合 (`CreditStore.Merge`) | 同じように、統合元の id が統合先の id になる |
| タグ、タグの種類、人物、組織の削除 | 規則は書かない。参照は存在しなくなり、所有者が変えるまで規則は何にも合わない |

書き換えは、変えたフォルダとその祖先に `GroupsChanged` を発行する。

## 通知 {#notices}

スマートフォルダが少なくとも 1 つあるとき、下の各書き込みは、コミットの後に、すべてのスマートフォルダとその祖先を挙げる `GroupsChanged` を 1 つ発行する ([R-13](research.md#r-13-writes-that-can-change-a-match-publish-groupschanged-for-every-smart-folder))。ストアは、フォルダの索引の作り直しのために `groupsChanged` を集めるのと同じように、書き込みのトランザクションの中で id を集める。

| 書き込み | ストアの経路 |
| --- | --- |
| 動画へのタグの付け外し (手で、または一括で) | `AttachTagByID`、`AttachTagByName`、`DetachTag`、`BatchTags`、自動タグの適用 |
| タグの名前の変更、削除、統合、または同義語の変更 | `TagStore` |
| タグの種類の設定か解除、または種類の削除 (#1136) | `TagStore`、タグの種類のルート |
| 人物か組織のクレジットの追加、解除、名前の変更、削除、統合 | `CreditStore`、`OverrideStore.ApplyFetchedWorkInfo` |
| お気に入りの設定か解除 | `SetFavorites` |
| `domain.WatchState` が保存済みのものと異なる進捗の保存 | `SaveProgress` |
| 動画が一覧に載る、ライブラリを離れる、またはまとめられるか、まとまりから外される | 取り込みの結果、`RemoveContent`、まとめることと継承の経路 |

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| `rule` と `sort` は両方 null か両方設定されている | `internal/store/invariants_test.go` |
| `rule` を持つグループの `origin_folder` は null である | `internal/store/invariants_test.go` |
| `rule` は有効な `domain.SmartRule` として解析できる | すべての書き込みでの `domain.ParseSmartRule`。`internal/store/invariants_test.go` |
| `sort` は `random` 以外の有効な `VideoSort` である | `domain.ValidateSmartSort` |
