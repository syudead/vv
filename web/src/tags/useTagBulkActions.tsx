import {
  type MutableRefObject,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { RequestFailed } from "../api/client";
import { tagKindCountsChanged, tagKindGone, type TagKind } from "../api/tagKinds";
import { batchTags, maxTagBatch, type Tag, type TagBatchResponse } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { useEscapeClearsSelection } from "../library/useEscapeClearsSelection";
import { appendUniqueTags } from "../recordList/listRows";
import { useToast } from "../ui/Toast";
import BulkTagDialog, { type BulkTagAction } from "./BulkTagDialog";
import MergeTagDialog from "./MergeTagDialog";
import TagSelectionBar, { type TagBulkPending } from "./TagSelectionBar";
import type { KindChoice } from "./TagSetKindMenu";
import {
  confirmTags,
  removeTags,
  replaceTag,
  setTagsKind,
  type TagPageRows,
} from "./tagPageRows";
import { selectedKinds } from "./tagSelection";
import { readTagPages, TagListMoved } from "./useTagPage";
import type { TagSelection } from "./useTagSelection";

type Track = <T>(write: Promise<T>) => Promise<T>;
type ApplyLocal = (
  update: (current: TagPageRows | undefined) => TagPageRows | undefined,
) => void;

/** Every matching card, read from the shown list's cursor; the selection takes them all. */
function withRest(page: TagPageRows, rest: readonly Tag[]): TagPageRows {
  return {
    ...page,
    rows: appendUniqueTags(page.rows, rest),
    nextCursor: undefined,
    boundary: undefined,
  };
}

function kindRefOf(choice: KindChoice) {
  return choice === "none"
    ? null
    : { id: choice.id, name: choice.name, cardFirst: choice.cardFirst };
}

function isKindGone(failure: unknown): boolean {
  return failure instanceof RequestFailed && failure.reason === "tag_kind_not_found";
}

/**
 * The selection bar's actions on the selected tag cards and their dialogs. Each result is applied to
 * the shown cards; ids the server no longer has (merged or deleted elsewhere) reload the list.
 */
export function useTagBulkActions({
  page,
  pageRef,
  searchRef,
  selection,
  kinds,
  applyLocal,
  refresh,
  track,
  afterWrite,
}: {
  page: TagPageRows | undefined;
  pageRef: MutableRefObject<TagPageRows | undefined>;
  /** The raw search the shown pages were read with. */
  searchRef: MutableRefObject<string>;
  selection: TagSelection;
  kinds: readonly TagKind[] | undefined;
  applyLocal: ApplyLocal;
  /** Read the list again from the top with the same criteria. */
  refresh: () => void;
  track: Track;
  /** After a write the server applied: the counts read with the list are read again. */
  afterWrite: () => void;
}): { bar: ReactNode; dialogs: ReactNode } {
  const toast = useToast();
  const { selected, drop, replace, clear } = selection;
  const [pending, setPending] = useState<TagBulkPending>(null);
  const [dialog, setDialog] = useState<{ action: BulkTagAction; ids: number[] } | null>(
    null,
  );
  const [dialogError, setDialogError] = useState<UiText | null>(null);
  const [merging, setMerging] = useState<readonly Tag[] | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  useEffect(() => () => abortRef.current?.abort(), []);

  const rows = useMemo(() => page?.rows ?? [], [page]);
  const summary = useMemo(() => selectedKinds(rows, selected), [rows, selected]);
  const guardedClear = useCallback(() => {
    if (pending === null) clear();
  }, [pending, clear]);
  useEscapeClearsSelection(selected.size > 0, guardedClear);

  /** The limit stops a batch before any request, as the bar's disabled actions do. */
  const ready = () =>
    pending === null && selected.size > 0 && selected.size <= maxTagBatch;

  function afterBatch(result: TagBatchResponse): ReadonlySet<number> {
    const applied = new Set(result.appliedIds);
    drop(applied);
    if (applied.size > 0) afterWrite();
    if (result.notFoundIds.length > 0) {
      toast(t.tags.selection.stale);
      refresh();
    }
    return applied;
  }

  async function confirm() {
    if (!ready()) return;
    setPending("confirm");
    try {
      const result = await track(batchTags("confirm", [...selected]));
      const applied = new Set(result.appliedIds);
      applyLocal((current) => current && confirmTags(current, applied));
      afterBatch(result);
      toast(t.tags.selection.confirmed(applied.size, result.notApplicableIds.length));
    } catch (failure) {
      toast(errorText(failure));
    } finally {
      setPending(null);
    }
  }

  async function setKind(choice: KindChoice) {
    if (!ready()) return;
    const ref = kindRefOf(choice);
    setPending("kind");
    try {
      const result = await track(
        batchTags(ref === null ? "clearKind" : { action: "setKind", kindId: ref.id }, [
          ...selected,
        ]),
      );
      const applied = new Set(result.appliedIds);
      applyLocal((current) => current && setTagsKind(current, applied, ref));
      afterBatch(result);
      if (applied.size > 0) tagKindCountsChanged();
      const skipped = result.notApplicableIds.length;
      toast(
        ref === null
          ? t.tags.selection.kindCleared(applied.size, skipped)
          : t.tags.selection.kindSet(ref.name, applied.size, skipped),
      );
    } catch (failure) {
      if (isKindGone(failure)) {
        tagKindGone();
        toast(t.tags.selection.kindGone);
        return;
      }
      toast(errorText(failure));
    } finally {
      setPending(null);
    }
  }

  function openDialog(action: BulkTagAction) {
    if (!ready()) return;
    setDialogError(null);
    setDialog({ action, ids: [...selected] });
  }

  async function runDialog() {
    if (dialog === null || pending !== null) return;
    const { action, ids } = dialog;
    setDialogError(null);
    setPending("dialog");
    try {
      const result = await track(batchTags(action, ids));
      const applied = new Set(result.appliedIds);
      const removed = rows.filter((tag) => applied.has(tag.id));
      applyLocal((current) => current && removeTags(current, removed));
      afterBatch(result);
      if (applied.size > 0) tagKindCountsChanged();
      setDialog(null);
      const skipped = result.notApplicableIds.length;
      toast(
        action === "reject"
          ? t.tags.selection.rejected(applied.size, skipped)
          : t.tags.selection.deleted(applied.size, skipped),
      );
    } catch (failure) {
      setDialogError(errorText(failure));
    } finally {
      setPending(null);
    }
  }

  function openMerge() {
    if (!ready()) return;
    setMerging(rows.filter((tag) => selected.has(tag.id)));
  }

  function merged(
    tag: Tag,
    sourceIds: readonly number[],
    notFoundIds: readonly number[],
    target: Tag,
  ) {
    const sources = merging ?? [];
    const gone = new Set(sourceIds);
    applyLocal((current) => {
      if (current === undefined) return current;
      const without = removeTags(
        current,
        sources.filter((item) => gone.has(item.id)),
      );
      const before = without.rows.find((item) => item.id === target.id) ?? target;
      return replaceTag(without, before, tag);
    });
    setMerging(null);
    clear();
    afterWrite();
    tagKindCountsChanged();
    const missing = new Set(notFoundIds);
    const done = sources.filter((item) => gone.has(item.id) && !missing.has(item.id));
    toast(
      done.length === 1
        ? t.tags.merged(done[0]!.name, tag.name)
        : t.tags.selection.merged(done.length, tag.name),
    );
    if (notFoundIds.length > 0) {
      toast(t.tags.selection.stale);
      refresh();
    }
  }

  function staleMerge() {
    setMerging(null);
    toast(t.tags.selection.stale);
    refresh();
  }

  /** Load the rest of the matching cards from the shown cursor, then select every card. */
  async function selectMatching() {
    const shown = pageRef.current;
    if (pending !== null || shown === undefined) return;
    if (shown.nextCursor === undefined) {
      replace(shown.rows.map((tag) => tag.id));
      return;
    }
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setPending("selectAll");
    try {
      const rest = await readTagPages(
        shown.query,
        searchRef.current,
        { cursor: shown.nextCursor, wanted: Infinity, expectAll: shown.totalAll },
        controller.signal,
      );
      if (controller.signal.aborted || pageRef.current?.query !== shown.query) return;
      const full = withRest(shown, rest.rows);
      applyLocal((current) =>
        current?.query === shown.query ? withRest(current, rest.rows) : current,
      );
      replace(full.rows.map((tag) => tag.id));
    } catch (failure) {
      if (controller.signal.aborted) return;
      if (failure instanceof TagListMoved) {
        toast(t.tags.list.changed);
        refresh();
        return;
      }
      toast(t.tags.selection.matchingFailed(errorText(failure)));
    } finally {
      if (abortRef.current === controller) setPending(null);
    }
  }

  const matching = page?.total ?? 0;
  const bar =
    selected.size === 0 ? null : (
      <TagSelectionBar
        count={selected.size}
        hasTentative={summary.tentative}
        hasConfirmed={summary.confirmed}
        pending={pending}
        selectAll={{
          loaded: rows.length,
          matching,
          matchingReason:
            matching > maxTagBatch
              ? t.tags.selection.matchingOverLimit(maxTagBatch)
              : undefined,
          pending: pending === "selectAll",
          onLoaded: () => replace(rows.map((tag) => tag.id)),
          onMatching: () => void selectMatching(),
        }}
        onConfirm={() => void confirm()}
        onReject={() => openDialog("reject")}
        onDelete={() => openDialog("delete")}
        onMerge={openMerge}
        onClear={guardedClear}
        kinds={kinds}
        sharedKind={summary.kind}
        onSetKind={(choice) => void setKind(choice)}
      />
    );

  const dialogs = (
    <>
      {dialog !== null && (
        <BulkTagDialog
          action={dialog.action}
          ids={dialog.ids}
          pending={pending === "dialog"}
          error={dialogError}
          onClose={() => {
            if (pending === null) setDialog(null);
          }}
          onSubmit={() => void runDialog()}
        />
      )}
      {merging !== null && (
        <MergeTagDialog
          sources={merging}
          fromSelection
          track={track}
          onClose={() => setMerging(null)}
          onMerged={merged}
          onStale={staleMerge}
        />
      )}
    </>
  );

  return { bar, dialogs };
}
