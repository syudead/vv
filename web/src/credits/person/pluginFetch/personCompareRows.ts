import type { CreditName, ExternalId } from "../../../api/credits";
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
  names: readonly CreditName[];
  externalIds: readonly ExternalId[];
  profile: PersonProfile;
  imageRevision?: number;
}

export type ProfileField = keyof PersonProfile;

/** The comparison's order: one row per field, so one row is one apply item; links are a list. */
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
];

/** How the apply treats a row: fills an empty value, replaces a stored one, or keeps the person's. */
export type RowKind = "fill" | "replace" | "kept";

export type RowItem = Exclude<
  PersonPluginItem,
  "image" | "names" | "externalIds" | "profile.links"
>;

/** One row per profile field the result carries; the face and the lists are chosen apart. */
export interface CompareRow {
  item: RowItem;
  kind: RowKind;
}

/**
 * What an apply takes: profile fields, the image chosen as the face, and the names, external IDs
 * and links it appends one by one.
 */
export interface PersonSelection {
  items: RowItem[];
  image: number | null;
  names: string[];
  externalIds: ExternalId[];
  links: string[];
}

export function profileItem(field: Exclude<ProfileField, "links">): RowItem {
  return `profile.${field}` as RowItem;
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

export function idKey(id: ExternalId): string {
  return `${id.source}\u0000${id.value}`;
}

/** A list's fetched values, split into those the apply would add and those the person has. */
export interface ListChoices<T> {
  fresh: T[];
  onPerson: T[];
}

function split<T>(values: readonly T[], has: (value: T) => boolean): ListChoices<T> {
  return {
    fresh: values.filter((value) => !has(value)),
    onPerson: values.filter(has),
  };
}

/** The fetched names by spelling, the fetched display name first, as the apply appends them. */
export function nameList(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
): ListChoices<string> {
  const stored = new Set(current.names.map((entry) => entry.name));
  return split(
    fetched.names.map((entry) => entry.name),
    (name) => stored.has(name),
  );
}

export function idList(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
): ListChoices<ExternalId> {
  const stored = new Set(current.externalIds.map(idKey));
  return split(fetched.externalIds, (id) => stored.has(idKey(id)));
}

export function linkList(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
): ListChoices<string> {
  const stored = new Set(current.profile.links ?? []);
  return split(fetched.profile?.links ?? [], (link) => stored.has(link));
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
 * One row per profile field the result carries, in the comparison's order. With `replace`, as on
 * the person page, a different stored value can be replaced.
 */
export function compareRows(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
  replace = false,
): CompareRow[] {
  const profile = fetched.profile ?? {};
  return PROFILE_FIELDS.filter((field) => hasValue(profile, field)).map((field) => ({
    item: profileItem(field as Exclude<ProfileField, "links">),
    kind: profileKind(current.profile, profile, field, replace),
  }));
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

/**
 * Filling rows, every value the lists would add, and the first image for a person without a face
 * start selected; replacing a stored value is the owner's choice.
 */
export function initialSelection(
  current: PersonCompareCurrent,
  fetched: PersonPluginFetchedPerson,
  replace = false,
): PersonSelection {
  return {
    items: compareRows(current, fetched, replace)
      .filter((row) => row.kind === "fill")
      .map((row) => row.item),
    image:
      current.imageRevision === undefined && faceChoices(current, fetched, replace) > 0
        ? 0
        : null,
    names: nameList(current, fetched).fresh,
    externalIds: idList(current, fetched).fresh,
    links: linkList(current, fetched).fresh,
  };
}

export function selectionEmpty(selection: PersonSelection): boolean {
  return (
    selection.items.length === 0 &&
    selection.image === null &&
    selection.names.length === 0 &&
    selection.externalIds.length === 0 &&
    selection.links.length === 0
  );
}

/** The apply body's selection: the image joins the items when one is chosen. */
export function selectionBody(selection: PersonSelection): {
  items: PersonPluginItem[];
  names: string[];
  externalIds: ExternalId[];
  links: string[];
} {
  return {
    items:
      selection.image === null ? [...selection.items] : [...selection.items, "image"],
    names: [...selection.names],
    externalIds: [...selection.externalIds],
    links: [...selection.links],
  };
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
  if (value.reason === "too_many") {
    if (value.item === "names") return text.tooManyNames;
    if (value.item === "profile.links") return text.tooManyLinks;
    return text.tooManyIds;
  }
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
