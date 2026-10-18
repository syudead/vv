import { X } from "lucide-react";
import { useEffect, useEffectEvent, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { addTagSynonym, refreshTags, removeTagSynonym, type Tag } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { isComposingKeyEvent, isComposingNativeKeyEvent } from "../lib/ime";
import { Badge } from "../ui/shadcn/badge";
import { Button } from "../ui/shadcn/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/shadcn/dialog";
import { Field, FieldError, FieldLabel } from "../ui/shadcn/field";
import { Input } from "../ui/shadcn/input";
import { Spinner } from "../ui/shadcn/spinner";
import { DialogError } from "./DialogError";
import { tagFieldError, useTagNameField, type TagFieldError } from "./tagNameField";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isMergeRequired(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_merge_required";
}

function mergeOwner(error: unknown): string | undefined {
  return error instanceof RequestFailed ? error.tagName : undefined;
}

interface MergeConfirm {
  name: string;
  sourceName: string;
  sourceId: number;
  /** Preserve pre-merge source state so callers can update filtered counts even for unloaded sources. */
  source: Tag;
  videoCount: number;
  hasSynonyms: boolean;
}

type FocusAfterRemoval = {
  target: { name: string } | "input";
  /** Wait for this chip to disappear; omit to restore focus immediately after failure. */
  removedName?: string;
};

export default function SynonymsDialog({
  tag,
  onClose,
  onTagUpdated,
  onSynonymRemoved,
  onStale,
}: {
  tag: Tag;
  onClose: () => void;
  onTagUpdated: (tag: Tag, removed?: Tag) => void;
  /** Remove from the caller's latest state, not captured props, so concurrent synonym removals compose. */
  onSynonymRemoved: (tagId: number, name: string) => void;
  onStale: () => void;
}) {
  const field = useTagNameField("", () => setAddError(null));
  const inputRef = useRef<HTMLInputElement | null>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const confirmMergeButton = useRef<HTMLButtonElement>(null);

  // Clear only the submitted spelling; preserve text entered while the request was pending.
  const fieldValueRef = useRef(field.value);
  fieldValueRef.current = field.value;

  // Read the latest props when composing responses, not the tag captured at request time.
  const tagRef = useRef(tag);
  tagRef.current = tag;

  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());
  const [addPending, setAddPending] = useState(false);
  const [addError, setAddError] = useState<TagFieldError | null>(null);

  const [confirm, setConfirm] = useState<MergeConfirm | null>(null);
  const [confirmPending, setConfirmPending] = useState(false);
  const [confirmError, setConfirmError] = useState<UiText | null>(null);

  const chipRefs = useRef(new Map<string, HTMLButtonElement>());
  const pendingFocusRef = useRef<FocusAfterRemoval | null>(null);

  // Dialog owns initial focus; move focus here only when switching confirmation views.
  const mountedRef = useRef(false);
  useEffect(() => {
    if (!mountedRef.current) {
      mountedRef.current = true;
      return;
    }
    if (confirm !== null) backRef.current?.focus();
    else inputRef.current?.focus();
  }, [confirm]);

  // Restore Merge or Back after confirmation failure; do not refocus just because the view changes.
  const restoreFocus = useEffectEvent(() => {
    (confirm !== null ? confirmMergeButton : backRef).current?.focus();
  });
  useEffect(() => {
    if (confirmError !== null) restoreFocus();
  }, [confirmError]);

  // After removal, focus next, previous, then input; failures restore the original remove button.
  useEffect(() => {
    const pending = pendingFocusRef.current;
    if (pending === null) return;
    if (pending.removedName !== undefined && tag.synonyms.includes(pending.removedName)) {
      return;
    }
    const target = pending.target;
    if (target !== "input" && removing.has(target.name)) return;
    pendingFocusRef.current = null;
    if (target === "input") {
      inputRef.current?.focus();
      return;
    }
    chipRefs.current.get(target.name)?.focus();
  }, [tag.synonyms, removing]);

  async function removeSynonym(name: string) {
    const order = tagRef.current.synonyms;
    const index = order.indexOf(name);
    const next = order[index + 1];
    const previous = index > 0 ? order[index - 1] : undefined;
    pendingFocusRef.current = {
      target:
        next !== undefined
          ? { name: next }
          : previous !== undefined
            ? { name: previous }
            : "input",
      removedName: name,
    };
    setRemoving((current) => new Set(current).add(name));
    setAddError(null);
    try {
      await removeTagSynonym(tag.id, name);
      // Let the parent's functional update compose concurrent removals instead of rebuilding from stale props.
      onSynonymRemoved(tag.id, name);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      pendingFocusRef.current = { target: { name } };
      setAddError({ kind: "other", message: errorText(failure) });
    } finally {
      setRemoving((current) => {
        const next = new Set(current);
        next.delete(name);
        return next;
      });
    }
  }

  /** Revalidate the name's owner before confirming its ID so another tab cannot redirect the merge. Retry plain registration at most once if that owner disappeared. */
  async function openConfirm(name: string, attempt = 0, ownerName?: string) {
    try {
      const list = await refreshTags();
      // Prefer the canonical tagName from the error, falling back to the submitted spelling.
      const found = list.find((item) => item.name === (ownerName ?? name));
      if (found === undefined) {
        if (attempt >= 1) {
          setAddError({ kind: "other", message: t.tags.synonymsDialog.addFailed });
          return;
        }
        // If the owner disappeared, retry plain registration and count this actual resend.
        await submitAdd(name, attempt + 1);
        return;
      }
      setConfirm({
        name,
        sourceName: found.name,
        sourceId: found.id,
        source: found,
        videoCount: found.videoCount,
        hasSynonyms: found.synonyms.length > 0,
      });
    } catch (failure) {
      setAddError({ kind: "other", message: errorText(failure) });
    }
  }

  async function submitAdd(spelling?: string, attempt = 0) {
    const name = spelling ?? field.trySpelling();
    if (name === null) return;
    // Preserve edits made while the request was pending.
    const submittedValue = fieldValueRef.current;
    setAddError(null);
    setAddPending(true);
    try {
      const updated = await addTagSynonym(tag.id, name);
      onTagUpdated(updated);
      if (fieldValueRef.current === submittedValue) field.setValue("");
      // Successful automatic retries must also leave confirmation mode.
      setConfirm(null);
      setAddPending(false);
    } catch (failure) {
      // Clear aria-busy before switching views so it cannot persist on the disappearing input.
      setAddPending(false);
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      if (isMergeRequired(failure)) {
        // Opening confirmation does not count as a resend.
        await openConfirm(name, attempt, mergeOwner(failure));
        return;
      }
      setAddError(tagFieldError(failure, { submitted: name, ownTagName: tag.name }));
    }
  }

  function backFromConfirm() {
    if (confirmPending) return;
    setConfirm(null);
    setConfirmError(null);
  }

  async function acceptMerge() {
    if (confirm === null) return;
    const submittedValue = fieldValueRef.current;
    setConfirmError(null);
    setConfirmPending(true);
    try {
      const updated = await addTagSynonym(tag.id, confirm.name, confirm.sourceId);
      // Tell the parent which source disappeared; background refresh may fail.
      onTagUpdated(updated, confirm.source);
      setConfirm(null);
      if (fieldValueRef.current === submittedValue) field.setValue("");
    } catch (failure) {
      if (isTagNotFound(failure)) {
        onStale();
        return;
      }
      if (isMergeRequired(failure)) {
        // Ownership changed after confirmation; refresh and ask to confirm the new owner.
        await openConfirm(confirm.name, 0, mergeOwner(failure));
        return;
      }
      setConfirmError(errorText(failure));
    } finally {
      setConfirmPending(false);
    }
  }

  /** Block closing while merging so late responses cannot update a removed dialog. */
  function handleClose() {
    if (confirmPending) return;
    onClose();
  }

  const reasonId = "synonym-add-reason";
  const errorId = "synonym-add-error";

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
    >
      <DialogContent
        aria-describedby={undefined}
        onEscapeKeyDown={(event) => {
          // Escape cancelling IME composition must not close the dialog.
          if (isComposingNativeKeyEvent(event)) event.preventDefault();
        }}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          inputRef.current?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{t.tags.synonymsDialog.title(tag.name)}</DialogTitle>
        </DialogHeader>
        {confirm === null ? (
          <div className="flex min-w-0 flex-col gap-3">
            {tag.synonyms.length > 0 && (
              <ul
                aria-label={t.tags.synonymsDialog.list}
                className="flex flex-wrap gap-1"
              >
                {tag.synonyms.map((name) => (
                  <li key={name} className="max-w-full min-w-0">
                    {}
                    <Badge variant="secondary" title={name} className="max-w-full pr-0.5">
                      <span className="min-w-0 truncate">{name}</span>
                      <Button
                        ref={(node) => {
                          if (node) chipRefs.current.set(name, node);
                          else chipRefs.current.delete(name);
                        }}
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t.tags.synonymsDialog.remove(name)}
                        aria-busy={removing.has(name) || undefined}
                        onClick={() => {
                          if (removing.has(name)) return;
                          void removeSynonym(name);
                        }}
                        className="size-5 rounded-sm"
                      >
                        <X aria-hidden="true" />
                      </Button>
                    </Badge>
                  </li>
                ))}
              </ul>
            )}

            <Field data-invalid={field.reason !== null || addError !== null || undefined}>
              <FieldLabel htmlFor="synonym-add-input" className="sr-only">
                {t.tags.synonymsDialog.add}
              </FieldLabel>
              <div className="flex items-center gap-2">
                <Input
                  id="synonym-add-input"
                  ref={inputRef}
                  value={field.value}
                  onChange={(event) => field.setValue(event.target.value)}
                  onPaste={field.onPaste}
                  onBeforeInput={field.onBeforeInput}
                  onKeyDown={(event) => {
                    if (isComposingKeyEvent(event)) return;
                    if (event.key === "Enter") {
                      event.preventDefault();
                      if (!addPending) void submitAdd();
                    }
                  }}
                  placeholder={t.tags.synonymsDialog.add}
                  aria-label={t.tags.synonymsDialog.add}
                  aria-describedby={
                    field.reason !== null
                      ? reasonId
                      : addError !== null
                        ? errorId
                        : undefined
                  }
                  aria-busy={addPending || undefined}
                  aria-invalid={
                    field.reason !== null || addError?.kind === "taken" || undefined
                  }
                  className="h-8"
                />
                <Button
                  size="sm"
                  onClick={() => {
                    if (!addPending) void submitAdd();
                  }}
                >
                  {t.tags.synonymsDialog.submit}
                </Button>
              </div>
              {field.reason !== null && (
                <FieldError id={reasonId} role={undefined}>
                  {field.reason}
                </FieldError>
              )}
              {field.reason === null &&
                addError !== null &&
                addError.kind === "taken" && (
                  <FieldError id={errorId} role={undefined}>
                    {addError.message}
                  </FieldError>
                )}
              {field.reason === null &&
                addError !== null &&
                addError.kind === "other" && (
                  <FieldError id={errorId} className="text-sm">
                    {addError.message}
                  </FieldError>
                )}
            </Field>
          </div>
        ) : (
          <>
            <p className="text-sm text-muted-foreground">
              {t.tags.synonymsDialog.mergeWarning(
                confirm.sourceName,
                confirm.videoCount,
                tag.name,
                confirm.hasSynonyms,
              )}
            </p>
            {confirmError !== null && <DialogError message={confirmError} />}
            <DialogFooter>
              <Button
                ref={backRef}
                variant="outline"
                size="sm"
                onClick={backFromConfirm}
                disabled={confirmPending}
              >
                {t.tags.synonymsDialog.back}
              </Button>
              <Button
                ref={confirmMergeButton}
                variant="destructive"
                size="sm"
                onClick={() => void acceptMerge()}
                disabled={confirmPending}
              >
                {confirmPending && <Spinner aria-hidden="true" />}
                {t.tags.synonymsDialog.merge}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
