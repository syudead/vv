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
import { creditDisplayName } from "../../credits/displayName";

/**
 * Missing IDs identify new credits; equal newKey values of the same kind create one entity on
 * Save. A provisional person's chip has no name; a naming chip carries the name Save gives it.
 */
export type Chip =
  | { key: string; id: number; name: string; provisional?: boolean; naming?: boolean }
  | { key: string; newKey: string; name: string };

export interface LinkRow {
  key: string;
  url: string;
}

export interface IdRow {
  key: string;
  source: string;
  value: string;
}

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

export type Role =
  | { kind: "person"; role: WorkPersonRole }
  | { kind: "organization"; role: WorkOrganizationRole };

let nextKey = 0;

export function newRowKey(prefix: string): string {
  nextKey += 1;
  return `${prefix}${String(nextKey)}`;
}

export function isNewChip(chip: Chip): chip is Extract<Chip, { newKey: string }> {
  return "newKey" in chip;
}

export function chipLabel(chip: Chip): string {
  return isNewChip(chip) ? chip.name : creditDisplayName(chip);
}

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
      ...(entry.provisional && { provisional: true }),
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

export function chipsOf(draft: Draft, role: Role): Chip[] {
  return role.kind === "person"
    ? draft.people[role.role]
    : draft.organizations[role.role];
}

export function withChips(draft: Draft, role: Role, chips: Chip[]): Draft {
  if (role.kind === "person") {
    return { ...draft, people: { ...draft.people, [role.role]: chips } };
  }
  return { ...draft, organizations: { ...draft.organizations, [role.role]: chips } };
}

export interface NewEntry {
  kind: CreditKind;
  newKey: string;
  name: string;
}

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

export interface Naming {
  id: number;
  name: string;
}

/** Provisional people to name on Save, once each; the first chip's name wins. */
export function namings(draft: Draft): Naming[] {
  const seen = new Set<number>();
  const entries: Naming[] = [];
  for (const role of workPersonRoles) {
    for (const chip of draft.people[role]) {
      if (isNewChip(chip) || chip.naming !== true || seen.has(chip.id)) continue;
      seen.add(chip.id);
      entries.push({ id: chip.id, name: chip.name });
    }
  }
  return entries;
}

/** Mark a named person's chips as ordinary, so a later save does not send the name again. */
export function withNamed(draft: Draft, id: number, name: string): Draft {
  const people = { ...draft.people };
  for (const role of workPersonRoles) {
    people[role] = people[role].map((chip) =>
      !isNewChip(chip) && chip.id === id ? { key: chip.key, id, name } : chip,
    );
  }
  return { ...draft, people };
}

/** Replace newKey values with created IDs immediately so later saves do not recreate them. */
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

function comparable(draft: Draft): string {
  const chip = (c: Chip) => {
    if (isNewChip(c)) return `n${c.newKey}`;
    return c.naming === true ? `p${String(c.id)}=${c.name}` : `p${String(c.id)}`;
  };
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

export function draftChanged(draft: Draft, loaded: Draft): boolean {
  return comparable(draft) !== comparable(loaded);
}

/** Map server field paths to details-${control} inputs for errors and focus. */
export type FieldTargets = ReadonlyMap<string, string>;

/** Omit empty rows and unresolved new credits; map submitted indices back to inputs for field errors. Create new credits before calling. */
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
