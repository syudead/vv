import type { Video } from "../../api/client";
import type { ExternalId } from "../../api/credits";
import type { WorkPerson } from "../../api/workInfo";
import type {
  WorkInfoApplyChoice,
  WorkInfoFetch,
  WorkInfoField,
} from "../../api/workInfoPlugins";
import {
  adds,
  type DetailChip,
  type DetailField,
  detailRow,
  type ListRow,
  videoWorkInfo,
} from "./detailRows";
import { FIELDS, selectableTags, titleRow } from "./fetchRules";

/**
 * What a work-info apply takes: whole fields, the chosen tags, the cover by position, and the
 * people, organizations, URLs and external IDs one by one.
 */
export interface WorkSelection {
  /** Title, code, description and dates; tags and the cover are chosen apart. */
  fields: WorkInfoField[];
  tags: string[];
  cover: number | null;
  people: number[];
  organizations: number[];
  urls: string[];
  externalIds: ExternalId[];
  /** Unnamed performers on the video matched with fetched performers, merged into them on apply. */
  samePeople: SamePair[];
}

/** An unnamed performer on the video and the fetched performer at at whom the owner says they are. */
export interface SamePair {
  at: number;
  personId: number;
}

/** How the apply treats a whole field: fills an empty value, replaces a stored one, or leaves it as it is. */
export type FieldKind = "fill" | "replace" | "same";

export type WholeField = Exclude<
  WorkInfoField,
  | "tags"
  | "cover"
  | "urls"
  | "externalIds"
  | `people.${string}`
  | `organizations.${string}`
>;

export type RoleField = Extract<
  WorkInfoField,
  `people.${string}` | `organizations.${string}`
>;

export const WHOLE_FIELDS = FIELDS.filter(
  (field): field is WholeField =>
    field === "title" ||
    field === "code" ||
    field === "description" ||
    field.startsWith("dates."),
);

export const ROLE_FIELDS = FIELDS.filter(
  (field): field is RoleField =>
    field.startsWith("people.") || field.startsWith("organizations."),
);

export interface WholeRow {
  field: WholeField;
  kind: FieldKind;
}

/** One row per whole field the result carries; the title is never empty, so it only replaces. */
export function wholeRows(video: Video, result: WorkInfoFetch): WholeRow[] {
  const info = videoWorkInfo(video);
  return WHOLE_FIELDS.flatMap((field): WholeRow[] => {
    if (field === "title") {
      const state = titleRow(video, result);
      return state === "missing"
        ? []
        : [{ field, kind: state === "same" ? "same" : "replace" }];
    }
    const row = detailRow(field, info, result);
    if (row.kind !== "single" || row.state === "missing") return [];
    if (row.state === "same") return [{ field, kind: "same" }];
    return [
      { field, kind: row.now === undefined || row.now === "" ? "fill" : "replace" },
    ];
  });
}

/** A fetched person or organization the owner may choose: it adds a link, or fills a linked person. */
export function choosable(chip: DetailChip): boolean {
  return adds(chip) || chip.fills === true;
}

export function roleRow(field: RoleField, video: Video, result: WorkInfoFetch): ListRow {
  return detailRow(field as DetailField, videoWorkInfo(video), result) as ListRow;
}

function choosableAt(video: Video, result: WorkInfoFetch, prefix: string): number[] {
  return ROLE_FIELDS.filter((field) => field.startsWith(prefix)).flatMap((field) =>
    roleRow(field, video, result)
      .fetched.filter(choosable)
      .flatMap((chip) => (chip.at === undefined ? [] : [chip.at])),
  );
}

function idKey(id: ExternalId): string {
  return `${id.source}\u0000${id.value}`;
}

export function freshUrls(video: Video, result: WorkInfoFetch): string[] {
  const stored = new Set(videoWorkInfo(video).urls);
  return [...new Set(result.urls)].filter((url) => !stored.has(url));
}

export function freshIds(video: Video, result: WorkInfoFetch): ExternalId[] {
  const stored = new Set(videoWorkInfo(video).externalIds.map(idKey));
  return result.externalIds.filter((id) => !stored.has(idKey(id)));
}

/**
 * Every change starts selected, as the work information of a video usually comes from its file
 * name or an earlier fetch; the first cover is chosen when there is one.
 */
export function initialWorkSelection(video: Video, result: WorkInfoFetch): WorkSelection {
  return {
    fields: wholeRows(video, result)
      .filter((row) => row.kind !== "same")
      .map((row) => row.field),
    tags: selectableTags(video, result),
    cover: result.covers.length > 0 ? 0 : null,
    people: choosableAt(video, result, "people."),
    organizations: choosableAt(video, result, "organizations."),
    urls: freshUrls(video, result),
    externalIds: freshIds(video, result),
    samePeople: [],
  };
}

/** The video's unnamed performers, each once: the people a fetched performer may turn out to be. */
export function unnamedPerformers(video: Video): WorkPerson[] {
  const seen = new Set<number>();
  return videoWorkInfo(video).people.filter((person) => {
    if (person.role !== "performer" || !person.provisional || seen.has(person.person.id))
      return false;
    seen.add(person.person.id);
    return true;
  });
}

/** The fetched performers the apply adds, which an unnamed performer may be matched with. */
export function samePersonChoices(video: Video, result: WorkInfoFetch): DetailChip[] {
  return roleRow("people.performer", video, result).fetched.filter(
    (chip) => adds(chip) && chip.at !== undefined,
  );
}

/** Matches the unnamed person with the entry at at, or keeps them apart when at is null. */
export function withSamePerson(
  selection: WorkSelection,
  personId: number,
  at: number | null,
): WorkSelection {
  const rest = selection.samePeople.filter(
    (pair) => pair.personId !== personId && pair.at !== at,
  );
  return { ...selection, samePeople: at === null ? rest : [...rest, { at, personId }] };
}

export function workSelectionEmpty(selection: WorkSelection): boolean {
  return (
    selection.fields.length === 0 &&
    selection.tags.length === 0 &&
    selection.cover === null &&
    selection.people.length === 0 &&
    selection.organizations.length === 0 &&
    selection.urls.length === 0 &&
    selection.externalIds.length === 0
  );
}

/** The apply body: tags and the cover join the whole fields when chosen, and empty lists are left out. */
export function workChoice(selection: WorkSelection): WorkInfoApplyChoice {
  const fields: WorkInfoField[] = [...selection.fields];
  const choice: WorkInfoApplyChoice = { fields };
  if (selection.people.length > 0) choice.people = [...selection.people];
  if (selection.organizations.length > 0)
    choice.organizations = [...selection.organizations];
  if (selection.urls.length > 0) choice.urls = [...selection.urls];
  if (selection.externalIds.length > 0) choice.externalIds = [...selection.externalIds];
  // A pair whose fetched performer was cleared has nobody to merge into.
  const same = selection.samePeople.filter((pair) => selection.people.includes(pair.at));
  if (same.length > 0) choice.samePeople = same;
  if (selection.tags.length > 0) {
    fields.push("tags");
    choice.tags = [...selection.tags];
  }
  if (selection.cover !== null) {
    fields.push("cover");
    choice.coverIndex = selection.cover;
  }
  return choice;
}
