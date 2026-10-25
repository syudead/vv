import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import { t } from "../i18n";
import { videoConditionChips } from "../library/videoConditions";

export type RefKind = SmartRuleRef["kind"];

/** Literal conditions use text; named references use ref, with missing marking deleted targets. */
export type ConditionChip =
  | { kind: "text"; key: string; text: string }
  | { kind: "ref"; key: string; ref: RefKind; id: number; missing: boolean };

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

function textChip(key: string, text: string): ConditionChip {
  return { kind: "text", key, text };
}

/**
 * Preserve editor field order with the video conditions last, omit empty conditions and mark missing references from both folder
 * and ancestor settings.
 */
export function conditionChips(
  rule: SmartRule,
  missing: readonly SmartRuleRef[] = [],
): ConditionChip[] {
  const chips: ConditionChip[] = [];
  if (rule.q !== undefined && rule.q !== "") {
    chips.push(textChip("q", t.smartFolders.condition.search(rule.q)));
  }
  if (rule.favorite === true) {
    chips.push(textChip("favorite", t.smartFolders.condition.favorite));
  }
  if (rule.tagKind !== undefined) chips.push(refChip("tagKind", rule.tagKind, missing));
  for (const id of rule.tags ?? []) chips.push(refChip("tag", id, missing));
  if (rule.person !== undefined) chips.push(refChip("person", rule.person, missing));
  if (rule.organization !== undefined) {
    chips.push(refChip("organization", rule.organization, missing));
  }
  for (const chip of videoConditionChips(rule))
    chips.push(textChip(chip.kind, chip.text));
  return chips;
}

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

export function hasMissing(chips: readonly ConditionChip[]): boolean {
  return chips.some((chip) => chip.kind === "ref" && chip.missing);
}
