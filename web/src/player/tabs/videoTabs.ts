import type { Video } from "../../api/client";
import type { WorkInfo } from "../../api/workInfo";

/** In strip order. */
export const videoTabs = [
  "overview",
  "work",
  "people",
  "versions",
  "thumbnail",
  "file",
] as const;

export type VideoTab = (typeof videoTabs)[number];

/** Whether the work has an item the Work tab lists: anything but the description and the performers. */
export function hasWorkItems(info: WorkInfo | undefined): boolean {
  if (info === undefined) return false;
  return (
    info.code !== undefined ||
    info.dates.length > 0 ||
    info.people.some((entry) => entry.role !== "performer") ||
    info.organizations.length > 0 ||
    info.urls.length > 0 ||
    info.externalIds.length > 0
  );
}

/** The credited people once each, in the work's order, so a person of two roles counts once. */
export function creditedPeopleCount(info: WorkInfo | undefined): number {
  return new Set(info?.people.map((entry) => entry.person.id)).size;
}

/**
 * The tabs a video has, in strip order. `workPlugin` is whether an approved work-info plugin
 * exists; callers pass false until the plugin list has been read.
 */
export function availableTabs(
  video: Video,
  owner: boolean,
  workPlugin: boolean,
): VideoTab[] {
  const shown: Record<VideoTab, boolean> = {
    overview: true,
    work: owner && (hasWorkItems(video.workInfo) || workPlugin),
    people: owner && creditedPeopleCount(video.workInfo) > 0,
    versions: (video.versions?.count ?? 0) >= 2,
    thumbnail: owner,
    file: true,
  };
  return videoTabs.filter((tab) => shown[tab]);
}

const tabQuery = "tab";

/** The tab the address's `tab` names; null without one or for an unknown name. */
export function tabFromSearch(search: string): VideoTab | null {
  const value = new URLSearchParams(search).get(tabQuery);
  return videoTabs.find((tab) => tab === value) ?? null;
}

/** Whether the address has a `tab` at all, known or not. */
export function hasTabParam(search: string): boolean {
  return new URLSearchParams(search).has(tabQuery);
}

/** Whether the address's `tab` already reads as `searchWithTab` would write it for this tab. */
export function searchShowsTab(search: string, tab: VideoTab): boolean {
  return new URLSearchParams(search).get(tabQuery) === (tab === "overview" ? null : tab);
}

/** The search with `tab` naming the selected tab; Overview, the default, leaves it out. */
export function searchWithTab(search: string, tab: VideoTab): string {
  const params = new URLSearchParams(search);
  if (tab === "overview") params.delete(tabQuery);
  else params.set(tabQuery, tab);
  const next = params.toString();
  return next === "" ? "" : `?${next}`;
}

/** The tab to select after the video response changes: the same one while the new list has it. */
export function nextTab(selected: VideoTab, tabs: readonly VideoTab[]): VideoTab {
  return tabs.includes(selected) ? selected : "overview";
}
