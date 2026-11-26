import { type Credit, type CreditKind, otherNames } from "../api/credits";
import { t } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";

export const creditPageLimit = 100;

export function kindTexts(kind: CreditKind) {
  return kind === "person" ? t.credits.person : t.credits.organization;
}

/** The first name after the display name that matches folded, unless the display name already does. */
export function matchedOtherName(credit: Credit, folded: string): string | undefined {
  if (folded === "" || foldForMatch(credit.name).includes(folded)) return undefined;
  return otherNames(credit).find((entry) => foldForMatch(entry.name).includes(folded))
    ?.name;
}

export function sortStorageKey(kind: CreditKind): string {
  return kind === "person" ? "vv.people.v1" : "vv.organizations.v1";
}
