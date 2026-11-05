import type { CreditName, CreditPatch, Person } from "../../../api/credits";
import type { components } from "../../../api/gen/openapi";
import type { IdRow, LinkRow } from "../../../player/detailsEditor/draft";
import { newRowKey } from "../../../player/detailsEditor/draft";
import { otherNames } from "../../../api/credits";
import {
  type AliasRow,
  aliasRowsOf,
  idRowsOf,
  idsToSend,
  namesToSend,
  type Sent,
} from "../../CreditDetailsDialog";

type ProfilePatch = components["schemas"]["PersonProfilePatch"];

export const textFields = [
  "gender",
  "bloodType",
  "birthDate",
  "deathDate",
  "birthplace",
  "cup",
  "bio",
] as const;

export const numberFields = [
  "heightCm",
  "bustCm",
  "waistCm",
  "hipsCm",
  "careerStartYear",
  "careerEndYear",
] as const;

export type TextField = (typeof textFields)[number];
export type NumberField = (typeof numberFields)[number];
export type ProfileField = TextField | NumberField;

/** Every input as typed; an empty string is a field left unset. */
export interface PersonDraft {
  name: string;
  /** The display name's language, which the page keeps without showing it. */
  nameLanguage: string;
  aliases: AliasRow[];
  externalIds: IdRow[];
  links: LinkRow[];
  profile: Record<ProfileField, string>;
}

export function draftFromPerson(person: Person): PersonDraft {
  const profile = {} as Record<ProfileField, string>;
  for (const field of textFields) profile[field] = person.profile[field] ?? "";
  for (const field of numberFields) {
    const value = person.profile[field];
    profile[field] = value === undefined ? "" : String(value);
  }
  return {
    name: person.name,
    nameLanguage: person.names[0]?.language ?? "",
    aliases: aliasRowsOf(otherNames(person)),
    externalIds: idRowsOf(person.externalIds),
    links: (person.profile.links ?? []).map((url) => ({ key: newRowKey("u"), url })),
    profile,
  };
}

// Fifteen digits stay a safe integer and are far more than any accepted value has, so an overlong
// paste still meets the server's range check instead of becoming Infinity, which JSON sends as null.
const maxDigits = 15;

/** Number inputs keep digits only, so a sent value is always a whole number or empty. */
export function digitsOnly(value: string): string {
  return value.replace(/\D/g, "").slice(0, maxDigits);
}

function sentLinks(rows: readonly LinkRow[]): { values: string[]; keys: string[] } {
  const kept = rows.filter((row) => row.url.trim() !== "");
  return { values: kept.map((row) => row.url.trim()), keys: kept.map((row) => row.key) };
}

/** The names list: the name field first, then the rows; a blank name with no rows is the provisional person's []. */
function personNamesToSend(draft: PersonDraft): { values: CreditName[]; keys: string[] } {
  const name = draft.name.trim();
  if (name === "" && noOtherNames(draft.aliases)) return { values: [], keys: [] };
  const language = draft.nameLanguage.trim();
  return namesToSend(language === "" ? { name } : { name, language }, draft.aliases);
}

function noOtherNames(rows: readonly AliasRow[]): boolean {
  return namesToSend(undefined, rows).values.length === 0;
}

function sameJson(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

function profilePatch(draft: PersonDraft, loaded: PersonDraft): ProfilePatch {
  const patch: ProfilePatch = {};
  for (const field of textFields) {
    const value = draft.profile[field].trim();
    if (value !== loaded.profile[field].trim())
      patch[field] = value === "" ? null : value;
  }
  for (const field of numberFields) {
    const value = draft.profile[field];
    if (value !== loaded.profile[field])
      patch[field] = value === "" ? null : Number(value);
  }
  const links = sentLinks(draft.links).values;
  if (!sameJson(links, sentLinks(loaded.links).values)) patch.links = links;
  return patch;
}

/** Server paths that name an input, and the index-to-row keys the dialog's mapping reads. */
export interface SentTargets extends Sent {
  linkKeys: string[];
}

/**
 * The PATCH body with only the properties that differ from the loaded person, so a concurrent
 * change to another property survives; edited lists replace their whole value.
 */
export function buildPersonPatch(
  draft: PersonDraft,
  loaded: PersonDraft,
): { patch: CreditPatch; sent: SentTargets } {
  const patch: CreditPatch = {};
  const names = personNamesToSend(draft);
  if (!sameJson(names.values, personNamesToSend(loaded).values))
    patch.names = names.values;
  const ids = idsToSend(draft.externalIds);
  if (!sameJson(ids.values, idsToSend(loaded.externalIds).values))
    patch.externalIds = ids.values;
  const profile = profilePatch(draft, loaded);
  if (Object.keys(profile).length > 0) patch.profile = profile;
  return {
    patch,
    sent: {
      nameKeys: names.keys,
      idKeys: ids.keys,
      linkKeys: sentLinks(draft.links).keys,
    },
  };
}

export function draftChanged(draft: PersonDraft, loaded: PersonDraft): boolean {
  return Object.keys(buildPersonPatch(draft, loaded).patch).length > 0;
}

/** The control a profile or name path names; the other names and IDs are the dialog's mapping. */
export function profileControl(field: string, sent: SentTargets): string | undefined {
  if (field === "name" || field === "names[0].name") return "name";
  if (field === "profile.links") return "links";
  const link = /^profile\.links\[(\d+)\]$/.exec(field);
  if (link !== null) {
    const key = sent.linkKeys[Number(link[1])];
    return key === undefined ? undefined : `url-${key}`;
  }
  const name = /^profile\.(\w+)$/.exec(field)?.[1];
  const known: readonly string[] = [...textFields, ...numberFields];
  return name !== undefined && known.includes(name) ? `profile-${name}` : undefined;
}
