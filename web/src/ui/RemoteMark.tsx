import { Laptop } from "lucide-react";

import type { UiText } from "@/i18n";
import { cn } from "@/lib/cn";
import { Tooltip, TooltipContent, TooltipTrigger } from "./shadcn/tooltip";

/**
 * The mark of an item the paired VVMDM holds, drawn in its line's text colour. Not a control: a link
 * that contains it references the sr-only text through `id` in its aria-describedby.
 */
export default function RemoteMark({
  label,
  id,
  size = "sm",
}: {
  /** "On {name}": the tooltip and the screen-reader text. */
  label: UiText;
  id?: string;
  /** `sm` in pills, cells and count lines; `md` in the video page's band. */
  size?: "sm" | "md";
}) {
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            data-slot="remote-mark"
            className="pointer-events-auto inline-flex shrink-0"
          >
            <Laptop
              aria-hidden="true"
              className={cn(size === "sm" ? "size-3" : "size-4")}
            />
          </span>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <span id={id} className="sr-only">
        {label}
      </span>
    </>
  );
}
