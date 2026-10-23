import { Ellipsis, Pencil, Plus, Shapes, Trash2 } from "lucide-react";
import type { MouseEvent } from "react";
import { Link } from "react-router";

import type { TagKind } from "../api/tagKinds";
import { t } from "../i18n";
import { isPlainClick } from "../lib/plainClick";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadingState } from "../ui/patterns/loading-state";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Spinner } from "../ui/shadcn/spinner";
import { Switch } from "../ui/shadcn/switch";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../ui/shadcn/table";
import CreateTagRow from "./CreateTagRow";
import { DialogError } from "./DialogError";
import RowNameInput from "./RowNameInput";
import { RowIconButton, RowMoreMenu } from "./TagRow";
import type { TagKindsTab } from "./useTagKindsTab";

const kindColumnCount = 4;

export default function TagKindsPanel({
  state,
  hrefFor,
  onOpenKind,
}: {
  state: TagKindsTab;
  hrefFor: (kind: TagKind) => string;
  onOpenKind: (kind: TagKind) => void;
}) {
  const { loadError, creating } = state;
  // Hide stale lists after read failure because deletion confirmation relies on their counts.
  const kinds = loadError === null ? state.kinds : undefined;
  return (
    <>
      {state.kinds === undefined && loadError === null && (
        <LoadingState label={t.tags.kinds.loading} layout="table" count={4} />
      )}
      {loadError !== null && (
        <ErrorState
          title={t.tags.kinds.loadFailed}
          retryLabel={t.common.retry}
          onRetry={state.reload}
        />
      )}
      {kinds !== undefined && kinds.length === 0 && !creating && (
        <EmptyState
          icon={<Shapes aria-hidden="true" />}
          title={t.tags.kinds.empty.title}
          description={t.tags.kinds.empty.description}
          action={
            <Button size="sm" onClick={state.openCreate}>
              <Plus aria-hidden="true" />
              {t.tags.kinds.newKind}
            </Button>
          }
        />
      )}
      {kinds !== undefined && (kinds.length > 0 || creating) && (
        <>
          <p className="text-sm text-muted-foreground">{t.tags.kinds.description}</p>
          <DataTable label={t.tags.kinds.table}>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="w-full">{t.tags.kinds.columns.name}</TableHead>
                <TableHead className="whitespace-nowrap">
                  {t.tags.kinds.columns.cardFirst}
                </TableHead>
                <TableHead className="text-right">{t.tags.kinds.columns.tags}</TableHead>
                <TableHead aria-hidden="true" />
              </TableRow>
            </TableHeader>
            {creating && (
              <TableBody>
                <CreateTagRow
                  columnCount={kindColumnCount}
                  pending={state.createPending}
                  error={state.createError}
                  onCancel={state.cancelCreate}
                  onSubmit={state.submitCreate}
                  onDraftChange={state.clearCreateError}
                  label={t.tags.kinds.create.label}
                  placeholder={t.tags.kinds.create.placeholder}
                />
              </TableBody>
            )}
            <TableBody>
              {kinds.map((kind) => (
                <KindRow
                  key={kind.id}
                  kind={kind}
                  state={state}
                  href={hrefFor(kind)}
                  onOpen={onOpenKind}
                />
              ))}
            </TableBody>
          </DataTable>
        </>
      )}
      {state.deleting !== null && (
        <DeleteKindDialog state={state} kind={state.deleting} />
      )}
    </>
  );
}

function KindRow({
  kind,
  state,
  href,
  onOpen,
}: {
  kind: TagKind;
  state: TagKindsTab;
  href: string;
  onOpen: (kind: TagKind) => void;
}) {
  const renaming = state.renamingId === kind.id;
  const blockStart = state.createPending || (state.renamePending && !renaming);
  const register = state.registerRefs;

  function open(event: MouseEvent<HTMLAnchorElement>) {
    if (!isPlainClick(event)) return;
    event.preventDefault();
    onOpen(kind);
  }

  return (
    <TableRow data-kind-id={kind.id}>
      <TableCell className="w-full max-w-0 whitespace-normal">
        {renaming ? (
          <RowNameInput
            idPrefix="kind-rename"
            rowId={kind.id}
            name={kind.name}
            label={t.tags.kinds.renameLabel(kind.name)}
            pending={state.renamePending}
            error={state.renameError}
            onSubmit={(name) => state.submitRename(kind, name)}
            onCancel={() => state.cancelRename(kind)}
            onDraftChange={state.clearRenameError}
          />
        ) : (
          <span className="relative flex min-w-0 items-center">
            <Link
              ref={(node) => register(kind.id, { nameLink: node })}
              to={href}
              onClick={open}
              title={kind.name}
              aria-label={t.tags.kinds.open(kind.name)}
              className="block min-w-0 truncate font-medium text-foreground after:absolute after:inset-0 hover:text-primary"
            >
              {kind.name}
            </Link>
          </span>
        )}
      </TableCell>
      <TableCell>
        <Switch
          checked={kind.cardFirst}
          disabled={state.cardFirstPendingId === kind.id}
          onCheckedChange={(next) => state.setCardFirst(kind, next)}
          aria-label={t.tags.kinds.cardFirst(kind.name)}
        />
      </TableCell>
      <TableCell className="text-right text-muted-foreground tabular-nums">
        {t.tags.kinds.tagCount(kind.tagCount)}
      </TableCell>
      <TableCell>
        <div className="hidden justify-end max-sm:flex [@media(pointer:coarse)]:flex">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                ref={(node) => register(kind.id, { actionsButton: node })}
                variant="ghost"
                size="icon-sm"
                aria-label={t.tags.row.actions}
                disabled={renaming}
              >
                <Ellipsis aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onSelect={() => state.startRename(kind)}
                disabled={blockStart}
              >
                <Pencil aria-hidden="true" />
                {t.tags.row.rename}
              </DropdownMenuItem>
              <DeleteItem disabled={blockStart} onSelect={() => state.openDelete(kind)} />
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="flex items-center justify-end gap-1 max-sm:hidden [@media(pointer:coarse)]:hidden">
          <RowIconButton
            ref={(node) => register(kind.id, { renameButton: node })}
            label={t.tags.row.rename}
            onClick={() => state.startRename(kind)}
            disabled={renaming || blockStart}
          >
            <Pencil aria-hidden="true" />
          </RowIconButton>
          <RowMoreMenu
            buttonRef={(node) => register(kind.id, { menuButton: node })}
            label={t.tags.row.more}
            disabled={renaming || blockStart}
          >
            <DeleteItem disabled={false} onSelect={() => state.openDelete(kind)} />
          </RowMoreMenu>
        </div>
      </TableCell>
    </TableRow>
  );
}

function DeleteItem({ disabled, onSelect }: { disabled: boolean; onSelect: () => void }) {
  return (
    <DropdownMenuItem variant="destructive" onSelect={onSelect} disabled={disabled}>
      <Trash2 aria-hidden="true" />
      {t.tags.row.delete}
    </DropdownMenuItem>
  );
}

/** Kind deletion preserves tags without a kind; confirmation depends on whether any tags are affected. */
function DeleteKindDialog({ state, kind }: { state: TagKindsTab; kind: TagKind }) {
  const pending = state.deletePending;
  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) state.cancelDelete();
      }}
      title={t.tags.kinds.deleteDialog.title(kind.name)}
      description={
        kind.tagCount === 0
          ? t.tags.kinds.deleteDialog.unused
          : t.tags.kinds.deleteDialog.used(kind.tagCount)
      }
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending
            ? t.tags.kinds.deleteDialog.submitting
            : t.tags.kinds.deleteDialog.submit}
        </>
      }
      onConfirm={() => state.performDelete(kind)}
      pending={pending}
    >
      {state.deleteError !== null && <DialogError message={state.deleteError} />}
    </ConfirmDialog>
  );
}
