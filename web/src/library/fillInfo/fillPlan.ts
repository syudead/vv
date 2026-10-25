import type { Video } from "../../api/client";
import type { WorkInfoFetch, WorkInfoField } from "../../api/workInfoPlugins";
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
import { t } from "../../i18n";

/** One field a fill writes and the values it writes there, for the review. */
export interface FillLine {
  field: WorkInfoField;
  values: string[];
}

/** What filling a video from a fetched work writes: the apply's selection and the lines the review shows. */
export interface FillPlan {
  selection: WorkSelection;
  lines: FillLine[];
}

/**
 * Choose only what the video has no value for: the title while a file video has no display name, tags when it has
 * none, each empty detail field, role, link and ID list, and the first cover when no image is applied
 * (docs/design-docs/link-fetch.md#a-selection-fills-its-empty-fields-after-a-review).
 */
export function fillPlan(video: Video, result: WorkInfoFetch): FillPlan {
  const info = videoWorkInfo(video);
  const lines: FillLine[] = [];
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
    lines.push({ field: "title", values: [result.title ?? ""] });
  }
  for (const row of wholeRows(video, result)) {
    const detail = row.field === "title" ? undefined : detailRow(row.field, info, result);
    if (row.kind !== "fill" || detail?.kind !== "single") continue;
    selection.fields.push(row.field);
    lines.push({ field: row.field, values: [detail.fetched ?? ""] });
  }
  if (video.tags.length === 0) {
    selection.tags = selectableTags(video, result);
    if (selection.tags.length > 0) lines.push({ field: "tags", values: selection.tags });
  }
  for (const field of ROLE_FIELDS) {
    const row = roleRow(field, video, result);
    if (row.now.length > 0) continue;
    const chips = row.fetched.filter((chip) => choosable(chip) && chip.at !== undefined);
    if (chips.length === 0) continue;
    const ats = chips.map((chip) => chip.at ?? 0);
    if (field.startsWith("people.")) selection.people.push(...ats);
    else selection.organizations.push(...ats);
    lines.push({ field, values: chips.map((chip) => chip.name) });
  }
  if (info.urls.length === 0) selection.urls = freshUrls(video, result);
  if (selection.urls.length > 0) lines.push({ field: "urls", values: selection.urls });
  if (info.externalIds.length === 0) selection.externalIds = freshIds(video, result);
  if (selection.externalIds.length > 0) {
    lines.push({
      field: "externalIds",
      values: selection.externalIds.map((id) =>
        t.player.workInfo.externalId(id.source, id.value),
      ),
    });
  }
  const cover = result.covers[0];
  if (video.thumbnailImage === undefined && cover !== undefined) {
    selection.cover = 0;
    lines.push({
      field: "cover",
      values: [t.player.workInfoFetch.coverSize(cover.width, cover.height)],
    });
  }
  return { selection, lines };
}
