import type { LibraryItem } from "../api/client";
import type { GroupRef } from "../api/groups";
import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import { fromSelectAll, type LibrarySelection } from "../library/selection";
import { conditionChips, hasMissing } from "./conditions";

// スマートフォルダのページが中身と条件から決める規則。画面の部品から外し、ロジックのテストで確かめる。

/**
 * BodyKind は読めたページの本体の形である。`content` は中身を並べる。`empty` と `missing` は
 * 動画もグループも無いフォルダで、子フォルダのカードの後に空の印を出す。`missing` は条件の
 * どれかが消えた実体を名指している（それで何にも一致しない）。
 */
export type BodyKind = "content" | "empty" | "missing";

/**
 * bodyKind は本体の形を決める。動画かグループが 1 つでも並ぶか、続きがあれば `content`。
 * そうでなく欠けた参照があれば `missing`、無ければ `empty`。欠けた参照は所有者の応答にだけ
 * 入る（ゲストには空のフォルダは届かない）。
 */
export function bodyKind(
  items: readonly LibraryItem[],
  nextCursor: string | undefined,
  missing: readonly SmartRuleRef[],
): BodyKind {
  const filled = items.some((item) => item.kind !== "smartFolder");
  if (filled || nextCursor !== undefined) return "content";
  return missing.length > 0 ? "missing" : "empty";
}

/** AncestorRule は祖先 1 つと、読めていればその条件である。 */
export interface AncestorRule {
  folder: GroupRef;
  rule?: SmartRule;
}

/**
 * missingParent は欠けた参照を自分の条件に持たないとき、それを持ついちばん近い祖先である
 * （欠けた状態の説明に「The condition is in "{parent}".」を足す）。自分の条件にあるか、祖先の
 * 条件がまだ読めていなければ undefined。`ancestors` は近い順。
 */
export function missingParent(
  own: SmartRule | undefined,
  ancestors: readonly AncestorRule[],
  missing: readonly SmartRuleRef[],
): GroupRef | undefined {
  if (own === undefined || hasMissing(conditionChips(own, missing))) return undefined;
  return ancestors.find(
    (ancestor) =>
      ancestor.rule !== undefined && hasMissing(conditionChips(ancestor.rule, missing)),
  )?.folder;
}

/**
 * selectAllLoaded はスマートフォルダのページの「Select all」の選択である。ここまでに並んだ
 * 動画と、グループ（全メンバー、グループとしても覚える）を選ぶ。子フォルダのカードは
 * 選ばない（中の動画はそれぞれのカードで選ぶ）。
 */
export function selectAllLoaded(items: readonly LibraryItem[]): LibrarySelection {
  const ids: number[] = [];
  const groups = [];
  for (const item of items) {
    if (item.kind === "video") ids.push(item.video.id);
    if (item.kind === "group") {
      ids.push(...item.group.videoIds);
      groups.push({ id: item.group.id, videoIds: item.group.videoIds });
    }
  }
  return fromSelectAll(ids, groups);
}
