import { useId, type Ref, type RefObject } from "react";

import { maxTagBatch } from "../api/tags";
import { t } from "../i18n";
import { Checkbox } from "../ui/shadcn/checkbox";
import { TableHead, TableHeader, TableRow } from "../ui/shadcn/table";

/**
 * TagListHead は表の列の見出しである（ui-design.md「Column header」）。先頭のチェックは
 * 「読み込んだものをすべて選ぶ」。読み込んだ行が上限を超えると押せず、理由を包みの
 * title と sr-only で添える。
 */
export default function TagListHead({
  headRef,
  selectAllRef,
  checked,
  selectableCount,
  overLimit,
  onToggle,
}: {
  headRef: Ref<HTMLTableSectionElement>;
  selectAllRef: RefObject<HTMLButtonElement | null>;
  checked: boolean | "indeterminate";
  selectableCount: number;
  overLimit: boolean;
  onToggle: () => void;
}) {
  const limitId = useId();
  return (
    <TableHeader ref={headRef}>
      <TableRow className="hover:bg-transparent">
        <TableHead>
          <div
            className="flex size-8 items-center justify-center"
            title={overLimit ? t.tags.selectAllOverLimit(maxTagBatch) : undefined}
          >
            <Checkbox
              ref={selectAllRef}
              checked={checked}
              onCheckedChange={onToggle}
              aria-label={
                checked === true
                  ? t.tags.clearSelection
                  : t.tags.selectAllLoaded(selectableCount)
              }
              aria-describedby={overLimit ? limitId : undefined}
              disabled={selectableCount === 0 || overLimit}
            />
            {overLimit && (
              <span id={limitId} className="sr-only">
                {t.tags.selectAllOverLimit(maxTagBatch)}
              </span>
            )}
          </div>
        </TableHead>
        <TableHead className="w-full">{t.tags.columns.name}</TableHead>
        <TableHead className="text-right">{t.tags.columns.videos}</TableHead>
        <TableHead aria-hidden="true" />
      </TableRow>
    </TableHeader>
  );
}
