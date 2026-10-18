import { ChevronLeft, ChevronRight } from "lucide-react";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";

export interface Neighbor {
  /** Unknown when the destination is absent from related results. */
  title: string | undefined;
  go: () => void;
}

/** Match control-bar visibility and disable hidden navigation. */
export default function NeighborArrows({
  previous,
  next,
  visible,
  container,
  labels = { previous: t.player.neighbors.previous, next: t.player.neighbors.next },
}: {
  previous: Neighbor | undefined;
  next: Neighbor | undefined;
  visible: boolean;
  labels?: { previous: UiText; next: UiText };
  /** Tooltips must render inside the fullscreen container to remain visible. */
  container: HTMLElement | null;
}) {
  return (
    <>
      {previous !== undefined && (
        <Arrow
          side="left"
          label={labels.previous}
          neighbor={previous}
          visible={visible}
          container={container}
        />
      )}
      {next !== undefined && (
        <Arrow
          side="right"
          label={labels.next}
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
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          aria-label={tip}
          data-neighbor-arrow={side}
          onClick={neighbor.go}
          className={cn(
            // Center above the control bar, 1.5rem above the full frame's midpoint.
            "absolute top-1/2 z-30 -mt-6 h-11/20 min-h-neighbor-arrow-min w-neighbor-arrow -translate-y-1/2 bg-overlay px-0 text-foreground transition duration-150 hover:bg-background motion-reduce:transition-none sm:h-1/2 sm:min-h-neighbor-arrow-min-sm sm:w-8 [&_svg]:size-5",
            side === "left"
              ? "left-0 rounded-l-none rounded-r-lg"
              : "right-0 rounded-l-lg rounded-r-none",
            visible
              ? "opacity-100"
              : // Hidden arrows remain keyboard-focusable but ignore pointer input.
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
  );
}
