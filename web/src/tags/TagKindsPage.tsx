import { Pencil, Shapes, Trash2 } from "lucide-react";
import { type RefObject, useId, useRef } from "react";
import { Link } from "react-router";

import type { TagKind } from "../api/tagKinds";
import { t } from "../i18n";
import { isComposingKeyEvent } from "../lib/ime";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageSection } from "../ui/patterns/page-section";
import { SettingsPage } from "../ui/patterns/settings-page";
import { DropdownMenuItem, DropdownMenuSeparator } from "../ui/shadcn/dropdown-menu";
import { Field, FieldError } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Label } from "../ui/shadcn/label";
import { Spinner } from "../ui/shadcn/spinner";
import { Switch } from "../ui/shadcn/switch";
import { useToast } from "../ui/Toast";
import { DialogError } from "../ui/DialogError";
import { KindMark } from "../ui/KindMark";
import { useTagNameField } from "../ui/nameField";
import RowNameInput from "../ui/RowNameInput";
import { RowMoreMenu } from "../ui/RowMoreMenu";
import { TagsSubpageHeader } from "./TagsSubpageHeader";
import { type TagKindEdits, useTagKindEdits } from "./useTagKindEdits";

/** `/tags/kinds`: create, rename and delete kinds; deleting one leaves its tags without a kind. */
export default function TagKindsPage() {
  const toast = useToast();
  const newKindRef = useRef<HTMLInputElement | null>(null);
  const state = useTagKindEdits({ toast, newKindRef });
  const { loadError } = state;
  // Hide a stale list after a failed read because the delete confirmation relies on its counts.
  const kinds = loadError === null ? state.kinds : undefined;
  return (
    <SettingsPage
      header={
        <TagsSubpageHeader
          title={t.tags.kinds.title}
          description={t.tags.kinds.description}
        />
      }
    >
      <PageSection title={t.tags.kinds.section}>
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
        {kinds?.length === 0 && (
          <EmptyState
            icon={<Shapes aria-hidden="true" />}
            title={t.tags.kinds.empty.title}
            description={t.tags.kinds.empty.description}
          />
        )}
        {kinds?.map((kind) => (
          <KindRow key={kind.id} kind={kind} state={state} />
        ))}
        {kinds !== undefined && <NewKindField state={state} inputRef={newKindRef} />}
      </PageSection>
      {state.deleting !== null && (
        <DeleteKindDialog state={state} kind={state.deleting} />
      )}
    </SettingsPage>
  );
}

function KindRow({ kind, state }: { kind: TagKind; state: TagKindEdits }) {
  const switchId = useId();
  const renaming = state.renamingId === kind.id;
  const blockStart = state.createPending || (state.renamePending && !renaming);
  return (
    <div data-kind-id={kind.id} className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <KindMark kind={kind.id} variant="dot" />
        {renaming ? (
          <div className="min-w-0 flex-1">
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
          </div>
        ) : (
          <>
            <span title={kind.name} className="min-w-0 font-medium wrap-anywhere">
              {kind.name}
            </span>
            <Link
              to={`/tags?kind=${String(kind.id)}`}
              aria-label={t.tags.kinds.openCount(kind.tagCount, kind.name)}
              className="shrink-0 text-xs whitespace-nowrap text-primary tabular-nums underline-offset-4 hover:underline"
            >
              {t.tags.kinds.tagCount(kind.tagCount)}
            </Link>
          </>
        )}
      </div>
      <div className="flex items-center justify-end gap-2 max-sm:w-full">
        <Switch
          id={switchId}
          checked={kind.cardFirst}
          disabled={state.cardFirstPendingId === kind.id}
          onCheckedChange={(next) => state.setCardFirst(kind, next)}
          aria-describedby={`${switchId}-kind`}
        />
        <Label htmlFor={switchId} className="text-xs font-normal text-muted-foreground">
          {t.tags.kinds.firstOnCards}
        </Label>
        <span id={`${switchId}-kind`} className="sr-only">
          {kind.name}
        </span>
        <RowMoreMenu
          buttonRef={(node) => state.registerMenu(kind.id, node)}
          label={t.tags.kinds.more(kind.name)}
          disabled={renaming || blockStart}
        >
          <DropdownMenuItem onSelect={() => state.startRename(kind)}>
            <Pencil aria-hidden="true" />
            {t.tags.row.rename}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => state.openDelete(kind)}>
            <Trash2 aria-hidden="true" />
            {t.tags.row.delete}
          </DropdownMenuItem>
        </RowMoreMenu>
      </div>
    </div>
  );
}

/** The section's last row; Enter creates and the field stays for the next name. */
function NewKindField({
  state,
  inputRef,
}: {
  state: TagKindEdits;
  inputRef: RefObject<HTMLInputElement | null>;
}) {
  const field = useTagNameField("", state.clearCreateError);
  const reasonId = useId();
  const { createError: error, createPending: pending } = state;
  const reason = field.reason;

  async function submit() {
    if (pending) return;
    const spelling = field.trySpelling();
    if (spelling === null) return;
    if (await state.submitCreate(spelling)) field.setValue("");
  }

  const message = reason ?? error?.message ?? null;
  return (
    <Field data-invalid={message !== null || undefined}>
      <div className="flex items-center gap-2">
        <Input
          ref={inputRef}
          value={field.value}
          onChange={(event) => field.setValue(event.target.value)}
          onPaste={field.onPaste}
          onBeforeInput={field.onBeforeInput}
          onKeyDown={(event) => {
            if (isComposingKeyEvent(event) || event.key !== "Enter") return;
            event.preventDefault();
            void submit();
          }}
          placeholder={t.tags.kinds.newKind}
          aria-label={t.tags.kinds.create.label}
          aria-describedby={message !== null ? reasonId : undefined}
          aria-busy={pending || undefined}
          aria-invalid={reason !== null || error?.kind === "taken" || undefined}
          className="h-8 flex-1"
        />
        {pending && <Spinner aria-label={t.tags.kinds.create.submitting} />}
      </div>
      {message !== null && (
        <FieldError
          id={reasonId}
          role={reason === null && error?.kind === "other" ? "alert" : undefined}
        >
          {message}
        </FieldError>
      )}
    </Field>
  );
}

/** The confirmation depends on whether any tags have the kind; they stay, without one. */
function DeleteKindDialog({ state, kind }: { state: TagKindEdits; kind: TagKind }) {
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
