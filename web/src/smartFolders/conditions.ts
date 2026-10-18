import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import { t } from "../i18n";

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
 * Preserve editor field order, omit empty conditions and mark missing references from both folder
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
