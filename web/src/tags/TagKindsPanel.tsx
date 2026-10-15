import { Ellipsis, Pencil, Plus, Shapes, Trash2 } from "lucide-react";
import type { MouseEvent } from "react";
import { Link } from "react-router";

import type { TagKind } from "../api/tagKinds";
import { t } from "../i18n";
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

/** kindColumnCount は表の列の数（名前・タグの数・操作）。 */
const kindColumnCount = 3;

/**
 * TagKindsPanel はタグ管理画面の「Kinds」のタブの中身である
 * （specs/051-tag-kinds/ui-design.md「Kinds tab」）。種類を名前の自然順にすべて並べ、
 * 作成の行・その場の改名・削除の確認の窓を持つ。状態は `useTagKindsTab` が持つ。
 * チェックと選択バーは無い（種類にまとめての操作は無い）。
 */
export default function TagKindsPanel({
  state,
  hrefFor,
  onOpenKind,
}: {
  state: TagKindsTab;
  /** 種類の名前のリンクの先（その種類で絞った「Tags」のタブ）。 */
  hrefFor: (kind: TagKind) => string;
  /** 種類の名前を押したとき。履歴の規則に沿って URL を書き換える。 */
  onOpenKind: (kind: TagKind) => void;
}) {
  const { kinds, loadError, creating } = state;
  return (
    <>
      {kinds === undefined && loadError === null && (
        <LoadingState label={t.tags.kinds.loading} layout="table" count={4} />
      )}
      {kinds === undefined && loadError !== null && (
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

/**
 * KindRow は種類の 1 行である。名前はその種類で絞った「Tags」のタブへのリンクで、改名中は
 * 名前の列だけを入力に置き換える。操作はタグの行と同じ `RowIconButton` と `RowMoreMenu` で、
 * タッチの端末と `sm` 未満では「Actions」1 つにまとめる（CSS だけで出し分ける）。
 */
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
    // 新しいタブで開く押し方はブラウザに任せる。
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
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

/** DeleteItem は行のメニューの「Delete…」である。 */
function DeleteItem({ disabled, onSelect }: { disabled: boolean; onSelect: () => void }) {
  return (
    <DropdownMenuItem variant="destructive" onSelect={onSelect} disabled={disabled}>
      <Trash2 aria-hidden="true" />
      {t.tags.row.delete}
    </DropdownMenuItem>
  );
}

/**
 * DeleteKindDialog は種類の削除の確認の窓である。その種類のタグは種類なしで残るので、
 * 文言は `tagCount` が 0 かどうかで変える（ui-design.md「Words」）。
 */
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
      onConfirm={state.performDelete}
      pending={pending}
    >
      {state.deleteError !== null && <DialogError message={state.deleteError} />}
    </ConfirmDialog>
  );
}
