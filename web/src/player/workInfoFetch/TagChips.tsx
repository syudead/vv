import { Check } from "lucide-react";
import { useState } from "react";

import type { VideoTag } from "../../api/client";
import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import TentativeMark from "../../ui/TentativeMark";
import type { ChipMark } from "./detailRows";
import { TAG_FOLD } from "./fetchRules";

export interface ChipItem {
  key?: string;
  name: string;
  tentative?: boolean;
  mark?: ChipMark;
  hint?: string;
  full?: string;
  numeric?: boolean;
}

export function chipsOfVideo(tags: readonly VideoTag[]): ChipItem[] {
  return tags.map((tag) => ({ name: tag.name, tentative: tag.tentative }));
}

function MarkText({ mark }: { mark: ChipMark | undefined }) {
  const text = t.player.workInfoFetch;
  if (mark === "onVideo")
    return (
      <>
        <Check aria-hidden="true" />
        <span className="sr-only">{`, ${text.onVideo}`}</span>
      </>
    );
  if (mark === undefined) return null;
  // Announce the name before the secondary marker.
  return (
    <span className="shrink-0 text-xs text-muted-foreground">
      <span className="sr-only">, </span>
      {mark === "new" ? text.newMark : text.notLinked}
    </span>
  );
}

export function Chip({ item }: { item: ChipItem }) {
  const text = t.player.workInfoFetch;
  const muted = item.mark === "onVideo" || item.mark === "notLinked";
  const hint = item.hint ?? (item.mark === "onVideo" ? text.onVideo : undefined);
  // Existing video values are confirmed, without tentative markers.
  const tentative = item.tentative === true && item.mark !== "onVideo";
  const badge = (
    <Badge
      variant="secondary"
      title={item.full ?? item.name}
      className={cn(
        "max-w-full justify-start font-normal",
        muted && "text-muted-foreground",
      )}
    >
      <span className={cn("min-w-0 truncate", item.numeric === true && "tabular-nums")}>
        {item.name}
      </span>
      <MarkText mark={item.mark} />
      {tentative && <TentativeMark />}
      {tentative && <span className="sr-only">{`, ${text.tentative}`}</span>}
    </Badge>
  );
  if (hint === undefined) return badge;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  );
}

export default function TagChips({
  items,
  trailing,
  trailingId,
}: {
  items: readonly ChipItem[];
  trailing?: string;
  /** Checkbox description target for an unselectable row. */
  trailingId?: string;
}) {
  const text = t.player.workInfoFetch;
  const [open, setOpen] = useState(false);
  const folded = !open && items.length > TAG_FOLD;
  const shown = folded ? items.slice(0, TAG_FOLD) : items;
  return (
    <>
      {shown.map((item) => (
        <Chip key={item.key ?? item.name} item={item} />
      ))}
      {items.length > TAG_FOLD && (
        <Button
          variant="link"
          size="sm"
          className="h-auto px-0 py-0 text-muted-foreground hover:text-primary"
          onClick={() => setOpen(folded)}
        >
          {folded ? text.more(items.length - TAG_FOLD) : text.showFewer}
        </Button>
      )}
      {trailing !== undefined && (
        <span id={trailingId} className="text-xs text-muted-foreground">
          {trailing}
        </span>
      )}
    </>
  );
}
