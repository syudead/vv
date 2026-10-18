import type { VideoFolder } from "../api/client";
import type { GroupRef } from "../api/groups";
import type { Crumb } from "../folders/folderPath";
import { t } from "../i18n";

export const TRAIL_PARAM = "trail";

/** Match api/openapi.yaml's maxItems. */
const maxTrailLength = 100;

export function parseGroupId(raw: string | undefined): number | null {
  if (raw === undefined || !/^[1-9]\d*$/.test(raw)) return null;
  const id = Number(raw);
  return Number.isSafeInteger(id) ? id : null;
}

/** Validate only trail syntax; the server validates current parent-child relationships. */
export function parseTrail(search: string): number[] {
  const raw = new URLSearchParams(search).get(TRAIL_PARAM);
  if (raw === null || raw === "") return [];
  return raw
    .split(",")
    .map(parseGroupId)
    .filter((id): id is number => id !== null)
    .slice(0, maxTrailLength);
}

export function groupPagePath(id: number, trail: readonly number[] = []): string {
  const base = `/groups/${String(id)}`;
  if (trail.length === 0) return base;
  return `${base}?${TRAIL_PARAM}=${trail.slice(-maxTrailLength).map(String).join(",")}`;
}

export function childGroupPath(
  childId: number,
  trail: readonly GroupRef[],
  id: number,
): string {
  return groupPagePath(childId, [...trail.map((group) => group.id), id]);
}

export function groupItemPath(
  item: { kind: "video" | "group"; video?: { id: number }; group?: { id: number } },
  trail: readonly GroupRef[],
  id: number,
): string {
  if (item.kind === "group" && item.group !== undefined) {
    return childGroupPath(item.group.id, trail, id);
  }
  return videoInGroupPath(item.video?.id ?? 0, id);
}

const GROUP_PARAM = "group";

/** Absent or invalid group context is omitted from read requests. */
export function groupContextOf(search: string): number | undefined {
  const raw = new URLSearchParams(search).get(GROUP_PARAM);
  return parseGroupId(raw ?? undefined) ?? undefined;
}

/** Pass group context so Up next follows this group's order. */
export function videoInGroupPath(videoId: number, groupId: number): string {
  return `/videos/${String(videoId)}?${GROUP_PARAM}=${String(groupId)}`;
}

/** Remove only group, preserving other query parameters; return empty when none remain. */
export function searchWithoutGroup(search: string): string {
  const params = new URLSearchParams(search);
  params.delete(GROUP_PARAM);
  return params.size === 0 ? "" : `?${params.toString()}`;
}

/** After dissolution or tagging, return to the final ancestor or the library. */
export function parentPagePath(trail: readonly GroupRef[]): string {
  const parent = trail[trail.length - 1];
  if (parent === undefined) return "/";
  return groupPagePath(
    parent.id,
    trail.slice(0, -1).map((each) => each.id),
  );
}

export function trailCrumbs(trail: readonly GroupRef[], name: string): Crumb[] {
  return [
    { label: t.library.title, to: "/" },
    ...trail.map((group, index) => ({
      label: group.name,
      to: groupPagePath(
        group.id,
        trail.slice(0, index).map((each) => each.id),
      ),
    })),
    { label: name },
  ];
}

/** Render unresolved URL trail segments as skeleton crumbs. */
export function loadingCrumbs(trail: readonly number[]): (Crumb | undefined)[] {
  return [{ label: t.library.title, to: "/" }, ...trail.map(() => undefined), undefined];
}

/** Until the root name is known, use only the relative path, or empty for the root itself. */
export function folderDisplayPath(folder: VideoFolder, rootName?: string): string {
  if (rootName === undefined) return folder.path;
  return folder.path === "" ? rootName : `${rootName}/${folder.path}`;
}
