import type { WatchFilter } from "../api/client";
import { t, type UiText } from "../i18n";

export const watchValues: readonly WatchFilter[] = [
  "all",
  "unwatched",
  "inProgress",
  "watched",
];

export function watchLabel(value: WatchFilter): UiText {
  return t.list.filter.watchOptions[value];
}

export function resultCountText(total: number): UiText {
  return t.list.resultCount(total);
}
