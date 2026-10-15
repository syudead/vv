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

/** ChipItem は比較の行に並べるチップの 1 つ（タグ、人物・団体、リンク、ID）である。 */
export interface ChipItem {
  /** 並びの中で一意な鍵。無ければ名前。 */
  key?: string;
  name: string;
  /** 仮のタグとして出す（破線の目印）。 */
  tentative?: boolean;
  /**
   * 取得した側の印。`onVideo` は薄いチップと Check、`new` は「New」、`notLinked` は薄いチップと
   * 「Not linked」（specs/052-plugin-work-info-fields/ui-design.md「Words」）。
   */
  mark?: ChipMark;
  /** ツールチップ。無ければ `onVideo` のチップだけが「On the video」を出す。 */
  hint?: string;
  /** 省略せずに見せる値（`title`）。無ければ名前。 */
  full?: string;
  /** 品番・ID のように数字の幅を揃える。 */
  numeric?: boolean;
}

/** chipsOfVideo は動画のタグを比較の行のチップにする。 */
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
  // 名前を先に読ませる従の印。読み上げは「{name}, New」になる。
  return (
    <span className="shrink-0 text-xs text-muted-foreground">
      <span className="sr-only">, </span>
      {mark === "new" ? text.newMark : text.notLinked}
    </span>
  );
}

function Chip({ item }: { item: ChipItem }) {
  const text = t.player.workInfoFetch;
  const muted = item.mark === "onVideo" || item.mark === "notLinked";
  const hint = item.hint ?? (item.mark === "onVideo" ? text.onVideo : undefined);
  // 動画にある件は確定した値として出し、仮の印を付けない（049 の取得した側のチップ）。
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

/**
 * TagChips はチップの並びで、11 以上なら先頭の 10 と「+{n} more」を出し、押すとその場で
 * 残りを出して「Show fewer」にする（ui-design「Compare」）。trailing は並びの後ろの添え書きである。
 */
export default function TagChips({
  items,
  trailing,
  trailingId,
}: {
  items: readonly ChipItem[];
  trailing?: string;
  /** trailing の要素の id。選べない行の理由としてチェックボックスが指す。 */
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
