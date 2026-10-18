import {
  Ban,
  Check,
  Ellipsis,
  Merge,
  Pencil,
  Tags as SynonymsIcon,
  Trash2,
} from "lucide-react";
import { type ComponentProps, memo, type ReactNode, type Ref } from "react";
import { Link } from "react-router";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { libraryFilterPath } from "../library/libraryPath";
import { TAG_PARAM } from "../library/tagCriteria";
import { Button } from "../ui/shadcn/button";
import { Checkbox } from "../ui/shadcn/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Spinner } from "../ui/shadcn/spinner";
import { TableCell, TableRow } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import TentativeMark from "../ui/TentativeMark";
import RowNameInput from "./RowNameInput";
import TagKindLabel from "./TagKindLabel";
import type { TagFieldError } from "./tagNameField";

export interface TagRowRefs {
  nameLink: HTMLAnchorElement | null;
  renameButton: HTMLButtonElement | null;
  synonymsButton: HTMLButtonElement | null;
  menuButton: HTMLButtonElement | null;
  actionsButton: HTMLButtonElement | null;
}

/** Rename only the name cell while preserving row layout. Memoization requires stable props because virtualization rerenders on scrolling/search. */
export default memo(TagRow);

function TagRow({
  index,
  measureRef,
  tag,
  renaming,
  pending,
  blockStart,
  error,
  registerRefs,
  onStartRename,
  onCancelRename,
  onSubmitRename,
  onOpenSynonyms,
  onOpenMerge,
  onDelete,
  confirming = false,
  onConfirm,
  onReject,
  onDraftChange,
  selected = false,
  onSelect,
}: {
  /** Virtual row index used by measurement. */
  index: number;
  measureRef: Ref<HTMLTableRowElement>;
  tag: Tag;
  renaming: boolean;
  pending: boolean;
  /** Another row's pending mutation blocks starting edits here. */
  blockStart: boolean;
  error: TagFieldError | null;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
  onStartRename: (tag: Tag) => void;
  onCancelRename: (tag: Tag) => void;
  onSubmitRename: (tag: Tag, name: string) => void;
  onOpenSynonyms: (tag: Tag) => void;
  onOpenMerge: (tag: Tag) => void;
  onDelete: (tag: Tag) => void;
  confirming?: boolean;
  onConfirm?: (tag: Tag) => void;
  onReject?: (tag: Tag) => void;
  onDraftChange?: () => void;
  selected?: boolean;
  onSelect?: (tag: Tag, selected: boolean) => void;
}) {
  return (
    <TableRow
      ref={measureRef}
      data-index={index}
      data-tag-id={tag.id}
      data-state={selected && !renaming ? "selected" : undefined}
    >
      <TableCell>
        {/* Do not select renaming rows: saving may move them to a different position. */}
        <div
          className="-my-1.5 flex size-8 items-center justify-center"
          onClick={(event) => {
            if (event.target !== event.currentTarget || renaming) return;
            onSelect?.(tag, !selected);
          }}
        >
          <Checkbox
            checked={selected && !renaming}
            onCheckedChange={(next) => onSelect?.(tag, next === true)}
            aria-label={t.tags.row.select(tag.name)}
            disabled={renaming}
          />
        </div>
      </TableCell>
      {/* max-w-0 keeps long names from expanding the table. */}
      <TableCell className="w-full max-w-0 whitespace-normal">
        {renaming ? (
          <RowNameInput
            idPrefix="tag-rename"
            rowId={tag.id}
            name={tag.name}
            label={t.tags.row.renameLabel(tag.name)}
            pending={pending}
            error={error}
            onSubmit={(name) => onSubmitRename(tag, name)}
            onCancel={() => onCancelRename(tag)}
            onDraftChange={onDraftChange}
          />
        ) : (
          <>
            {/* Expand the link hit area while keeping tentative markers above it for tooltip input. */}
            <span className="relative flex min-w-0 items-center gap-1">
              <Link
                ref={(node) => registerRefs(tag.id, { nameLink: node })}
                to={libraryFilterPath(TAG_PARAM, tag.id)}
                title={tag.name}
                aria-label={t.tags.row.open(tag.name)}
                className="block min-w-0 truncate font-medium text-foreground after:absolute after:inset-0 hover:text-primary"
              >
                {tag.name}
              </Link>
              {tag.tentative && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="relative inline-flex shrink-0">
                      <TentativeMark size="row" />
                      <span className="sr-only">{t.tags.tentative}</span>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>{t.tags.tentative}</TooltipContent>
                </Tooltip>
              )}
              <TagKindLabel kind={tag.kind} />
            </span>
            {tag.synonyms.length > 0 && (
              <p
                title={t.tags.row.synonyms(tag.synonyms.join(" · "))}
                className="truncate text-xs text-muted-foreground"
              >
                {t.tags.row.synonyms(tag.synonyms.join(" · "))}
              </p>
            )}
          </>
        )}
      </TableCell>
      <TableCell className="text-right text-muted-foreground tabular-nums">
        {t.tags.row.videoCount(tag.videoCount)}
      </TableCell>
      <TableCell>
        <CompactActions
          tag={tag}
          renaming={renaming}
          blockStart={blockStart}
          confirming={confirming}
          registerRefs={registerRefs}
          onStartRename={onStartRename}
          onOpenSynonyms={onOpenSynonyms}
          onOpenMerge={onOpenMerge}
          onDelete={onDelete}
          onConfirm={onConfirm}
          onReject={onReject}
        />
        <div className="flex items-center justify-end gap-1 max-sm:hidden [@media(pointer:coarse)]:hidden">
          {/* Reserve confirmation width on all rows to align actions. */}
          {!tag.tentative && <span aria-hidden="true" className="size-8 shrink-0" />}
          {tag.tentative && (
            <RowIconButton
              label={t.tags.row.confirm}
              aria-busy={confirming || undefined}
              aria-disabled={confirming || undefined}
              onClick={() => {
                // Preserve button focus while pending; suppress repeat clicks instead of disabling it.
                if (confirming) return;
                onConfirm?.(tag);
              }}
              disabled={renaming}
            >
              {confirming ? <Spinner aria-hidden="true" /> : <Check aria-hidden="true" />}
            </RowIconButton>
          )}
          <RowIconButton
            ref={(node) => registerRefs(tag.id, { renameButton: node })}
            label={t.tags.row.rename}
            onClick={() => onStartRename(tag)}
            disabled={renaming || blockStart || confirming}
          >
            <Pencil aria-hidden="true" />
          </RowIconButton>
          <RowIconButton
            ref={(node) => registerRefs(tag.id, { synonymsButton: node })}
            label={t.tags.row.synonymsButton}
            onClick={() => onOpenSynonyms(tag)}
            disabled={renaming}
          >
            <SynonymsIcon aria-hidden="true" />
          </RowIconButton>
          <RowMoreMenu
            buttonRef={(node) => registerRefs(tag.id, { menuButton: node })}
            label={t.tags.row.more}
            disabled={renaming || blockStart || confirming}
          >
            <DropdownMenuItem onSelect={() => onOpenMerge(tag)}>
              <Merge aria-hidden="true" />
              {t.tags.row.merge}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            {tag.tentative ? (
              <DropdownMenuItem variant="destructive" onSelect={() => onReject?.(tag)}>
                <Ban aria-hidden="true" />
                {t.tags.row.reject}
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem variant="destructive" onSelect={() => onDelete(tag)}>
                <Trash2 aria-hidden="true" />
                {t.tags.row.delete}
              </DropdownMenuItem>
            )}
          </RowMoreMenu>
        </div>
      </TableCell>
    </TableRow>
  );
}

export function RowIconButton({
  label,
  ...props
}: Omit<ComponentProps<typeof Button>, "variant" | "size" | "aria-label"> & {
  label: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={label} {...props} />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export function RowMoreMenu({
  buttonRef,
  label,
  disabled,
  children,
}: {
  buttonRef: Ref<HTMLButtonElement>;
  label: string;
  disabled: boolean;
  children: ReactNode;
}) {
  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              ref={buttonRef}
              variant="ghost"
              size="icon-sm"
              aria-label={label}
              disabled={disabled}
            >
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}

/** CSS selects compact actions for touch/narrow layouts and removes hidden actions from tab order. */
function CompactActions({
  tag,
  renaming,
  blockStart,
  confirming,
  registerRefs,
  onStartRename,
  onOpenSynonyms,
  onOpenMerge,
  onDelete,
  onConfirm,
  onReject,
}: {
  tag: Tag;
  renaming: boolean;
  blockStart: boolean;
  confirming: boolean;
  registerRefs: (id: number, refs: Partial<TagRowRefs>) => void;
  onStartRename: (tag: Tag) => void;
  onOpenSynonyms: (tag: Tag) => void;
  onOpenMerge: (tag: Tag) => void;
  onDelete: (tag: Tag) => void;
  onConfirm?: (tag: Tag) => void;
  onReject?: (tag: Tag) => void;
}) {
  const startBlocked = blockStart || confirming;
  return (
    <div className="hidden justify-end max-sm:flex [@media(pointer:coarse)]:flex">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            ref={(node) => registerRefs(tag.id, { actionsButton: node })}
            variant="ghost"
            size="icon-sm"
            aria-label={t.tags.row.actions}
            aria-busy={confirming || undefined}
            disabled={renaming}
            onPointerDown={(event) => {
              if (confirming) event.preventDefault();
            }}
            onKeyDown={(event) => {
              if (confirming && (event.key === "Enter" || event.key === " ")) {
                event.preventDefault();
              }
            }}
          >
            {confirming ? (
              <Spinner aria-hidden="true" />
            ) : (
              <Ellipsis aria-hidden="true" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {tag.tentative && (
            <DropdownMenuItem onSelect={() => onConfirm?.(tag)} disabled={confirming}>
              <Check aria-hidden="true" />
              {t.tags.row.confirm}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => onStartRename(tag)} disabled={startBlocked}>
            <Pencil aria-hidden="true" />
            {t.tags.row.rename}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onOpenSynonyms(tag)}>
            <SynonymsIcon aria-hidden="true" />
            {t.tags.row.synonymsButton}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => onOpenMerge(tag)} disabled={startBlocked}>
            <Merge aria-hidden="true" />
            {t.tags.row.merge}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          {tag.tentative ? (
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => onReject?.(tag)}
              disabled={startBlocked}
            >
              <Ban aria-hidden="true" />
              {t.tags.row.reject}
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => onDelete(tag)}
              disabled={startBlocked}
            >
              <Trash2 aria-hidden="true" />
              {t.tags.row.delete}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
