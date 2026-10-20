import type { Alias, ExternalId } from "../../../api/credits";
import type {
  PersonPluginFetchedPerson,
  PersonPluginItem,
  SkippedPersonValue,
} from "../../../api/personPluginFetches";
import { t, type UiText } from "../../../i18n";
import type { SkippedLine } from "../../../player/workInfoFetch/fetchRules";
import { type PersonProfile, personFactGroups } from "../personFacts";

/** The person a result is compared with; a lookup of a person the apply creates has no name or values. */
export interface PersonCompareCurrent {
  name?: string;
  aliases: readonly Alias[];
  externalIds: readonly ExternalId[];
  profile: PersonProfile;
  imageRevision?: number;
}

export type ProfileField = keyof PersonProfile;

/** The comparison's order: one row per field, so one row is one apply item. */
const PROFILE_FIELDS: readonly ProfileField[] = [
  "birthDate",
  "deathDate",
  "gender",
  "birthplace",
  "heightCm",
  "bustCm",
  "waistCm",
  "hipsCm",
  "cup",
  "bloodType",
  "careerStartYear",
  "careerEndYear",
  "bio",
  "links",
];

export interface CompareRow {
  item: PersonPluginItem;
  /** The apply would write it; otherwise the person already has it and the row is kept. */
  changes: boolean;
  /** The apply would write it over a stored value, so it starts unselected. */
  replaces?: boolean;
}

export function profileItem(field: ProfileField): PersonPluginItem {
  return `profile.${field}`;
}

export function profileFieldOf(item: PersonPluginItem): ProfileField | undefined {
  return item.startsWith("profile.")
    ? (item.slice("profile.".length) as ProfileField)
    : undefined;
}

function hasValue(profile: PersonProfile, field: ProfileField): boolean {
  const value = profile[field];
  return Array.isArray(value) ? value.length > 0 : value !== undefined;
}

export function aliasOnPerson(current: PersonCompareCurrent, alias: Alias): boolean {
  return (
    alias.name === current.name ||
    current.aliases.some((item) => item.name === alias.name)
  );
}

export function idOnPerson(current: PersonCompareCurrent, id: ExternalId): boolean {
  return current.externalIds.some(
    (item) => item.source === id.source && item.value === id.value,
  );
}

export function newAliases(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
): number {
  return fetched.aliases.filter((alias) => !aliasOnPerson(current, alias)).length;
}

export function newIds(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
): number {
  return fetched.externalIds.filter((id) => !idOnPerson(current, id)).length;
}

function sameValue(a: PersonProfile, b: PersonProfile, field: ProfileField): boolean {
  return JSON.stringify(a[field]) === JSON.stringify(b[field]);
}

function storedRow(item: PersonPluginItem, replace: boolean, same: boolean): CompareRow {
  return replace && !same
    ? { item, changes: true, replaces: true }
    : { item, changes: false };
}

/**
 * One row per item the result carries, in the comparison's order. With `replace`, as on the
 * person page, a stored face or a different stored profile value can be replaced.
 */
export function compareRows(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
  replace = false,
): CompareRow[] {
  const rows: CompareRow[] = [];
  if (fetched.hasImage === true)
    rows.push(
      current.imageRevision === undefined
        ? { item: "image", changes: true }
        : storedRow("image", replace, false),
    );
  const profile = fetched.profile ?? {};
  for (const field of PROFILE_FIELDS) {
    if (!hasValue(profile, field)) continue;
    const item = profileItem(field);
    rows.push(
      hasValue(current.profile, field)
        ? storedRow(item, replace, sameValue(current.profile, profile, field))
        : { item, changes: true },
    );
  }
  if (fetched.aliases.length > 0)
    rows.push({ item: "aliases", changes: newAliases(current, fetched) > 0 });
  if (fetched.externalIds.length > 0)
    rows.push({ item: "externalIds", changes: newIds(current, fetched) > 0 });
  return rows;
}

export function initialItems(rows: readonly CompareRow[]): PersonPluginItem[] {
  return rows
    .filter((row) => row.changes && row.replaces !== true)
    .map((row) => row.item);
}

const FACT_IDS: Partial<Record<ProfileField, string>> = {
  birthDate: "born",
  deathDate: "died",
  gender: "gender",
  birthplace: "birthplace",
  heightCm: "height",
  cup: "cup",
  bloodType: "bloodType",
};

/**
 * One field written as the person page writes it; a date takes its age from the other date of
 * `profile`, or of `context` when `profile` lacks it.
 */
export function profileValueText(
  field: Exclude<ProfileField, "bio" | "links">,
  profile: PersonProfile,
  context: PersonProfile,
  today: Date,
): string {
  const value = profile[field];
  const factId = FACT_IDS[field];
  if (factId !== undefined) {
    const dated: PersonProfile = {
      birthDate: profile.birthDate ?? context.birthDate,
      deathDate: profile.deathDate ?? context.deathDate,
      [field]: value,
    };
    const fact = personFactGroups(dated, today)
      .flatMap((group) => group.facts)
      .find((item) => item.id === factId);
    if (fact !== undefined) return fact.value;
  }
  if (
    typeof value === "number" &&
    field !== "careerStartYear" &&
    field !== "careerEndYear"
  )
    return t.credits.personPage.centimetres(value);
  return String(value);
}

export function skippedPersonReason(
  value: Pick<SkippedPersonValue, "item" | "reason" | "holderName">,
): UiText {
  const text = t.credits.personPage.pluginFetch.skipped;
  if (value.reason === "taken") return text.taken(value.holderName ?? "");
  if (value.reason === "too_many")
    return value.item === "aliases" ? text.tooManyAliases : text.tooManyIds;
  return text.invalid;
}

/** The result step's lines, in the response's order. */
export function skippedPersonLines(values: readonly SkippedPersonValue[]): SkippedLine[] {
  const items = t.credits.personPage.pluginFetch.items;
  return values.map((value, index) => ({
    key: `${value.item}\u0000${String(index)}`,
    label: items[value.item],
    value: value.value,
    reason: skippedPersonReason(value),
  }));
}
