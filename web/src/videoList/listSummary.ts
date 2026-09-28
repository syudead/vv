import type { WatchFilter } from "../api/client";
import { t, type UiText } from "../i18n";

/** watchValues は視聴状態の選択肢の並びである。 */
export const watchValues: readonly WatchFilter[] = [
  "all",
  "unwatched",
  "inProgress",
  "watched",
];

/** watchLabel は視聴状態の選択肢の表示名である。 */
export function watchLabel(value: WatchFilter): UiText {
  return t.list.filter.watchOptions[value];
}

/** resultCountText は検索や絞り込み後の全件数を表示する（「1 video」「2 videos」）。 */
export function resultCountText(total: number): UiText {
  return t.list.resultCount(total);
}
