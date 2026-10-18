import { Minus } from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";

import {
  detachVideoTag,
  maxVideoTagsSelection,
  summarizeVideoTags,
  type VideoTagsSummary,
} from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { PopoverContent } from "../ui/shadcn/popover";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";
import { isTagNotFound, overLimitMessage } from "./selectionErrors";
import { buildRemoveOptions, removableSummary } from "./tagChoices";
import TagCommand, { type TagChoice } from "../ui/TagCommand";

const noSmartFolders: readonly number[] = [];

/** Candidates and removal cover the selected smart folders' current members as well as selectedIds. */
export default function RemoveTagPopover({
  open,
  onOpenChange,
  selectedIds,
  smartFolderIds = noSmartFolders,
  onRemoved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedIds: readonly number[];
  smartFolderIds?: readonly number[];
  onRemoved: (tagId: number) => void;
}) {
  const toast = useToast();
  const headingId = useId();
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [value, setValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<UiText | null>(null);
  const [loading, setLoading] = useState(false);
  const [fetchFailed, setFetchFailed] = useState(false);
  const [summary, setSummary] = useState<VideoTagsSummary | null>(null);

  // Guard the interval before the parent closes an over-limit popover.
  const overLimit = selectedIds.length > maxVideoTagsSelection;

  // Sequence summary requests because response order need not match selection changes.
  const summarySeq = useRef(0);
  const fetchSummary = useCallback(() => {
    // Empty selections are about to close; the summary endpoint requires at least one ID.
    if (selectedIds.length === 0 && smartFolderIds.length === 0) return;
    if (selectedIds.length > maxVideoTagsSelection) {
      // Invalidate earlier summaries so they cannot replace the over-limit state.
      summarySeq.current += 1;
      setLoading(false);
      setFetchFailed(false);
      setSummary(null);
      return;
    }
    const seq = (summarySeq.current += 1);
    setLoading(true);
    setFetchFailed(false);
    summarizeVideoTags(selectedIds, { smartFolderIds })
      .then((result) => {
        if (summarySeq.current !== seq) return;
        setSummary(removableSummary(result));
      })
      .catch(() => {
        if (summarySeq.current !== seq) return;
        setFetchFailed(true);
      })
      .finally(() => {
        if (summarySeq.current === seq) setLoading(false);
      });
  }, [selectedIds, smartFolderIds]);

  // Hide stale candidates while refreshing for a changed selection.
  const wasOpenRef = useRef(false);
  useEffect(() => {
    if (!open) {
      wasOpenRef.current = false;
      return;
    }
    const justOpened = !wasOpenRef.current;
    wasOpenRef.current = true;
    if (justOpened) {
      setValue("");
      setErrorMessage(null);
    }
    setSummary(null);
    fetchSummary();
  }, [open, fetchSummary]);

  // Focus the input after loading; onOpenAutoFocus runs only on initial mount.
  const ready =
    open && !overLimit && !loading && !fetchFailed && (summary?.items.length ?? 0) > 0;
  useEffect(() => {
    if (ready) inputRef.current?.focus();
  }, [ready]);

  const { options, exactOption } =
    summary === null
      ? { options: [], exactOption: null }
      : buildRemoveOptions(summary, value);

  function submit(option: TagChoice) {
    setErrorMessage(null);
    // Recheck current selection size immediately before submission.
    if (selectedIds.length > maxVideoTagsSelection) {
      setErrorMessage(overLimitMessage());
      return;
    }
    setSubmitting(true);
    const tagId = Number(option.id);
    const displayName = option.label;
    void detachVideoTag(selectedIds, tagId, { smartFolderIds })
      .then((result) => {
        toast(t.library.selection.removed(result.applied, displayName));
        setValue("");
        fetchSummary();
        onRemoved(tagId);
      })
      .catch((error: unknown) => {
        if (isTagNotFound(error)) {
          toast(t.library.selection.tagGone(displayName));
          fetchSummary();
          return;
        }
        setErrorMessage(t.library.selection.removeFailed(errorText(error)));
      })
      .finally(() => setSubmitting(false));
  }

  return (
    <PopoverContent
      side="top"
      align="start"
      aria-labelledby={headingId}
      onOpenAutoFocus={(event) => {
        if (overLimit || loading || fetchFailed || summary?.items.length === 0) {
          event.preventDefault();
        } else {
          inputRef.current?.focus();
        }
      }}
      // Consume Escape without clearing the parent list's selection.
      onEscapeKeyDown={(event) => {
        event.preventDefault();
        onOpenChange(false);
      }}
    >
      <h2 id={headingId} className="sr-only">
        {t.library.selection.removeTag}
      </h2>
      {overLimit && (
        <p role="alert" className="text-xs text-destructive">
          {overLimitMessage()}
        </p>
      )}
      {!overLimit && loading && (
        <p
          role="status"
          className="flex items-center gap-2 text-xs text-muted-foreground"
        >
          <Spinner aria-hidden="true" />
          {t.library.selection.loading}
        </p>
      )}
      {!overLimit && !loading && fetchFailed && (
        <div className="flex flex-col items-start gap-2">
          <p role="alert" className="text-xs text-destructive">
            {t.library.selection.summaryFailed}
          </p>
          <Button variant="ghost" size="sm" onClick={fetchSummary}>
            {t.common.retry}
          </Button>
        </div>
      )}
      {!overLimit &&
        !loading &&
        !fetchFailed &&
        summary !== null &&
        summary.items.length === 0 && (
          <p className="text-xs text-muted-foreground">
            {t.library.selection.nothingToRemove}
          </p>
        )}
      {!overLimit &&
        !loading &&
        !fetchFailed &&
        summary !== null &&
        summary.items.length > 0 && (
          <>
            <TagCommand
              label={t.library.selection.removeTag}
              value={value}
              onValueChange={setValue}
              choices={options}
              exactChoice={exactOption}
              onSelect={submit}
              icon={<Minus aria-hidden="true" />}
              busy={submitting}
              inputRef={inputRef}
            />
            {errorMessage !== null && (
              <p role="alert" className="text-xs text-destructive">
                {errorMessage}
              </p>
            )}
          </>
        )}
    </PopoverContent>
  );
}
