import { ChevronLeft, ChevronRight } from "lucide-react";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

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
  container,
}: {
  previous: Neighbor | undefined;
  next: Neighbor | undefined;
  visible: boolean;
  /** 全画面にしている入れ物。題名の吹き出しはその中に描かないと全画面の間に見えない。 */
  container: HTMLElement | null;
}) {
  return (
    <>
      {previous !== undefined && (
        <Arrow
          side="left"
          label={t.player.neighbors.previous}
          neighbor={previous}
          visible={visible}
          container={container}
        />
      )}
      {next !== undefined && (
        <Arrow
          side="right"
          label={t.player.neighbors.next}
          neighbor={next}
          visible={visible}
          container={container}
        />
      )}
    </>
  );
}

function Arrow({
  side,
  label,
  neighbor,
  visible,
  container,
}: {
  side: "left" | "right";
  label: UiText;
  neighbor: Neighbor;
  visible: boolean;
  container: HTMLElement | null;
}) {
  const Icon = side === "left" ? ChevronLeft : ChevronRight;
  const tip =
    neighbor.title === undefined
      ? label
      : t.player.neighbors.withTitle(label, neighbor.title);
  return (
    // 操作バー（約 3rem、bottom-12）を除いた映像の上下中央に置く。入れ物は映像への操作を
    // 通し、つまみだけを押せるようにする。
    <div
      className={cn(
        "pointer-events-none absolute top-0 bottom-12 z-30 flex items-center",
        side === "left" ? "left-0" : "right-0",
      )}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            aria-label={tip}
            data-neighbor-arrow={side}
            onClick={neighbor.go}
            className={cn(
              "h-1/2 min-h-16 w-8 bg-overlay px-0 text-foreground transition duration-150 hover:bg-background motion-reduce:transition-none [&_svg]:size-5",
              side === "left"
                ? "rounded-l-none rounded-r-lg"
                : "rounded-l-lg rounded-r-none",
              visible
                ? "pointer-events-auto opacity-100"
                : // 見えない間は押せなくする。キーボードでフォーカスが来たときだけ見せる。
                  "pointer-events-none opacity-0 focus-visible:opacity-100",
            )}
          >
            <Icon aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side={side === "left" ? "right" : "left"} container={container}>
          {tip}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}
