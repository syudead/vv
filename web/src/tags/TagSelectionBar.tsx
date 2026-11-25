import {
  Ban,
  Check,
  ChevronDown,
  ListChecks,
  Merge,
  Ellipsis,
  Trash2,
} from "lucide-react";
import { useId } from "react";

import type { TagKind } from "../api/tagKinds";
import { maxTagBatch } from "../api/tags";
import { t, type UiText } from "../i18n";
import { SelectionBar } from "../ui/patterns/selection-bar";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Spinner } from "../ui/shadcn/spinner";
import TagSetKindMenu, { type KindChoice } from "./TagSetKindMenu";

/** What `Select all` offers: the loaded cards, or every card the criteria match. */
export interface TagSelectAll {
  loaded: number;
  matching: number;
  /** Why every matching card cannot be selected, or undefined. */
  matchingReason: UiText | undefined;
  /** The rest of the matching cards are loading. */
  pending: boolean;
  onLoaded: () => void;
  onMatching: () => void;
}

export type TagBulkPending = "confirm" | "kind" | "dialog" | "selectAll" | null;

/** Why a disabled menu item above cannot be chosen, readable without hover or focus. */
function MenuReason({ children }: { children: UiText }) {
  return (
    <p className="max-w-menu px-2 py-1.5 text-xs text-muted-foreground">{children}</p>
  );
}

function SelectAllMenu({ all, disabled }: { all: TagSelectAll; disabled: boolean }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" disabled={disabled}>
          {all.pending ? (
            <Spinner aria-hidden="true" />
          ) : (
            <ListChecks aria-hidden="true" />
          )}
          <span>{t.tags.selection.selectAll}</span>
          <ChevronDown aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuItem onSelect={all.onLoaded}>
          {t.tags.selection.allLoaded(all.loaded)}
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={all.matchingReason !== undefined}
          title={all.matchingReason}
          onSelect={all.onMatching}
        >
          {t.tags.selection.allMatching(all.matching)}
        </DropdownMenuItem>
        {all.matchingReason !== undefined && (
          <MenuReason>{all.matchingReason}</MenuReason>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * The bottom bar while tag cards are selected. An action that cannot apply to the selection stays
 * in place, disabled with its reason, so the bar does not move as the selection changes.
 */
export default function TagSelectionBar({
  count,
  hasTentative,
  hasConfirmed,
  pending,
  selectAll,
  onConfirm,
  onReject,
  onDelete,
  onMerge,
  onClear,
  kinds,
  sharedKind,
  onSetKind,
}: {
  count: number;
  hasTentative: boolean;
  hasConfirmed: boolean;
  pending: TagBulkPending;
  selectAll: TagSelectAll;
  onConfirm: () => void;
  onReject: () => void;
  onDelete: () => void;
  onMerge: () => void;
  onClear: () => void;
  kinds: readonly TagKind[] | undefined;
  /** Shared kind ID or none; undefined for mixed kinds. */
  sharedKind: string | undefined;
  onSetKind: (choice: KindChoice) => void;
}) {
  const reasonId = useId();
  const busy = pending !== null;
  const overLimit = count > maxTagBatch;
  const limitReason = overLimit ? t.tags.selection.overLimit(maxTagBatch) : undefined;
  const disabled = busy || overLimit;
  const tentativeReason = hasTentative ? undefined : t.tags.selection.noTentative;
  const confirmedReason = hasConfirmed ? undefined : t.tags.selection.noConfirmed;
  const shared = {
    title: limitReason,
    "aria-describedby": limitReason === undefined ? undefined : reasonId,
  };

  return (
    <div role="region" aria-label={t.tags.selection.region} className="contents">
      <SelectionBar
        count={t.tags.selection.count(count)}
        clearLabel={t.tags.selection.clear}
        clearDisabled={busy}
        onClear={onClear}
      >
        <SelectAllMenu all={selectAll} disabled={busy} />
        <Button
          variant="ghost"
          size="sm"
          disabled={disabled}
          onClick={onMerge}
          {...shared}
        >
          <Merge aria-hidden="true" />
          <span>{t.tags.selection.mergeInto}</span>
        </Button>
        <TagSetKindMenu
          kinds={kinds}
          checked={sharedKind}
          disabled={disabled}
          pending={pending === "kind"}
          title={limitReason}
          describedBy={shared["aria-describedby"]}
          onChoose={onSetKind}
        />
        <Button
          variant="ghost"
          size="sm"
          disabled={disabled || !hasTentative}
          onClick={onConfirm}
          {...shared}
          title={limitReason ?? tentativeReason}
        >
          {pending === "confirm" ? (
            <Spinner aria-hidden="true" />
          ) : (
            <Check aria-hidden="true" />
          )}
          <span>{t.tags.selection.confirm}</span>
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" disabled={disabled} {...shared}>
              <Ellipsis aria-hidden="true" />
              <span>{t.tags.selection.more}</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              variant="destructive"
              disabled={!hasTentative}
              title={tentativeReason}
              onSelect={onReject}
            >
              <Ban aria-hidden="true" />
              {t.tags.selection.reject}
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              disabled={!hasConfirmed}
              title={confirmedReason}
              onSelect={onDelete}
            >
              <Trash2 aria-hidden="true" />
              {t.tags.selection.delete}
            </DropdownMenuItem>
            {/* A disabled item takes no focus or hover, so its reason is also written out. */}
            {[tentativeReason, confirmedReason].map(
              (reason) =>
                reason !== undefined && <MenuReason key={reason}>{reason}</MenuReason>,
            )}
          </DropdownMenuContent>
        </DropdownMenu>
        {limitReason !== undefined && (
          <span id={reasonId} className="w-full text-end text-xs text-muted-foreground">
            {limitReason}
          </span>
        )}
      </SelectionBar>
    </div>
  );
}
