import { ChevronLeft, ChevronRight } from "lucide-react";

import { cn } from "../lib/cn";
import Tooltip from "../ui/Tooltip";

interface Neighbor {
  /** 移り先の題名。関連動画の並びに無いときは分からない。 */
  title: string | undefined;
  go: () => void;
}

/**
 * NeighborArrows はプレイヤーの左右の端に寄せた、前後の動画へ移るつまみである。
 *
 * 映像を隠さないよう幅は細くし、押しやすいよう縦に長くする。見せる時期は操作バーと同じで、
 * `visible` が偽の間は見えなくし、押せなくする。前後が無い側は出さない。
 */
export default function NeighborArrows({
  previous,
  next,
  visible,
}: {
  previous: Neighbor | undefined;
  next: Neighbor | undefined;
  visible: boolean;
}) {
  return (
    <>
      {previous !== undefined && (
        <Arrow side="left" label="前の動画" neighbor={previous} visible={visible} />
      )}
      {next !== undefined && (
        <Arrow side="right" label="次の動画" neighbor={next} visible={visible} />
      )}
    </>
  );
}

function Arrow({
  side,
  label,
  neighbor,
  visible,
}: {
  side: "left" | "right";
  label: string;
  neighbor: Neighbor;
  visible: boolean;
}) {
  const Icon = side === "left" ? ChevronLeft : ChevronRight;
  const tip = neighbor.title === undefined ? label : `${label}: ${neighbor.title}`;
  return (
    <Tooltip content={tip} side={side === "left" ? "right" : "left"}>
      <button
        type="button"
        aria-label={tip}
        data-neighbor-arrow={side}
        onClick={neighbor.go}
        className={cn(
          // 操作バー（約 3rem）を除いた映像の上下中央に置く。
          "absolute top-[calc(50%-1.5rem)] z-30 flex h-[55%] min-h-18 w-7 -translate-y-1/2 items-center justify-center bg-overlay text-fg transition-[opacity,background-color] duration-150 hover:bg-bg motion-reduce:transition-none sm:h-1/2 sm:min-h-24 sm:w-8",
          side === "left" ? "left-0 rounded-r-lg" : "right-0 rounded-l-lg",
          visible
            ? "opacity-100"
            : // 見えない間は押せなくする。キーボードでフォーカスが来たときだけ見せる。
              "pointer-events-none opacity-0 focus-visible:opacity-100",
        )}
      >
        <Icon className="size-5 sm:size-5.5" aria-hidden="true" />
      </button>
    </Tooltip>
  );
}
