import type { Video } from "../../api/client";
import type { ExternalId } from "../../api/credits";
import type {
  WorkDateKind,
  WorkInfo,
  WorkOrganizationRole,
  WorkPersonRole,
} from "../../api/workInfo";
import type {
  WorkInfoFetch,
  WorkInfoField,
  WorkInfoMatch,
} from "../../api/workInfoPlugins";
import { t } from "../../i18n";
import { foldForMatch } from "../../lib/foldForMatch";
import { withoutScheme } from "../WorkInfo";
import type { RowState } from "./fetchRules";

export type DetailField = Exclude<WorkInfoField, "title" | "tags" | "cover">;

export type CreditKind = "person" | "organization";

/** notLinked marks ambiguous matches that cannot be applied; unmarked entries add existing entities or values. */
export type ChipMark = "onVideo" | "new" | "notLinked";

export interface DetailChip {
  key: string;
  /** Use the current entity name when a fetched entry matches an existing entity. */
  name: string;
  mark?: ChipMark;
  /** Original fetched spelling when it differs from the matched entity's current name. */
  fetchedAs?: string;
  full?: string;
}

export interface SingleRow {
  kind: "single";
  state: RowState;
  now?: string;
  fetched?: string;
}

export interface ListRow {
  kind: "list";
  state: RowState;
  now: DetailChip[];
  fetched: DetailChip[];
  /** Count new entities, excluding unresolved matches. */
  fresh: number;
}

export type DetailRow = SingleRow | ListRow;

const DATE_FIELDS = {
  "dates.release": "release",
  "dates.recorded": "recorded",
  "dates.published": "published",
} as const satisfies Partial<Record<DetailField, WorkDateKind>>;

const PERSON_FIELDS = {
  "people.performer": "performer",
  "people.director": "director",
  "people.photographer": "photographer",
  "people.streamer": "streamer",
} as const satisfies Partial<Record<DetailField, WorkPersonRole>>;

const ORGANIZATION_FIELDS = {
  "organizations.maker": "maker",
  "organizations.label": "label",
} as const satisfies Partial<Record<DetailField, WorkOrganizationRole>>;

type DateField = keyof typeof DATE_FIELDS;
type PersonField = keyof typeof PERSON_FIELDS;
type OrganizationField = keyof typeof ORGANIZATION_FIELDS;

function isDateField(field: DetailField): field is DateField {
  return field in DATE_FIELDS;
}

function isPersonField(field: DetailField): field is PersonField {
  return field in PERSON_FIELDS;
}

function isOrganizationField(field: DetailField): field is OrganizationField {
  return field in ORGANIZATION_FIELDS;
}

export function isDetailField(field: WorkInfoField): field is DetailField {
  return field !== "title" && field !== "tags" && field !== "cover";
}

export function creditKind(field: WorkInfoField): CreditKind | undefined {
  if (!isDetailField(field)) return undefined;
  if (isPersonField(field)) return "person";
  return isOrganizationField(field) ? "organization" : undefined;
}

export function chipHint(
  chip: DetailChip,
  kind: CreditKind | undefined,
): string | undefined {
  const text = t.player.workInfoFetch;
  if (chip.mark === "new") return text.newHint;
  if (chip.mark === "notLinked") return text.notLinkedHint[kind ?? "person"];
  if (chip.fetchedAs === undefined) return undefined;
  // Preserve fetched aliases in tooltips even for entities already on the video.
  return chip.mark === "onVideo"
    ? text.onVideoFetchedAs(chip.fetchedAs)
    : text.fetchedAs(chip.fetchedAs);
}

export function isDateRow(field: WorkInfoField): boolean {
  return isDetailField(field) && isDateField(field);
}

const EMPTY_INFO: WorkInfo = {
  dates: [],
  people: [],
  organizations: [],
  urls: [],
  externalIds: [],
};

export function videoWorkInfo(video: Video): WorkInfo {
  return video.workInfo ?? EMPTY_INFO;
}

function singleRow(now: string | undefined, fetched: string | undefined): SingleRow {
  let state: RowState = "differs";
  if (fetched === undefined || fetched === "") state = "missing";
  else if (fetched === now) state = "same";
  return { kind: "single", state, now, fetched };
}

function adds(chip: DetailChip): boolean {
  return chip.mark === undefined || chip.mark === "new";
}

/** Selectable when additions exist; unresolved-only rows are nothing, and fully present rows are same. */
function listState(fetched: readonly DetailChip[]): RowState {
  if (fetched.length === 0) return "missing";
  if (fetched.some(adds)) return "differs";
  return fetched.some((chip) => chip.mark === "notLinked") ? "nothing" : "same";
}

function listRow(
  now: DetailChip[],
  fetched: DetailChip[],
  fresh = fetched.filter(adds).length,
): ListRow {
  return { kind: "list", state: listState(fetched), now, fetched, fresh };
}

function creditChip(
  entry: { name: string; match: WorkInfoMatch },
  linked: ReadonlySet<number>,
  key: string,
): DetailChip {
  const { match } = entry;
  if (match.state === "ambiguous") return { key, name: entry.name, mark: "notLinked" };
  if (match.state === "new") return { key, name: entry.name, mark: "new" };
  if (match.id === undefined) return { key, name: entry.name };
  const name = match.name ?? entry.name;
  return {
    key,
    name,
    mark: linked.has(match.id) ? "onVideo" : undefined,
    fetchedAs: name === entry.name ? undefined : entry.name,
  };
}

type FetchedCredit = { name: string; externalIds: ExternalId[]; match: WorkInfoMatch };

interface NewCreditGroup {
  ids: Set<string>;
  fold: string;
}

/** Match backend createdIndex grouping by shared external ID, or normalized name when no ID applies. */
function joinNewCredit(groups: NewCreditGroup[], entry: FetchedCredit): boolean {
  const ids = entry.externalIds.map(idKey);
  const fold = foldForMatch(entry.name.trim());
  const group =
    groups.find((g) => ids.some((id) => g.ids.has(id))) ??
    groups.find((g) => g.fold === fold);
  if (group === undefined) {
    groups.push({ ids: new Set(ids), fold });
    return true;
  }
  for (const id of ids) group.ids.add(id);
  return false;
}

/** Count distinct entities, matching backend deduplication of existing and newly created credits. */
function freshCredits(
  fetched: readonly FetchedCredit[],
  chips: readonly DetailChip[],
): number {
  const existing = new Set<number>();
  const groups: NewCreditGroup[] = [];
  let fresh = 0;
  fetched.forEach((entry, index) => {
    const chip = chips[index];
    if (chip === undefined || !adds(chip)) return;
    const { match } = entry;
    if (match.state === "new") {
      if (joinNewCredit(groups, entry)) fresh++;
    } else if (match.id === undefined || !existing.has(match.id)) {
      if (match.id !== undefined) existing.add(match.id);
      fresh++;
    }
  });
  return fresh;
}

function creditRow(
  stored: readonly { id: number; name: string }[],
  fetched: readonly FetchedCredit[],
): ListRow {
  const linked = new Set(stored.map((credit) => credit.id));
  const chips = fetched.map((entry, index) => creditChip(entry, linked, String(index)));
  return listRow(
    stored.map((credit) => ({ key: String(credit.id), name: credit.name })),
    chips,
    freshCredits(fetched, chips),
  );
}

function urlChip(url: string, key: string, onVideo: boolean): DetailChip {
  return {
    key,
    name: withoutScheme(url),
    full: url,
    mark: onVideo ? "onVideo" : undefined,
  };
}

function urlsRow(info: WorkInfo, result: WorkInfoFetch): ListRow {
  const stored = new Set(info.urls);
  return listRow(
    info.urls.map((url) => urlChip(url, url, false)),
    result.urls.map((url, index) => urlChip(url, String(index), stored.has(url))),
  );
}

function idKey(id: ExternalId): string {
  return `${id.source}\u0000${id.value}`;
}

function idChip(id: ExternalId, key: string, onVideo: boolean): DetailChip {
  return {
    key,
    name: t.player.workInfo.externalId(id.source, id.value),
    mark: onVideo ? "onVideo" : undefined,
  };
}

function idsRow(info: WorkInfo, result: WorkInfoFetch): ListRow {
  const stored = new Set(info.externalIds.map(idKey));
  return listRow(
    info.externalIds.map((id) => idChip(id, idKey(id), false)),
    result.externalIds.map((id, index) =>
      idChip(id, String(index), stored.has(idKey(id))),
    ),
  );
}

function dateOf(
  dates: readonly { kind: WorkDateKind; date: string }[],
  kind: WorkDateKind,
): string | undefined {
  return dates.find((entry) => entry.kind === kind)?.date;
}

function creditDetailRow(
  field: PersonField | OrganizationField,
  info: WorkInfo,
  result: WorkInfoFetch,
): ListRow {
  if (isPersonField(field)) {
    const role = PERSON_FIELDS[field];
    return creditRow(
      info.people.filter((entry) => entry.role === role).map((entry) => entry.person),
      result.people.filter((entry) => entry.role === role),
    );
  }
  const role = ORGANIZATION_FIELDS[field];
  return creditRow(
    info.organizations
      .filter((entry) => entry.role === role)
      .map((entry) => entry.organization),
    result.organizations.filter((entry) => entry.role === role),
  );
}

export function detailRow(
  field: DetailField,
  info: WorkInfo,
  result: WorkInfoFetch,
): DetailRow {
  if (field === "code") return singleRow(info.code, result.code);
  if (field === "description") return singleRow(info.description, result.description);
  if (field === "urls") return urlsRow(info, result);
  if (field === "externalIds") return idsRow(info, result);
  if (isDateField(field)) {
    const kind = DATE_FIELDS[field];
    return singleRow(dateOf(info.dates, kind), dateOf(result.dates, kind));
  }
  return creditDetailRow(field, info, result);
}
