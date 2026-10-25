import type { Video } from "../../api/client";
import type { WorkInfoFetch } from "../../api/workInfoPlugins";
import { detailRow, videoWorkInfo } from "../../player/workInfoFetch/detailRows";
import { selectableTags, titleRow } from "../../player/workInfoFetch/fetchRules";
import {
  choosable,
  freshIds,
  freshUrls,
  ROLE_FIELDS,
  roleRow,
  type WorkSelection,
  wholeRows,
} from "../../player/workInfoFetch/workSelection";

/**
 * What filling a video from a fetched work writes, and the fetched code and release date the review shows so the
 * owner can tell at a glance whether the right work was found.
 */
export interface FillPlan {
  selection: WorkSelection;
  facts: string[];
}

/**
 * Choose only what the video has no value for: the title while a file video has no display name, tags when it has
 * none, each empty detail field, role, link and ID list, and the first cover when no image is applied
 * (docs/design-docs/link-fetch.md#a-selection-fills-its-empty-fields-after-a-review).
 */
export function fillPlan(video: Video, result: WorkInfoFetch): FillPlan {
  const info = videoWorkInfo(video);
  const selection: WorkSelection = {
    fields: [],
    tags: [],
    cover: null,
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    samePeople: [],
  };
  if (!video.virtual && !video.displayName && titleRow(video, result) === "differs") {
    selection.fields.push("title");
  }
  for (const row of wholeRows(video, result)) {
    const detail = row.field === "title" ? undefined : detailRow(row.field, info, result);
    if (row.kind !== "fill" || detail?.kind !== "single") continue;
    selection.fields.push(row.field);
  }
  if (video.tags.length === 0) selection.tags = selectableTags(video, result);
  for (const field of ROLE_FIELDS) {
    const row = roleRow(field, video, result);
    if (row.now.length > 0) continue;
    const ats = row.fetched
      .filter((chip) => choosable(chip) && chip.at !== undefined)
      .map((chip) => chip.at ?? 0);
    if (field.startsWith("people.")) selection.people.push(...ats);
    else selection.organizations.push(...ats);
  }
  if (info.urls.length === 0) selection.urls = freshUrls(video, result);
  if (info.externalIds.length === 0) selection.externalIds = freshIds(video, result);
  if (video.thumbnailImage === undefined && result.covers.length > 0) selection.cover = 0;
  const release = result.dates.find((date) => date.kind === "release")?.date;
  const facts = [result.code, release].filter(
    (fact): fact is string => fact !== undefined && fact !== "",
  );
  return { selection, facts };
}
