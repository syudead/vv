import { CircleDashed, VideoOff, X } from "lucide-react";
import type { RefObject } from "react";

import { t } from "../i18n";
import { Toggle } from "../ui/shadcn/toggle";

export type TagFilter = "tentative" | "unused";

/**
 * TagFilterChips は見出しの下の、効いている絞り込みのチップである（ui-design.md
 * 「Active filters」）。押された Toggle で、押し戻すと `onRemove` を呼ぶ
 * （components.md「Toggle and ToggleGroup」）。どちらも効いていなければ何も描かない。
 */
export default function TagFilterChips({
  tentativeOnly,
  unusedOnly,
  onRemove,
  tentativeRef,
  unusedRef,
}: {
  tentativeOnly: boolean;
  unusedOnly: boolean;
  onRemove: (filter: TagFilter) => void;
  tentativeRef: RefObject<HTMLButtonElement | null>;
  unusedRef: RefObject<HTMLButtonElement | null>;
}) {
  if (!tentativeOnly && !unusedOnly) return null;
  return (
    <ul
      aria-label={t.tags.activeFilters.label}
      className="flex flex-wrap items-center gap-2"
    >
      {tentativeOnly && (
        <li>
          <Toggle
            ref={tentativeRef}
            variant="outline"
            size="sm"
            pressed
            onPressedChange={() => onRemove("tentative")}
            aria-label={t.tags.activeFilters.remove(t.tags.tentativeOnly)}
            title={t.tags.activeFilters.remove(t.tags.tentativeOnly)}
          >
            <CircleDashed aria-hidden="true" />
            {t.tags.tentativeOnly}
            <X aria-hidden="true" />
          </Toggle>
        </li>
      )}
      {unusedOnly && (
        <li>
          <Toggle
            ref={unusedRef}
            variant="outline"
            size="sm"
            pressed
            onPressedChange={() => onRemove("unused")}
            aria-label={t.tags.activeFilters.remove(t.tags.unusedOnly)}
            title={t.tags.activeFilters.remove(t.tags.unusedOnly)}
          >
            <VideoOff aria-hidden="true" />
            {t.tags.unusedOnly}
            <X aria-hidden="true" />
          </Toggle>
        </li>
      )}
    </ul>
  );
}
