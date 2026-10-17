import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import { t } from "../i18n";

// スマートフォルダのページの条件の行が、条件から決めるチップ。画面の部品から外し、ロジックのテストで確かめる。

/** RefKind は名前を引いて見せる条件の種類である（欠けた参照の `kind` と同じ）。 */
export type RefKind = SmartRuleRef["kind"];

/**
 * ConditionChip は条件 1 つのチップである。文字の決まったもの（検索、視聴状態、お気に入り、
 * 再生可否、追加日、長さ）は `text`、タグ・種類・人物・団体は名前を引く `ref` で、欠けた
 * 参照なら `missing`。
 */
export type ConditionChip =
  | { kind: "text"; key: string; text: string }
  | { kind: "ref"; key: string; ref: RefKind; id: number; missing: boolean };

/** refChip は名前を引くチップである。`missing` の参照なら欠けた印を付ける。 */
function refChip(
  ref: RefKind,
  id: number,
  missing: readonly SmartRuleRef[],
): ConditionChip {
  return {
    kind: "ref",
    key: `${ref}:${String(id)}`,
    ref,
    id,
    missing: missing.some((each) => each.kind === ref && each.id === id),
  };
}

/** textChip は文字の決まったチップである。 */
function textChip(key: string, text: string): ConditionChip {
  return { kind: "text", key, text };
}

/**
 * conditionChips は条件 `rule` のチップを、条件の編集の行の順（検索、視聴状態、お気に入り、
 * 再生可否、タグの種類、タグ、人物、団体、追加日、長さ）に返す。`missing` はフォルダと祖先の
 * 欠けた参照で、名指されたチップに欠けた印を付ける。空の条件はチップを持たない。
 */
export function conditionChips(
  rule: SmartRule,
  missing: readonly SmartRuleRef[] = [],
): ConditionChip[] {
  const chips: ConditionChip[] = [];
  if (rule.q !== undefined && rule.q !== "") {
    chips.push(textChip("q", t.smartFolders.condition.search(rule.q)));
  }
  if (rule.watch !== undefined) {
    chips.push(textChip("watch", t.list.filter.watchOptions[rule.watch]));
  }
  if (rule.favorite === true) {
    chips.push(textChip("favorite", t.smartFolders.condition.favorite));
  }
  if (rule.playable === true) {
    chips.push(textChip("playable", t.smartFolders.condition.playable));
  }
  if (rule.tagKind !== undefined) chips.push(refChip("tagKind", rule.tagKind, missing));
  for (const id of rule.tags ?? []) chips.push(refChip("tag", id, missing));
  if (rule.person !== undefined) chips.push(refChip("person", rule.person, missing));
  if (rule.organization !== undefined) {
    chips.push(refChip("organization", rule.organization, missing));
  }
  if (rule.addedWithin !== undefined) {
    chips.push(
      textChip("addedWithin", t.smartFolders.condition.addedWithin(rule.addedWithin)),
    );
  }
  if (rule.minDuration !== undefined || rule.maxDuration !== undefined) {
    chips.push(
      textChip(
        "length",
        t.smartFolders.condition.length(rule.minDuration, rule.maxDuration),
      ),
    );
  }
  return chips;
}

/** refText は名前の分かった参照のチップの文字である（「Tag: 猫」）。 */
export function refText(ref: RefKind, name: string): string {
  const words = t.smartFolders.condition;
  switch (ref) {
    case "tag":
      return words.tag(name);
    case "tagKind":
      return words.kind(name);
    case "person":
      return words.person(name);
    case "organization":
      return words.organization(name);
  }
}

/** hasMissing は条件のチップに欠けた参照があるかを返す。 */
export function hasMissing(chips: readonly ConditionChip[]): boolean {
  return chips.some((chip) => chip.kind === "ref" && chip.missing);
}
