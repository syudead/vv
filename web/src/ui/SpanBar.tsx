import { cn } from "@/lib/cn";

import { Button } from "./shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "./shadcn/tooltip";
import { spanPlacement, type TimeSpan } from "./spanPlacement";

// See web/registry/rules/components.md, SpanBar.

export interface SpanBarProps<S extends TimeSpan> {
  /** The lane's length in seconds, the video's length. */
  lengthSeconds: number;
  spans: readonly S[];
  /** Names the lane for screen readers, such as whose spans it holds. */
  label: string;
  /** One short line per span, its tooltip and its button's name. */
  spanLabel: (span: S) => string;
  /** Makes each block a button; without it the blocks are plain and grey. */
  onPress?: (span: S) => void;
  className?: string;
}

const block = "absolute inset-y-0 h-full min-w-1 rounded-full p-0";

/** One lane the length of a video with a block per span at its place and width. */
export function SpanBar<S extends TimeSpan>({
  lengthSeconds,
  spans,
  label,
  spanLabel,
  onPress,
  className,
}: SpanBarProps<S>) {
  return (
    <div
      role="group"
      aria-label={label}
      data-slot="span-bar"
      className={cn("relative h-3 w-full rounded-full bg-muted", className)}
    >
      {spans.map((span) => {
        const place = spanPlacement(span, lengthSeconds);
        if (place === null) return null;
        const style = {
          left: `${String(place.left)}%`,
          width: `${String(place.width)}%`,
        };
        const text = spanLabel(span);
        const key = `${String(span.start)}-${String(span.end)}`;
        if (onPress === undefined) {
          return (
            <span
              key={key}
              title={text}
              className={cn(block, "bg-muted-foreground")}
              style={style}
            />
          );
        }
        return (
          <Tooltip key={key}>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                aria-label={text}
                className={cn(
                  block,
                  "bg-primary hover:bg-primary-hover active:bg-primary-active",
                )}
                style={style}
                onClick={() => onPress(span)}
              />
            </TooltipTrigger>
            <TooltipContent>{text}</TooltipContent>
          </Tooltip>
        );
      })}
    </div>
  );
}
