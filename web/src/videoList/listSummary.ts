import type { WatchFilter } from "../api/client";

/** watchOptions は視聴状態の選択肢と表示名である。 */
export const watchOptions: { value: WatchFilter; label: string }[] = [
  { value: "all", label: "すべて" },
  { value: "unwatched", label: "未視聴" },
  { value: "inProgress", label: "視聴途中" },
  { value: "watched", label: "視聴済み" },
];

/** resultCountText は検索や絞り込み後の全件数を表示する。 */
export function resultCountText(total: number): string {
  return `${total.toLocaleString("ja-JP")}件`;
}
