import { Check } from "lucide-react";
import { useState } from "react";

import type { VideoTag } from "../../api/client";
import { t } from "../../i18n";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import TentativeMark from "../../ui/TentativeMark";
import { TAG_FOLD } from "./fetchRules";

/** ChipItem は比較の行に並べるタグの 1 つである。 */
export interface ChipItem {
  name: string;
  /** 仮のタグとして出す（破線の目印）。 */
  tentative: boolean;
  /** 動画に付いている名前で、取得した側では薄いチップと Check で出す。 */
  onVideo?: boolean;
}

/** chipsOfVideo は動画のタグを比較の行のチップにする。 */
export function chipsOfVideo(tags: readonly VideoTag[]): ChipItem[] {
  return tags.map((tag) => ({ name: tag.name, tentative: tag.tentative }));
}

function Chip({ item }: { item: ChipItem }) {
  const text = t.player.workInfoFetch;
  if (item.onVideo === true) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge
            variant="secondary"
            title={item.name}
            className="max-w-full justify-start font-normal text-muted-foreground"
          >
            <span className="min-w-0 truncate">{item.name}</span>
            <Check aria-hidden="true" />
            <span className="sr-only">{`, ${text.onVideo}`}</span>
          </Badge>
        </TooltipTrigger>
        <TooltipContent>{text.onVideo}</TooltipContent>
      </Tooltip>
    );
  }
  return (
    <Badge
      variant="secondary"
      title={item.name}
      className="max-w-full justify-start font-normal"
    >
      <span className="min-w-0 truncate">{item.name}</span>
      {item.tentative && <TentativeMark />}
      {item.tentative && <span className="sr-only">{`, ${text.tentative}`}</span>}
    </Badge>
  );
}

/**
 * TagChips はタグのチップの並びで、11 以上なら先頭の 10 と「+{n} more」を出し、押すとその場で
 * 残りを出して「Show fewer」にする（ui-design「Compare」）。trailing は並びの後ろの添え書きである。
 */
export default function TagChips({
  items,
  trailing,
}: {
  items: readonly ChipItem[];
  trailing?: string;
}) {
  const text = t.player.workInfoFetch;
  const [open, setOpen] = useState(false);
  const folded = !open && items.length > TAG_FOLD;
  const shown = folded ? items.slice(0, TAG_FOLD) : items;
  return (
    <>
      {shown.map((item) => (
        <Chip key={item.name} item={item} />
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
        <span className="text-xs text-muted-foreground">{trailing}</span>
      )}
    </>
  );
}
