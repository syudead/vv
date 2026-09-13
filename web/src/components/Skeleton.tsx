/**
 * Skeleton は通信中の骨組みを描く（contracts/screen-states.md 1.・2.）。
 *
 * 読み上げには渡さない（aria-hidden）。**領域全体へ「読み込み中」を伝えるのは
 * 呼び出し側の責任である** — 骨組みを 12 個並べて 12 回読み上げても意味が無く、
 * 伝えたいのは「この領域はいま読み込み中である」という 1 つの事実だからである
 * （contracts/screen-states.md 3.「読み上げ」）。呼び出し側は骨組みを包む要素に
 * role="status" などを置くこと。
 *
 * 明滅の動きは motion-reduce: で止める（FR-023 / R-410）。止めるのは動きだけで、
 * 地の色は常に出る。
 */
export default function Skeleton({
  shape = "tile",
  height = "h-4",
  className = "",
}: {
  /** tile は一覧の項目用（枠と同じ 16:9）。row は行用で、高さを height で決める。 */
  shape?: "tile" | "row";
  /** row のときの高さ。Tailwind の実用クラスで渡す（既定は h-4）。 */
  height?: string;
  className?: string;
}) {
  const size = shape === "tile" ? "aspect-video w-full" : `w-full ${height}`;

  return (
    <div
      aria-hidden
      className={`animate-pulse rounded-card bg-surface-raised motion-reduce:animate-none ${size} ${className}`}
    />
  );
}
