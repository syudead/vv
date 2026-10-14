---
source: specs/050-drag-organize/data-model.md
sourceHash: 1cff7c6d981cf95ce0dcc02d4d5ed024b0d2375486bf35b4fc00fdc09d663af4
---

# データモデル: ドラッグ&ドロップで並べ替え、グループにし、統合し、まとめる {#data-model-drag-and-drop-to-reorder-group-merge-and-bundle}

親 Issue: #1075。テーブルも列も加えず、マイグレーションも書かない: 下の操作は [048 データモデル](../048-groups/data-model.md) と [030 データモデル](../030-video-versions/data-model.md) のテーブルに書き込み、取り消しの記録はメモリだけにある ([R-2](research.md#r-2-the-undo-journal-lives-in-internalapp-in-memory-for-10-minutes))。モデルの残りは変わらない ([internal/store/migrations/](../../internal/store/migrations/))。

## 操作 {#operations}

各操作は新しい `OrganizeStore` の役割の 1 つのトランザクションで、コミットの後に `GroupsChanged` (まとめでは `VideoBundleChanged` も) を発行し、そのイメージを返す。ドラッグしたカードは R-4 のとおりに解決する。解決しない動画かグループ、または `from` にないドラッグしたカードは `ErrGroupTargetNotFound` で、何も書かれない ([048 R-12](../048-groups/research.md#r-12-edits-are-positional-and-fail-whole-when-a-target-is-gone))。

| 操作 | 規則 |
| --- | --- |
| 並べ替え | グループ G のドラッグした項目が、画面の順で、G の 1 つの項目の後か先頭へまとまって動く。位置は `0 … n-1` のまま |
| 移動 | ドラッグしたカードを画面の順で `to` に付け加え、すでにあるものは飛ばす。`from` があるときは、それらを `from` から取り除く。`to` がないと、取り除きだけが起きる (ライブラリにドロップした)。フォルダのメンバーを取り除くとその除外を書き、最後の項目を取り除くと、048 の Remove と同じように `from` を解散する |
| グループ作成 | 対象のタイトルを 100 文字に切ったものを名前とし、対象とその後にドラッグした動画を持つグループを作る ([R-6](research.md#r-6-make-a-group-puts-the-new-group-where-the-target-was))。`from` があると、新しいグループは `from` の中の対象の位置を取り、対象とドラッグした項目は `from` を離れる |
| 統合 | ドラッグしたカードを画面の順にたどる: グループは [R-5](research.md#r-5-a-merge-hands-the-dropped-groups-favorites-playlist-items-and-parents-to-the-target) のとおりに統合し、動画は移動と同じように対象に付け加える。そのため、混ざった選択は 1 つのトランザクション、1 つのトークンになる |
| まとめ | 対象を代表とする `VersionStore.Bundle` の規則 ([030 データモデル](../030-video-versions/data-model.md))。すでにまとまっている動画はそのまとまり全体を連れてくる (エッジケース) |
| 取り消し | 今の行が変更後イメージと等しいとき、変更前イメージを書き戻し、それから操作と同じようにフォルダの索引を作り直し、触れたプレイリストの番号を振り直す。そうでなければ `ErrUndoConflict` で、何も書かれない |

`domain.ValidateNesting` は移動、グループ作成、統合に当てはまる: `to` であるか `to` の祖先の 1 つであるドラッグしたグループは `ErrGroupCycle` である (要件 15、エッジケース)。グループをそれ自身に統合するのも同じエラーである。

## 取り消しのイメージ {#undo-image}

イメージは、操作が書き込み、削除し、番号を振り直した、下のテーブルのすべての行をその id とともに、操作のトランザクションの中で変更の前と後に読んだものである。ストアはそれをバイト列に直列化し、それを読み戻すのはストアだけである ([R-1](research.md#r-1-undo-restores-the-rows-the-operation-changed-held-by-the-server))。

| 操作 | イメージの行 |
| --- | --- |
| 並べ替え | G の `group_items` |
| 移動 | `from`、`to`、解散の連鎖が届きうるすべての祖先の `groups`、`group_items` (`group_id` と `child_id` による)、`group_exclusions`、`group_favorites`。それらのどれかを持つすべてのプレイリストの `playlist_items`。それらのフォルダの `folder_group_overrides` |
| グループ作成 | 新しいグループの `groups`。新しいグループと `from` の `group_items` |
| 統合 | ドラッグしたグループ、その親、対象についての移動の行 |
| まとめ | 吸収したすべてのまとまりと新しいまとまりの `video_bundles` と `video_bundle_members`。吸収したすべてのキーと新しいキーの `group_items`、`group_exclusions`、`playlist_items` を、グループかプレイリストの全体で。`copyUserData` が新しいキーの下に書くユーザーデータの行。まとめが取り除く `video_version_candidates` |

どのイメージも、取り消しが削除するか作り直すグループを参照する行も含む: `child_id` による `group_items`、`group_id` による `group_favorites` と `playlist_items`。そのため、操作の後に加わった参照 (新しいグループをお気に入りにした、プレイリストに加えた、別のグループに入れた) は変更後イメージと異なり、取り消しはそれを落とす連鎖削除ではなく `ErrUndoConflict` になる。

削除したグループかまとまりを戻すときは、古い id で挿入し直す。SQLite の `autoincrement` はその id を二度と払い出さないので、その id は空いている。

## 規則 {#rules}

| 規則 | 強制する場所 |
| --- | --- |
| 入れ子と「自分の子孫」へのドロップ | `domain.ValidateNesting` |
| 対象のタイトルからの新しいグループの名前 | 100 文字に切った後の `domain.ValidateGroupName` |
| プレイリストと親での統合の置き換え | `domain.PlanGroupMerge`。`OrganizeStore` が適用する |
| 取り消しトークンは 1 回だけ使われ、10 分生き、同時に最大 100 個 | `app.Organizer` |
| どの操作とどの取り消しの後も、触れた各グループの位置は `0 … n-1` | `internal/store/invariants_test.go` (既存の規則) |

## 変わらないもの {#what-does-not-change}

既存のグループ、バージョン、お気に入りのルートはその振る舞いを保ち、取り消しトークンを返さない: 048 と 030 のメニューとダイアログはそのままで、上の操作を通るのはドロップだけである。
