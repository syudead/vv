import { ChevronRight } from "lucide-react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import { CollapsibleTrigger } from "@/ui/shadcn/collapsible";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/ui/shadcn/tooltip";

// The expand button of a section that shows part of itself through a Collapsible; patterns only.

/** The expand button's text for its current state: a tooltip and the name read aloud. */
export interface ExpandToggleLabel {
  tooltip: string;
  name: string;
}

export interface ExpandToggleProps {
  expanded: boolean;
  label: ExpandToggleLabel | undefined;
  /** The button's `data-slot`, which its section's selectors read. */
  slot: string;
}

/** A `ghost` `icon-sm` trigger whose chevron turns down while its section is expanded. */
export function ExpandToggle({ expanded, label, slot }: ExpandToggleProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <CollapsibleTrigger asChild>
          <Button
            data-slot={slot}
            variant="ghost"
            size="icon-sm"
            aria-label={label?.name}
          >
            <ChevronRight
              aria-hidden="true"
              className={cn("transition-transform", expanded && "rotate-90")}
            />
          </Button>
        </CollapsibleTrigger>
      </TooltipTrigger>
      <TooltipContent>{label?.tooltip}</TooltipContent>
    </Tooltip>
  );
}
