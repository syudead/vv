import type { Video, WatchFilter } from "../api/client";
import { formatBytes, formatDuration } from "../lib/format";
import type { ListCriteria } from "./listCriteria";

/** watchOptions は視聴状態の選択肢と表示名である（絞り込みと一致なしのチップで使う）。 */
export const watchOptions: { value: WatchFilter; label: string }[] = [
  { value: "all", label: "すべて" },
  { value: "unwatched", label: "未視聴" },
  { value: "inProgress", label: "視聴途中" },
  { value: "watched", label: "視聴済み" },
];

/**
 * conditionLabels は一致なしの状態に並べる、効いている条件の名前である。
 * フォルダ画面はこれに範囲のチップを自分で足す（ui-design.md「No-match state」）。
 */
export function conditionLabels(criteria: ListCriteria): string[] {
  const labels: string[] = [];
  if (criteria.query !== "") labels.push(`検索語「${criteria.query}」`);
  if (criteria.watch !== "all") {
    const watch = watchOptions.find((option) => option.value === criteria.watch);
    if (watch !== undefined) labels.push(watch.label);
  }
  if (criteria.playable) labels.push("再生できるものだけ");
  return labels;
}

/**
 * summarize は Stash の「1-8 of 8 (34m 11s - 263 MB)」に当たる一行。件数はサーバーの
 * total（検索語と絞り込みをすべて適用した全件）、合計時間と大きさは読み込んだ分である。
 * ライブラリ・フォルダ画面の検索結果・最上位の検索結果が同じ書式を使う。
 */
export function summarize(shown: Video[], total: number): string {
  const count = Math.max(total, shown.length);
  const durationMs = shown.reduce((sum, video) => sum + (video.durationMs ?? 0), 0);
  const bytes = shown.reduce((sum, video) => sum + video.sizeBytes, 0);
  const head =
    shown.length === 0
      ? "0 件"
      : shown.length >= count
        ? `${count.toLocaleString("ja-JP")} 件`
        : `1–${shown.length.toLocaleString("ja-JP")} / ${count.toLocaleString("ja-JP")} 件`;
  const detail = [durationMs > 0 ? formatDuration(durationMs) : "", formatBytes(bytes)]
    .filter((part) => part !== "")
    .join(" · ");
  return detail === "" ? head : `${head}（${detail}）`;
}
