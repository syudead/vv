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
import { withoutScheme } from "../WorkInfo";
import type { RowState } from "./fetchRules";

// 比較の段の作品の詳細の行（specs/052-plugin-work-info-fields/ui-design.md「Compare」
// 「States」）。今の値はページの workInfo から、取得した値は取得の結果と照合の答えから作る。

/** DetailField は題名とタグ以外の、作品の詳細の行の項目である。 */
export type DetailField = Exclude<WorkInfoField, "title" | "tags">;

/** CreditKind は人物・団体の行の実体の種類である。 */
export type CreditKind = "person" | "organization";

/**
 * ChipMark は取得した側のチップの印である。`onVideo` は動画にすでにある件、`new` は反映で
 * 作られる件、`notLinked` は複数に当たり反映されない件で、無ければ足される既存の件か値である。
 */
export type ChipMark = "onVideo" | "new" | "notLinked";

/** DetailChip は一覧の行に並べる 1 件である。 */
export interface DetailChip {
  key: string;
  /** チップに出す名前。既存の実体に当たった件はその今の名前。 */
  name: string;
  mark?: ChipMark;
  /** 既存の実体に当たった件で、取得した綴りが今の名前と違うときの取得した名前。 */
  fetchedAs?: string;
  /** 省略せずに見せる値（URL の全体）。 */
  full?: string;
}

/** SingleRow は品番・説明・日付の行である。 */
export interface SingleRow {
  kind: "single";
  state: RowState;
  now?: string;
  fetched?: string;
}

/** ListRow は人物・団体・リンク・ID の行である。 */
export interface ListRow {
  kind: "list";
  state: RowState;
  now: DetailChip[];
  fetched: DetailChip[];
  /** 動画にまだ無い件の数（`{n} new`）。`notLinked` は数えない。 */
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

/** creditKind は人物・団体の行の実体の種類で、それ以外の行なら undefined である。 */
export function creditKind(field: WorkInfoField): CreditKind | undefined {
  if (field === "title" || field === "tags") return undefined;
  if (isPersonField(field)) return "person";
  return isOrganizationField(field) ? "organization" : undefined;
}

/** isDateRow は日付の行かを返す。 */
export function isDateRow(field: WorkInfoField): boolean {
  return field !== "title" && field !== "tags" && isDateField(field);
}

const EMPTY_INFO: WorkInfo = {
  dates: [],
  people: [],
  organizations: [],
  urls: [],
  externalIds: [],
};

/** videoWorkInfo はページの動画の作品情報で、無ければ空である。 */
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

/**
 * listState は一覧の行の状態である。足せる件があれば選べ、無くて結び付かない件があれば
 * `nothing`（Nothing to add）、すべて動画にあれば `same` である。
 */
function listState(fetched: readonly DetailChip[]): RowState {
  if (fetched.length === 0) return "missing";
  if (fetched.some(adds)) return "differs";
  return fetched.some((chip) => chip.mark === "notLinked") ? "nothing" : "same";
}

function listRow(now: DetailChip[], fetched: DetailChip[]): ListRow {
  return {
    kind: "list",
    state: listState(fetched),
    now,
    fetched,
    fresh: fetched.filter(adds).length,
  };
}

/**
 * creditChip は取得した人物・団体の 1 件を照合の答えでチップにする。既存の実体に当たれば
 * その今の名前で、その役割にすでに結び付いていれば `onVideo` である。
 */
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

function creditRow(
  stored: readonly { id: number; name: string }[],
  fetched: readonly { name: string; match: WorkInfoMatch }[],
): ListRow {
  const linked = new Set(stored.map((credit) => credit.id));
  return listRow(
    stored.map((credit) => ({ key: String(credit.id), name: credit.name })),
    fetched.map((entry, index) => creditChip(entry, linked, String(index))),
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

/** detailRow は作品の詳細の行 field を、今の作品情報 info と取得の結果 result から作る。 */
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
