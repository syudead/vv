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
  WorkInfoFetchedPerson,
  WorkInfoField,
  WorkInfoMatch,
} from "../../api/workInfoPlugins";
import { creditDisplayName } from "../../credits/displayName";
import { personFactGroups } from "../../credits/person/personFacts";
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
  /** Position in the result's people of a face kept with the fetch. */
  faceIndex?: number;
  /** Fetched profile values; applying writes them only into the person's empty fields. */
  profile?: FetchedProfile;
  /** A person already on the video whose missing face or profile fields applying may fill. */
  fills?: boolean;
}

export type FetchedProfile = NonNullable<WorkInfoFetchedPerson["profile"]>;

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

/** Selectable when additions or fills exist; unresolved-only rows are nothing, and fully present rows are same. */
function listState(fetched: readonly DetailChip[]): RowState {
  if (fetched.length === 0) return "missing";
  if (fetched.some((chip) => adds(chip) || chip.fills === true)) return "differs";
  return fetched.some((chip) => chip.mark === "notLinked") ? "nothing" : "same";
}

function listRow(
  now: DetailChip[],
  fetched: DetailChip[],
  fresh = fetched.filter(adds).length,
): ListRow {
  return { kind: "list", state: listState(fetched), now, fetched, fresh };
}

function matchChip(
  entry: { name: string; match: WorkInfoMatch },
  linked: ReadonlySet<number>,
  key: string,
): DetailChip {
  const { match } = entry;
  if (match.state === "ambiguous") return { key, name: entry.name, mark: "notLinked" };
  if (match.state === "new") return { key, name: entry.name, mark: "new" };
  if (match.id === undefined) return { key, name: entry.name };
  // A matched person without a canonical name is provisional; the match carries its name as "".
  const name =
    match.name === undefined
      ? entry.name
      : creditDisplayName({ name: match.name, provisional: match.name === "" });
  return {
    key,
    name,
    mark: linked.has(match.id) ? "onVideo" : undefined,
    fetchedAs: name === entry.name ? undefined : entry.name,
  };
}

type FetchedCredit = {
  name: string;
  externalIds: ExternalId[];
  match: WorkInfoMatch;
  hasImage?: boolean;
  profile?: FetchedProfile;
  /** Position in the result's people; organizations have none. */
  at?: number;
};

/** Applying the role writes a linked person's fetched face and profile only where the person has none. */
function creditChip(
  entry: FetchedCredit,
  linked: ReadonlySet<number>,
  faced: ReadonlySet<number>,
  key: string,
): DetailChip {
  const chip = matchChip(entry, linked, key);
  if (chip.mark === "notLinked") return chip;
  const faceIndex = entry.hasImage === true ? entry.at : undefined;
  const profile =
    entry.profile !== undefined && Object.keys(entry.profile).length > 0
      ? entry.profile
      : undefined;
  const id = entry.match.id;
  const fillsFace = faceIndex !== undefined && id !== undefined && !faced.has(id);
  const fills = chip.mark === "onVideo" && (fillsFace || profile !== undefined);
  return {
    ...chip,
    ...(faceIndex === undefined ? {} : { faceIndex }),
    ...(profile === undefined ? {} : { profile }),
    ...(fills ? { fills } : {}),
  };
}

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
  stored: readonly { id: number; name: string; provisional?: boolean; faced?: boolean }[],
  fetched: readonly FetchedCredit[],
): ListRow {
  const linked = new Set(stored.map((credit) => credit.id));
  const faced = new Set(
    stored.filter((credit) => credit.faced === true).map((credit) => credit.id),
  );
  const chips = fetched.map((entry, index) =>
    creditChip(entry, linked, faced, String(index)),
  );
  return listRow(
    stored.map((credit) => ({ key: String(credit.id), name: creditDisplayName(credit) })),
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
      info.people
        .filter((entry) => entry.role === role)
        .map((entry) => ({ ...entry.person, faced: entry.imageRevision !== undefined })),
      result.people
        .map((entry, at) => ({ ...entry, at }))
        .filter((entry) => entry.role === role),
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

/** A fetched profile written as on the person page, each value after its term so short values read alone. */
export function fillValues(profile: FetchedProfile, today: Date): string[] {
  const text = t.player.workInfoFetch;
  const values = personFactGroups(profile, today).flatMap((group) =>
    group.facts.map((fact) => text.fillFact(fact.term, fact.value)),
  );
  if (profile.bio !== undefined) values.push(text.fillBio);
  if (profile.links !== undefined && profile.links.length > 0)
    values.push(t.credits.personPage.links);
  return values;
}
