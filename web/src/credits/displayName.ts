import { t } from "../i18n";

/** The name to show for a person or organization; a provisional person has none of its own. */
export function creditDisplayName(credit: {
  name: string;
  provisional?: boolean;
}): string {
  return credit.provisional === true ? t.credits.unnamedPerson : credit.name;
}

/**
 * Up to two letters that stand in for a missing face: the first character of each of the first
 * two words, so "中村 楓" gives "中楓" and "Aoi Sato" gives "AS". A provisional person has none.
 */
export function creditInitials(credit: { name: string; provisional?: boolean }): string {
  if (credit.provisional === true) return "";
  const words = credit.name
    .trim()
    .split(/\s+/u)
    .filter((word) => word !== "");
  return words
    .slice(0, 2)
    .map((word) => (Array.from(word)[0] ?? "").toUpperCase())
    .join("");
}
