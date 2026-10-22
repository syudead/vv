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

/** The tab to select after the video response changes: the same one while the new list has it. */
export function nextTab(selected: VideoTab, tabs: readonly VideoTab[]): VideoTab {
  return tabs.includes(selected) ? selected : "overview";
}
