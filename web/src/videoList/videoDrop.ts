import { maxBundleSelection } from "../api/client";
import type { LibraryGroup, Video } from "../api/client";
import { t, type UiText } from "../i18n";
import type { OntoAnswer } from "./cardDragRules";

/** Code-point limit matching domain.ValidateGroupName. */
export const groupNameLimit = 100;

/** Reject lifted groups: group creation accepts videos only. */
export function videoOnto(liftedAllVideos: boolean): OntoAnswer<UiText> {
  return liftedAllVideos ? { label: t.organize.drop.groupOrBundle } : "refused";
}

/** Count lifted cards plus the target against 20; the server expands existing bundles. */
export function bundleOverLimit(liftedCount: number): boolean {
  return liftedCount + 1 > maxBundleSelection;
}

function codePoints(value: string): number {
  return Array.from(value).length;
}

/** Match Go Unicode White_Space explicitly; JavaScript trim differs at U+0085 and U+FEFF. */
const whiteSpace =
  "\t\n\v\f\r \u0085\u00A0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000";
const leadingWhiteSpace = new RegExp(`^[${whiteSpace}]+`, "u");
const trailingWhiteSpace = new RegExp(`[${whiteSpace}]+$`, "u");

/**
 * Match domain.GroupNameFromTitle order: trim leading whitespace, take 100 code points, then trim
 * trailing whitespace.
 */
export function provisionalGroupName(title: string): string {
  const cut = Array.from(title.replace(leadingWhiteSpace, ""))
    .slice(0, groupNameLimit)
    .join("");
  return cut.replace(trailingWhiteSpace, "");
}

export type NameFieldOutcome =
  { kind: "keep" } | { kind: "invalid" } | { kind: "rename"; name: string };

/** Trimmed empty or unchanged names close without saving; overlength names keep the field open. */
export function nameFieldOutcome(value: string, provisional: string): NameFieldOutcome {
  const name = value.trim();
  if (name === "" || name === provisional) return { kind: "keep" };
  if (codePoints(name) > groupNameLimit) return { kind: "invalid" };
  return { kind: "rename", name };
}

export function nameTooLong(value: string): boolean {
  return codePoints(value.trim()) > groupNameLimit;
}

function latest(values: readonly string[], fallback: string): string {
  return values.reduce((max, value) => (value > max ? value : max), fallback);
}

/** Owner-only optimistic group: target video first, then lifted videos in display order. */
export function provisionalGroup(
  id: number,
  target: Video,
  lifted: readonly Video[],
): LibraryGroup {
  const members = [target, ...lifted];
  const durations = members.flatMap((video) =>
    video.durationMs === undefined ? [] : [video.durationMs],
  );
  return {
    id,
    name: provisionalGroupName(target.title),
    videoCount: members.length,
    watchedCount: 0,
    watchState: "unwatched",
    ...(durations.length === 0
      ? {}
      : { durationMs: durations.reduce((sum, value) => sum + value, 0) }),
    sizeBytes: members.reduce((sum, video) => sum + video.sizeBytes, 0),
    addedAt: latest(
      members.map((video) => video.addedAt),
      target.addedAt,
    ),
    previews: members
      .flatMap((video) =>
        video.thumbnailUrl === undefined
          ? []
          : [{ videoId: video.id, thumbnailUrl: video.thumbnailUrl }],
      )
      .slice(0, 4),
    openVideoId: target.id,
    videoIds: members.map((video) => video.id),
    tags: [],
    favorite: false,
  };
}

/** made replaces the target for grouping; otherwise only lifted cards disappear for bundling. */
export interface DropOverlay<T> {
  target: string;
  lifted: ReadonlySet<string>;
  made: T | null;
}

export function applyDropOverlay<T>(
  items: readonly T[],
  keyOf: (item: T) => string,
  overlay: DropOverlay<T> | null,
): readonly T[] {
  if (overlay === null) return items;
  if (overlay.made === null)
    return items.filter((item) => !overlay.lifted.has(keyOf(item)));
  return replaceWithGroup(items, keyOf, overlay.target, overlay.lifted, overlay.made);
}

/** Place the new group at the target's position and remove lifted items. */
export function replaceWithGroup<T>(
  items: readonly T[],
  keyOf: (item: T) => string,
  target: string,
  lifted: ReadonlySet<string>,
  made: T,
): T[] {
  return items.flatMap((item) => {
    const key = keyOf(item);
    if (key === target) return [made];
    return lifted.has(key) ? [] : [item];
  });
}
