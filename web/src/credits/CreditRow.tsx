import { Ellipsis, IdCard, Merge, Pencil, Trash2 } from "lucide-react";
import { memo, type Ref } from "react";
import { Link } from "react-router";

import type { Credit, CreditKind } from "../api/credits";
import { t } from "../i18n";
import RowNameInput from "../tags/RowNameInput";
import type { TagFieldError } from "../tags/tagNameField";
import { RowIconButton, RowMoreMenu, type TagRowRefs } from "../tags/TagRow";
import { creditNameRules } from "../ui/CreditCommand";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { kindTexts, secondLine } from "./creditRows";

/** Keep handler identities stable to avoid rerendering memoized rows. */
export interface CreditRowHandlers {
  onStartRename: (credit: Credit) => void;
  onCancelRename: (credit: Credit) => void;
  onSubmitRename: (credit: Credit, name: string) => void;
  onOpenDetails: (credit: Credit) => void;
  onOpenMerge: (credit: Credit) => void;
  onDelete: (credit: Credit) => void;
  onDraftChange: () => void;
}

export const creditRowIdAttribute = "data-credit-id";

/** Virtualized rows are memoized; callers must stabilize props. */
export default memo(CreditRow);

function CreditRow({
  kind,
  index,
  measureRef,
  credit,
  renaming,
  pending,
  blockStart,
  error,
  registerRefs,
  onStartRename,
  onCancelRename,
  onSubmitRename,
  onOpenDetails,
  onOpenMerge,
  onDelete,
  onDraftChange,
}: CreditRowHandlers & {
  kind: CreditKind;
  /** Virtualization reads this position through data-index. */
  index: number;
  measureRef: Ref<HTMLTableRowElement>;
  credit: Credit;
  renaming: boolean;
  pending: boolean;
  /** Another row is saving, so this row cannot start renaming or deletion. */
  blockStart: boolean;
  error: TagFieldError | null;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
}) {
  const line = secondLine(credit);
  const parameter = kind === "person" ? "person" : "organization";
  const mergeLabel = kindTexts(kind).mergeInto;

  return (
    <TableRow ref={measureRef} data-index={index} data-credit-id={credit.id}>
      {/* max-w-0 prevents long names from widening the table. */}
      <TableCell className="w-full max-w-0 whitespace-normal">
        {renaming ? (
          <RowNameInput
            idPrefix={`${parameter}-rename`}
            rowId={credit.id}
            name={credit.name}
            rules={creditNameRules}
            label={t.credits.row.renameLabel(credit.name)}
            pending={pending}
            error={error}
            onSubmit={(name) => onSubmitRename(credit, name)}
            onCancel={() => onCancelRename(credit)}
            onDraftChange={onDraftChange}
          />
        ) : (
          <>
            {/*
             * Expand the link's pseudo-element across the name cell so its whitespace remains
             * clickable.
             */}
            <span className="relative flex min-w-0 items-center">
              <Link
                ref={(node) => registerRefs(credit.id, { nameLink: node })}
                to={`/?${parameter}=${String(credit.id)}`}
                title={credit.name}
                aria-label={t.credits.row.open(credit.name)}
                className="block min-w-0 truncate font-medium text-foreground after:absolute after:inset-0 hover:text-primary"
              >
                {credit.name}
              </Link>
            </span>
            {line !== null && (
              <p title={line} className="truncate text-xs text-muted-foreground">
                {line}
              </p>
            )}
          </>
        )}
      </TableCell>
      <TableCell className="w-16 text-right text-muted-foreground tabular-nums">
        {t.credits.row.videoCount(credit.videoCount)}
      </TableCell>
      <TableCell>
        <div className="hidden justify-end max-sm:flex [@media(pointer:coarse)]:flex">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                ref={(node) => registerRefs(credit.id, { actionsButton: node })}
                variant="ghost"
                size="icon-sm"
                aria-label={t.credits.row.actions}
                disabled={renaming}
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onSelect={() => onStartRename(credit)}
                disabled={blockStart}
              >
                <Pencil aria-hidden="true" />
                {t.credits.row.rename}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onOpenDetails(credit)}>
                <IdCard aria-hidden="true" />
                {t.credits.row.details}
              </DropdownMenuItem>
              <MenuEnd
                mergeLabel={mergeLabel}
                blockStart={blockStart}
                onMerge={() => onOpenMerge(credit)}
                onDelete={() => onDelete(credit)}
              />
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="flex items-center justify-end gap-1 max-sm:hidden [@media(pointer:coarse)]:hidden">
          <RowIconButton
            ref={(node) => registerRefs(credit.id, { renameButton: node })}
            label={t.credits.row.rename}
            onClick={() => onStartRename(credit)}
            disabled={renaming || blockStart}
          >
            <Pencil aria-hidden="true" />
          </RowIconButton>
          <RowIconButton
            ref={(node) => registerRefs(credit.id, { synonymsButton: node })}
            label={t.credits.row.details}
            onClick={() => onOpenDetails(credit)}
            disabled={renaming}
          >
            <IdCard aria-hidden="true" />
          </RowIconButton>
          <RowMoreMenu
            buttonRef={(node) => registerRefs(credit.id, { menuButton: node })}
            label={t.credits.row.more}
            disabled={renaming || blockStart}
          >
            <MenuEnd
              mergeLabel={mergeLabel}
              blockStart={false}
              onMerge={() => onOpenMerge(credit)}
              onDelete={() => onDelete(credit)}
            />
          </RowMoreMenu>
        </div>
      </TableCell>
    </TableRow>
  );
}

function MenuEnd({
  mergeLabel,
  blockStart,
  onMerge,
  onDelete,
}: {
  mergeLabel: string;
  blockStart: boolean;
  onMerge: () => void;
  onDelete: () => void;
}) {
  return (
    <>
      <DropdownMenuItem onSelect={onMerge} disabled={blockStart}>
        <Merge aria-hidden="true" />
        {mergeLabel}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem variant="destructive" onSelect={onDelete} disabled={blockStart}>
        <Trash2 aria-hidden="true" />
        {t.credits.row.delete}
      </DropdownMenuItem>
    </>
  );
}
