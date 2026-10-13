import type { Video } from "../../api/client";
import type { CreditKind } from "../../api/credits";
import {
  type VideoDetailsInput,
  type WorkDateKind,
  type WorkOrganizationRole,
  type WorkPersonRole,
  workDateKinds,
  workOrganizationRoles,
  workPersonRoles,
} from "../../api/workInfo";

// 編集画面の下書き（specs/047-video-work-info/ui-design.md「Editor」の Fields・Save・Leaving）。
// 画面の部品に依らない値の操作（動画から作る、変わったかを比べる、保存の本文にする、
// 誤りの経路を入力へ写す）をここに集める。

/**
 * Chip は役割の 1 件である。`id` があれば保存済みの人物・団体、無ければ `Save` で作る
 * 新しい名前で、同じ種類で同じ `newKey` のチップは 1 件として 1 度だけ作る。
 */
export type Chip =
  | { key: string; id: number; name: string }
  | { key: string; newKey: string; name: string };

/** Row は Links の 1 行か IDs の 1 行の値である。 */
export interface LinkRow {
  key: string;
  url: string;
}

export interface IdRow {
  key: string;
  source: string;
  value: string;
}

/** Draft は編集画面の入力のすべてである。 */
export interface Draft {
  name: string;
  code: string;
  description: string;
  dates: Record<WorkDateKind, string>;
  people: Record<WorkPersonRole, Chip[]>;
  organizations: Record<WorkOrganizationRole, Chip[]>;
  urls: LinkRow[];
  externalIds: IdRow[];
}

/** Role は人物か団体の役割である。 */
export type Role =
  | { kind: "person"; role: WorkPersonRole }
  | { kind: "organization"; role: WorkOrganizationRole };

let nextKey = 0;

/** newRowKey は行とチップの、画面の中だけで使う鍵を作る。 */
export function newRowKey(prefix: string): string {
  nextKey += 1;
  return `${prefix}${String(nextKey)}`;
}

/** isNewChip は、チップがまだ作っていない名前かを返す。 */
export function isNewChip(chip: Chip): chip is Extract<Chip, { newKey: string }> {
  return "newKey" in chip;
}

/** draftFromVideo は動画の今の表示名と作品情報から下書きを作る。 */
export function draftFromVideo(video: Video): Draft {
  const info = video.workInfo;
  const dates = { release: "", recorded: "", published: "" };
  for (const date of info?.dates ?? []) dates[date.kind] = date.date;
  const people: Draft["people"] = {
    performer: [],
    director: [],
    photographer: [],
    streamer: [],
  };
  for (const entry of info?.people ?? []) {
    people[entry.role].push({
      key: newRowKey("c"),
      id: entry.person.id,
      name: entry.person.name,
    });
  }
  const organizations: Draft["organizations"] = { maker: [], label: [] };
  for (const entry of info?.organizations ?? []) {
    organizations[entry.role].push({
      key: newRowKey("c"),
      id: entry.organization.id,
      name: entry.organization.name,
    });
  }
  return {
    name: video.displayName ?? "",
    code: info?.code ?? "",
    description: info?.description ?? "",
    dates,
    people,
    organizations,
    urls: (info?.urls ?? []).map((url) => ({ key: newRowKey("u"), url })),
    externalIds: (info?.externalIds ?? []).map((id) => ({
      key: newRowKey("i"),
      source: id.source,
      value: id.value,
    })),
  };
}

/** chipsOf は下書きの、その役割のチップを返す。 */
export function chipsOf(draft: Draft, role: Role): Chip[] {
  return role.kind === "person"
    ? draft.people[role.role]
    : draft.organizations[role.role];
}

/** withChips は下書きの、その役割のチップを置き換えたものを返す。 */
export function withChips(draft: Draft, role: Role, chips: Chip[]): Draft {
  if (role.kind === "person") {
    return { ...draft, people: { ...draft.people, [role.role]: chips } };
  }
  return { ...draft, organizations: { ...draft.organizations, [role.role]: chips } };
}

/** NewEntry は `Save` で作る新しい名前 1 件である。 */
export interface NewEntry {
  kind: CreditKind;
  newKey: string;
  name: string;
}

/** newEntries は下書きの新しい名前を、人物の役割の順、団体の役割の順に 1 件ずつ返す。 */
export function newEntries(draft: Draft): NewEntry[] {
  const seen = new Set<string>();
  const entries: NewEntry[] = [];
  const add = (kind: CreditKind, chips: readonly Chip[]) => {
    for (const chip of chips) {
      if (!isNewChip(chip)) continue;
      const id = `${kind}:${chip.newKey}`;
      if (seen.has(id)) continue;
      seen.add(id);
      entries.push({ kind, newKey: chip.newKey, name: chip.name });
    }
  };
  for (const role of workPersonRoles) add("person", draft.people[role]);
  for (const role of workOrganizationRoles)
    add("organization", draft.organizations[role]);
  return entries;
}

/**
 * withCreated は、作った新しい名前（`newKey` から id）を下書きのチップへ写す。写したチップは
 * `New` の印を失い、次の `Save` で作り直さない。
 */
export function withCreated(
  draft: Draft,
  kind: CreditKind,
  newKey: string,
  id: number,
): Draft {
  const replace = (chips: Chip[]) =>
    chips.map((chip) =>
      isNewChip(chip) && chip.newKey === newKey
        ? { key: chip.key, id, name: chip.name }
        : chip,
    );
  if (kind === "person") {
    const people = { ...draft.people };
    for (const role of workPersonRoles) people[role] = replace(people[role]);
    return { ...draft, people };
  }
  const organizations = { ...draft.organizations };
  for (const role of workOrganizationRoles)
    organizations[role] = replace(organizations[role]);
  return { ...draft, organizations };
}

/** comparable は、変わったかを比べるための下書きの形（前後の空白と空の行を除く）である。 */
function comparable(draft: Draft): string {
  const chip = (c: Chip) => (isNewChip(c) ? `n${c.newKey}` : `p${String(c.id)}`);
  return JSON.stringify({
    name: draft.name.trim(),
    code: draft.code.trim(),
    description: draft.description.trim(),
    dates: workDateKinds.map((kind) => draft.dates[kind].trim()),
    people: workPersonRoles.map((role) => draft.people[role].map(chip)),
    organizations: workOrganizationRoles.map((role) =>
      draft.organizations[role].map(chip),
    ),
    urls: draft.urls.map((row) => row.url.trim()).filter((url) => url !== ""),
    externalIds: draft.externalIds
      .map((row) => [row.source.trim(), row.value.trim()])
      .filter(([source, value]) => source !== "" || value !== ""),
  });
}

/** draftChanged は、下書きが読み込んだ値から変わったかを返す（ui-design.md「Leaving」）。 */
export function draftChanged(draft: Draft, loaded: Draft): boolean {
  return comparable(draft) !== comparable(loaded);
}

/**
 * Target は、誤りの `field` が指す入力である。`control` はその入力の要素の id の末尾で、
 * 画面は `details-${control}` の要素に理由を出してフォーカスを移す。
 */
export type FieldTargets = ReadonlyMap<string, string>;

/**
 * toDetailsInput は下書きを `PUT /api/videos/{id}/details` の本文にし、誤りの `field`
 * （送った一覧の位置で数えた経路）から入力への写しを添える。空の行と空の値は送らない。
 * まだ作っていない名前のチップは送らない（呼び手が先に作り、withCreated で id にしておく）。
 */
export function toDetailsInput(draft: Draft): {
  body: VideoDetailsInput;
  targets: FieldTargets;
} {
  const targets = new Map<string, string>([
    ["displayName", "name"],
    ["code", "code"],
    ["description", "description"],
  ]);
  const dates = workDateKinds.flatMap((kind) => {
    const date = draft.dates[kind].trim();
    if (date === "") return [];
    return [{ kind, date }];
  });
  dates.forEach((date, index) => {
    targets.set(`dates[${String(index)}].date`, `date-${date.kind}`);
  });
  const people = workPersonRoles.flatMap((role) =>
    draft.people[role].flatMap((chip) =>
      isNewChip(chip) ? [] : [{ role, chip, id: chip.id }],
    ),
  );
  people.forEach(({ chip }, index) => {
    targets.set(`people[${String(index)}].person.id`, `chip-${chip.key}`);
  });
  const organizations = workOrganizationRoles.flatMap((role) =>
    draft.organizations[role].flatMap((chip) =>
      isNewChip(chip) ? [] : [{ role, chip, id: chip.id }],
    ),
  );
  organizations.forEach(({ chip }, index) => {
    targets.set(`organizations[${String(index)}].organization.id`, `chip-${chip.key}`);
  });
  const urls = draft.urls.filter((row) => row.url.trim() !== "");
  urls.forEach((row, index) => targets.set(`urls[${String(index)}]`, `url-${row.key}`));
  const externalIds = draft.externalIds.filter(
    (row) => row.source.trim() !== "" || row.value.trim() !== "",
  );
  externalIds.forEach((row, index) => {
    const at = `externalIds[${String(index)}]`;
    targets.set(at, `id-source-${row.key}`);
    targets.set(`${at}.source`, `id-source-${row.key}`);
    targets.set(`${at}.value`, `id-value-${row.key}`);
  });
  const code = draft.code.trim();
  const description = draft.description.trim();
  return {
    body: {
      displayName: draft.name.trim(),
      workInfo: {
        code: code === "" ? null : code,
        description: description === "" ? null : description,
        dates,
        people: people.map(({ role, id }) => ({ role, person: { id } })),
        organizations: organizations.map(({ role, id }) => ({
          role,
          organization: { id },
        })),
        urls: urls.map((row) => row.url.trim()),
        externalIds: externalIds.map((row) => ({
          source: row.source.trim(),
          value: row.value.trim(),
        })),
      },
    },
    targets,
  };
}
