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

/** How the apply treats a row: fills an empty value, replaces a stored one, or keeps the person's. */
export type RowKind = "fill" | "replace" | "kept";

export type RowItem = Exclude<PersonPluginItem, "image">;

/** One row per item the result carries; the face is chosen apart, since several images compete for it. */
export interface CompareRow {
  item: RowItem;
  kind: RowKind;
}

export function profileItem(field: ProfileField): RowItem {
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
): Alias[] {
  return fetched.aliases.filter((alias) => !aliasOnPerson(current, alias));
}

export function newIds(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
): ExternalId[] {
  return fetched.externalIds.filter((id) => !idOnPerson(current, id));
}

function profileKind(
  current: PersonProfile,
  fetched: PersonProfile,
  field: ProfileField,
  replace: boolean,
): RowKind {
  if (!hasValue(current, field)) return "fill";
  const same = JSON.stringify(current[field]) === JSON.stringify(fetched[field]);
  return replace && !same ? "replace" : "kept";
}

/**
 * One row per item the result carries, in the comparison's order. With `replace`, as on the
 * person page, a different stored profile value can be replaced; lists only ever append.
 */
export function compareRows(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
  replace = false,
): CompareRow[] {
  const rows: CompareRow[] = [];
  const profile = fetched.profile ?? {};
  for (const field of PROFILE_FIELDS) {
    if (hasValue(profile, field))
      rows.push({
        item: profileItem(field),
        kind: profileKind(current.profile, profile, field, replace),
      });
  }
  if (fetched.aliases.length > 0)
    rows.push({
      item: "aliases",
      kind: newAliases(current, fetched).length > 0 ? "fill" : "kept",
    });
  if (fetched.externalIds.length > 0)
    rows.push({
      item: "externalIds",
      kind: newIds(current, fetched).length > 0 ? "fill" : "kept",
    });
  return rows;
}

/** Filling rows start selected; replacing a stored value is the owner's choice per row. */
export function initialItems(rows: readonly CompareRow[]): PersonPluginItem[] {
  return rows.filter((row) => row.kind === "fill").map((row) => row.item);
}

/**
 * The images the face can come from: all of them where the face may be replaced, and none where
 * the apply keeps a stored face.
 */
export function faceChoices(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
  replace = false,
): number {
  const count = fetched.imageCount ?? 0;
  return replace || current.imageRevision === undefined ? count : 0;
}

/** A person without a face starts with the first image; a stored face stays until the owner picks one. */
export function initialImage(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
  replace = false,
): number | null {
  return current.imageRevision === undefined && faceChoices(current, fetched, replace) > 0
    ? 0
    : null;
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
