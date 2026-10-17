import { CircleDashed, Shapes, VideoOff, X } from "lucide-react";
import type { ReactNode, RefObject } from "react";

import { t, type UiText } from "../i18n";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Toggle } from "../ui/shadcn/toggle";
import type { TagKindFilter } from "./tagListUrl";

export type TagFilter = "tentative" | "unused" | "kind";

/** tagFilterOrder はチップの並び順である。外したあとのフォーカスの行き先もこの順で探す。 */
export const tagFilterOrder: readonly TagFilter[] = ["tentative", "unused", "kind"];

/**
 * TagFilterChips は見出しの下の、効いている絞り込みのチップである。押された Toggle で、押し戻すと `onRemove` を呼ぶ
 * （components.md「Toggle and ToggleGroup」）。どれも効いていなければ何も描かない。
 *
 * 種類のチップは `Shapes` を添えて
 * `Kind: Genre` か `No kind` と書き、タグの絞り込みと読み違えないようにする。種類の名前を
 * まだ読んでいない間は名前の代わりに `Skeleton` を置き、読み上げ名を「Remove the kind
 * filter」にする。
 */
export default function TagFilterChips({
  tentativeOnly,
  unusedOnly,
  kind,
  kindName,
  onRemove,
  chipRefs,
}: {
  tentativeOnly: boolean;
  unusedOnly: boolean;
  kind: TagKindFilter;
  /** 種類の絞り込みの種類の名前。読み込み前（と `none`）は undefined。 */
  kindName: string | undefined;
  onRemove: (filter: TagFilter) => void;
  chipRefs: Record<TagFilter, RefObject<HTMLButtonElement | null>>;
}) {
  if (!tentativeOnly && !unusedOnly && kind === null) return null;
  const kindText = kind === "none" ? t.tags.kindFilter.none : kindChipText(kindName);
  return (
    <ul
      aria-label={t.tags.activeFilters.label}
      className="flex flex-wrap items-center gap-2"
    >
      {tentativeOnly && (
        <FilterChip
          buttonRef={chipRefs.tentative}
          onRemove={() => onRemove("tentative")}
          label={t.tags.activeFilters.remove(t.tags.tentativeOnly)}
          icon={<CircleDashed aria-hidden="true" />}
        >
          {t.tags.tentativeOnly}
        </FilterChip>
      )}
      {unusedOnly && (
        <FilterChip
          buttonRef={chipRefs.unused}
          onRemove={() => onRemove("unused")}
          label={t.tags.activeFilters.remove(t.tags.unusedOnly)}
          icon={<VideoOff aria-hidden="true" />}
        >
          {t.tags.unusedOnly}
        </FilterChip>
      )}
      {kind !== null && (
        <FilterChip
          buttonRef={chipRefs.kind}
          onRemove={() => onRemove("kind")}
          label={
            kindText === undefined
              ? t.tags.kindFilter.removeUnnamed
              : t.tags.activeFilters.remove(kindText)
          }
          icon={<Shapes aria-hidden="true" />}
        >
          {kindText === undefined ? (
            <Skeleton className="h-3 w-12" />
          ) : (
            <span title={kindText} className="max-w-chip-label min-w-0 truncate">
              {kindText}
            </span>
          )}
        </FilterChip>
      )}
    </ul>
  );
}

function kindChipText(name: string | undefined): UiText | undefined {
  return name === undefined ? undefined : t.tags.kindFilter.chip(name);
}

function FilterChip({
  buttonRef,
  onRemove,
  label,
  icon,
  children,
}: {
  buttonRef: RefObject<HTMLButtonElement | null>;
  onRemove: () => void;
  label: UiText;
  icon: ReactNode;
  children: ReactNode;
}) {
  return (
    <li>
      <Toggle
        ref={buttonRef}
        variant="outline"
        size="sm"
        pressed
        onPressedChange={onRemove}
        aria-label={label}
        title={label}
      >
        {icon}
        {children}
        <X aria-hidden="true" />
      </Toggle>
    </li>
  );
}
