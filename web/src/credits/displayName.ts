import { t } from "../i18n";

/** The name to show for a person or organization; a provisional person has none of its own. */
export function creditDisplayName(credit: {
  id: number;
  name: string;
  provisional?: boolean;
}): string {
  return credit.provisional === true ? t.credits.personNumber(credit.id) : credit.name;
}
