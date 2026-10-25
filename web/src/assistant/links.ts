import { videoConditionNames } from "../api/client";
import type { AssistantCriteria, AssistantDestination } from "../api/assistant";
import type { SmartRule } from "../api/smartFolders";
import { personPath } from "../credits/person/personPath";
import { groupPagePath } from "../groups/groupPaths";
import {
  ORGANIZATION_PARAM,
  PERSON_PARAM,
  TAG_KIND_PARAM,
} from "../library/creditCriteria";
import { LIBRARY_PATH, libraryFilterPath } from "../library/libraryPath";
import { serializeTagIds, TAG_PARAM } from "../library/tagCriteria";
import { playlistPath } from "../playlists/playlistItems";
import { settingsPagePath } from "../settings/settingsPages";
import { smartFolderPagePath } from "../smartFolders/smartFolderPaths";
import { ruleKey } from "../smartFolders/smartRule";
import { normalizeQuery } from "../videoList/listCriteria";
import { libraryRule } from "./screenContext";

/** The library URL with the outcome's conditions, in the parameter names the library reads. */
export function criteriaPath({ rule, sort }: AssistantCriteria): string {
  const params = new URLSearchParams();
  const query = normalizeQuery(rule.q ?? "");
  if (query !== "") params.set("q", query);
  if (rule.favorite === true) params.set("fav", "1");
  if (sort !== undefined && sort !== "random") params.set("sort", sort);
  for (const id of serializeTagIds(rule.tags ?? [])) params.append(TAG_PARAM, id);
  if (rule.tagKind !== undefined) params.set(TAG_KIND_PARAM, String(rule.tagKind));
  if (rule.person !== undefined) params.set(PERSON_PARAM, String(rule.person));
  if (rule.organization !== undefined) {
    params.set(ORGANIZATION_PARAM, String(rule.organization));
  }
  for (const name of videoConditionNames) {
    const value = rule[name];
    if (value !== undefined) params.set(name, String(value));
  }
  const search = params.toString();
  return search === "" ? LIBRARY_PATH : `${LIBRARY_PATH}?${search}`;
}

/** Whether the library at this location shows the outcome's conditions; the sort may differ. */
export function showsCriteria(
  pathname: string,
  search: string,
  criteria: AssistantCriteria,
): boolean {
  if (pathname !== LIBRARY_PATH) return false;
  return sortedKey(libraryRule(search) ?? {}) === sortedKey(criteria.rule);
}

/** The library keeps tag ids sorted in its URL, so the order of tags means nothing here. */
function sortedKey(rule: SmartRule): string {
  return ruleKey({ ...rule, tags: [...(rule.tags ?? [])].sort((a, b) => a - b) });
}

const listPaths: Partial<Record<AssistantDestination["kind"], string>> = {
  library: LIBRARY_PATH,
  folders: "/folders",
  tags: "/tags",
  people: "/people",
  organizations: "/organizations",
  playlists: "/playlists",
  history: "/history",
  duplicates: "/duplicates",
  activity: "/activity",
};

function entityPath(kind: AssistantDestination["kind"], id: number): string | undefined {
  switch (kind) {
    case "video":
      return `/videos/${String(id)}`;
    case "person":
      return personPath(id);
    case "organization":
      return libraryFilterPath(ORGANIZATION_PARAM, id);
    case "tag":
      return libraryFilterPath(TAG_PARAM, id);
    case "playlist":
      return playlistPath(id);
    case "group":
      return groupPagePath(id);
    case "smartFolder":
      return smartFolderPagePath(id);
    default:
      return undefined;
  }
}

/** The route a navigate outcome opens. */
export function destinationPath(destination: AssistantDestination): string {
  if (destination.kind === "settings") {
    return destination.settingsPage === undefined
      ? "/settings"
      : settingsPagePath(destination.settingsPage);
  }
  if (destination.id !== undefined) {
    const path = entityPath(destination.kind, destination.id);
    if (path !== undefined) return path;
  }
  return listPaths[destination.kind] ?? LIBRARY_PATH;
}

/** Whether the location is still the page a navigate outcome opened. */
export function atDestination(pathname: string, search: string, path: string): boolean {
  const [destinationPathname, destinationSearch = ""] = path.split("?");
  if (pathname !== destinationPathname) return false;
  if (destinationSearch === "") return true;
  // Library pages add their sort to the URL; compare only the parameters the destination sets.
  const current = new URLSearchParams(search);
  return [...new URLSearchParams(destinationSearch)].every(([name, value]) =>
    current.getAll(name).includes(value),
  );
}
