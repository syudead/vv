import { Plus } from "lucide-react";
import {
  type RefObject,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { RequestFailed } from "../api/client";
import {
  batchTags,
  confirmTag,
  createTag,
  deleteTag,
  maxTagBatch,
  rejectTag,
  renameTag,
  type Tag,
} from "../api/tags";
import { type TagKind, tagKindCountsChanged, tagKindGone } from "../api/tagKinds";
import { errorText, t, type UiText } from "../i18n";
import {
  readTagListPreferences,
  writeTagListPreferences,
} from "../preferences/tagListPreferences";
import { AdminTablePage } from "../ui/patterns/admin-table-page";
import { DataTable } from "../ui/patterns/data-table";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { TableBody } from "../ui/shadcn/table";
import { useToast } from "../ui/Toast";
import type { HistoryMode } from "../videoList/listCriteria";
import BulkTagDialog, { type BulkTagAction } from "./BulkTagDialog";
import CreateTagRow from "./CreateTagRow";
import DeleteTagDialog from "./DeleteTagDialog";
import MergeTagDialog from "./MergeTagDialog";
import RejectedNames from "./RejectedNames";
import RejectTagDialog from "./RejectTagDialog";
import TagKindsPanel from "./TagKindsPanel";
import SynonymsDialog from "./SynonymsDialog";
import type { TagListSort } from "./tagListOrder";
import {
  addTag,
  confirmTags,
  insertionIndex,
  matchesTagQuery,
  removeTags,
  replaceTag,
  setTagsKind,
  type TagPageRows,
} from "./tagPageRows";
import { tagFieldError, type TagFieldError } from "./tagNameField";
import {
  serializeTagListCriteria,
  type TagKindFilter,
  type TagListCriteria,
  type TagListTab,
  useTagListCriteria,
} from "./tagListUrl";
import TagRow from "./TagRow";
import {
  TagListChanged,
  TagLoadFailed,
  TagLoadingMore,
  TagLoadMoreFailed,
  TagStaleList,
} from "./TagListNotices";
import TagSelectionBar from "./TagSelectionBar";
import { tagCountText, tagListView } from "./tagListView";
import {
  countSelectable,
  keepLoaded,
  selectAllCheck,
  selectedKinds,
  withoutIds,
} from "./tagSelection";
import { useRejectedNames } from "./useRejectedNames";
import { useTagKindsTab } from "./useTagKindsTab";
import TagFilterChips, { type TagFilter, tagFilterOrder } from "./TagFilterChips";
import type { KindChoice } from "./TagSetKindMenu";
import { kindNameOf, useTagKindFilter } from "./useTagKindFilter";
import TagListEmptyState from "./TagListEmptyState";
import TagListHead from "./TagListHead";
import TagsTabs, { tagsPanelId, tagsTabId } from "./TagsTabs";
import { useSearchQuery, useTagPage } from "./useTagPage";
import { ROW_OVERSCAN, useVirtualTagRows } from "./useVirtualTagRows";
import { useAdminTableBand, useListScrollMargin } from "./useAdminTableBand";
import { virtualBodyRows } from "./virtualBodyRows";
import TopBarPortal from "../shell/TopBarPortal";
import TagToolbar from "./TagToolbar";

function isTagNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_found";
}

function isTagNotTentative(error: unknown): boolean {
  return error instanceof RequestFailed && error.code === "tag_not_tentative";
}

function isTagKindNotFound(error: unknown): boolean {
  return error instanceof RequestFailed && error.reason === "tag_kind_not_found";
}

const noFilters = { tentativeOnly: false, unusedOnly: false, kind: null } as const;

const filterOff: Record<TagFilter, Partial<TagListCriteria>> = {
  tentative: { tentativeOnly: false },
  unused: { unusedOnly: false },
  kind: { kind: null },
};

/** Active filters remain enabled even with zero tags so users can clear them. */
function filterButtonDisabled(
  page: TagPageRows | undefined,
  noTags: boolean,
  criteria: TagListCriteria,
): boolean {
  if (page === undefined) return true;
  const filtering =
    criteria.tentativeOnly || criteria.unusedOnly || criteria.kind !== null;
  return noTags && !filtering;
}

function appliedKindOf(page: TagPageRows | undefined): TagKindFilter {
  return page?.query.kind ?? null;
}

function nextChip(
  removed: TagFilter,
  active: Record<TagFilter, boolean>,
): TagFilter | undefined {
  const index = tagFilterOrder.indexOf(removed);
  const remaining = tagFilterOrder.filter(
    (filter) => filter !== removed && active[filter],
  );
  return (
    remaining.find((filter) => tagFilterOrder.indexOf(filter) > index) ?? remaining.at(-1)
  );
}

/** Stable callbacks keep memoized rows from rerendering. */
interface RowHandlers {
  onStartRename: (tag: Tag) => void;
  onCancelRename: (tag: Tag) => void;
  onSubmitRename: (tag: Tag, name: string) => void;
  onOpenSynonyms: (tag: Tag) => void;
  onOpenMerge: (tag: Tag) => void;
  onDelete: (tag: Tag) => void;
  onConfirm: (tag: Tag) => void;
  onReject: (tag: Tag) => void;
  onDraftChange: () => void;
  onSelect: (tag: Tag, selected: boolean) => void;
}

/** Read server-filtered pages independently of the shared tag cache; apply mutations locally and reload for missing targets. Persist criteria and tab in the URL. */
export default function TagsPage() {
  const toast = useToast();
  const rejected = useRejectedNames();
  const {
    page,
    loadError,
    firstPending,
    more,
    appliedSearch,
    pageRef,
    load,
    reload,
    loadMore,
    retryMore,
    applyLocal,
  } = useTagPage(() => scrollListToTop());
  /** Missing URL sort uses the device preference. */
  const [preferredSort] = useState<TagListSort>(() => readTagListPreferences().sort);
  const { criteria, apply: applyCriteria } = useTagListCriteria(preferredSort);
  const { query: search, tentativeOnly, unusedOnly, kind, sort, tab: urlTab } = criteria;
  /** Delay URL-driven tab changes while mutations are pending. */
  const [tab, setTab] = useState<TagListTab>(urlTab);
  const newKindButtonRef = useRef<HTMLButtonElement | null>(null);
  const kindsTab = useTagKindsTab({ toast, newKindRef: newKindButtonRef });
  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);

  /** Pin the original renaming row in sorted order even when refreshed results omit it. */
  const [renaming, setRenaming] = useState<Tag | null>(null);
  const renamingId = renaming?.id ?? null;
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);

  const [deletingTag, setDeletingTag] = useState<Tag | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);

  /** Freeze merge sources when opening the dialog. */
  const [merging, setMerging] = useState<{
    sources: readonly Tag[];
    fromSelection: boolean;
  } | null>(null);
  /** Keep the synonym dialog open even when mutations remove its row from current results. */
  const [synonymsTag, setSynonymsTag] = useState<Tag | null>(null);
  const synonymsTagId = synonymsTag?.id ?? null;
  /** Original row order supplies focus fallback if the source disappears. */
  const synonymsOrderRef = useRef<readonly Tag[]>([]);

  const [confirming, setConfirming] = useState<ReadonlySet<number>>(new Set());
  const confirmingRef = useRef(new Set<number>());

  const [rejectingTag, setRejectingTag] = useState<Tag | null>(null);
  const [rejectPending, setRejectPending] = useState(false);
  const [rejectError, setRejectError] = useState<UiText | null>(null);

  /** Selection contains loaded non-renaming rows only. */
  const [selected, setSelected] = useState<ReadonlySet<number>>(new Set());
  const [bulkPending, setBulkPending] = useState<"confirm" | "kind" | "dialog" | null>(
    null,
  );
  const [bulkDialog, setBulkDialog] = useState<{
    action: BulkTagAction;
    ids: readonly number[];
  } | null>(null);
  const [bulkError, setBulkError] = useState<UiText | null>(null);
  const selectAllRef = useRef<HTMLButtonElement | null>(null);
  const tabsId = useId();
  const barConfirmRef = useRef<HTMLButtonElement | null>(null);
  const barSetKindRef = useRef<HTMLButtonElement | null>(null);
  const barMergeRef = useRef<HTMLButtonElement | null>(null);
  const barRejectRef = useRef<HTMLButtonElement | null>(null);
  const barDeleteRef = useRef<HTMLButtonElement | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
  const filterButtonRef = useRef<HTMLButtonElement | null>(null);
  const chipRefs: Record<TagFilter, RefObject<HTMLButtonElement | null>> = {
    tentative: useRef<HTMLButtonElement | null>(null),
    unused: useRef<HTMLButtonElement | null>(null),
    kind: useRef<HTMLButtonElement | null>(null),
  };
  const listBoxRef = useRef<HTMLDivElement | null>(null);
  const {
    bandRef,
    headRef,
    stuckBottom,
    rowsTop,
    scrollMargin,
    setScrollMargin,
    scrollListToTop: scrollBoxToTop,
  } = useAdminTableBand();

  /** Read current filters when responses arrive, rather than the filters captured at submission. */
  const filtersRef = useRef<{
    tentativeOnly: boolean;
    unusedOnly: boolean;
    kind: TagKindFilter;
  }>({ tentativeOnly: false, unusedOnly: false, kind: null });

  /** Prefer Filter for empty filtered results, New tag otherwise, then the header checkbox if the selection bar hides creation. */
  function fallbackFocus() {
    const filtered =
      filtersRef.current.tentativeOnly ||
      filtersRef.current.unusedOnly ||
      filtersRef.current.kind !== null;
    const candidates = filtered
      ? [filterButtonRef.current, createButtonRef.current, selectAllRef.current]
      : [createButtonRef.current, selectAllRef.current, filterButtonRef.current];
    candidates
      .find((element) => element?.isConnected === true && !element.disabled)
      ?.focus();
    // Do not retain temporarily rendered focus candidates in tab order.
    releasePinIfFocusOutside();
  }

  function focusFilterButton() {
    const filter = filterButtonRef.current;
    if (filter?.isConnected === true && !filter.disabled) filter.focus();
    else createButtonRef.current?.focus();
  }

  function scrollListToTop() {
    scrollBoxToTop(listBoxRef.current);
  }

  function reloadList() {
    setSelected(new Set());
    void reload();
  }

  function changeSort(next: TagListSort) {
    applyCriteria({ sort: next }, "push");
    writeTagListPreferences({ sort: next });
  }

  function commitQuery(next: string, mode: HistoryMode) {
    applyCriteria({ query: next }, mode);
  }

  function changeTab(next: TagListTab): boolean {
    if (next === tab) return true;
    setSelected(new Set());
    setCreating(false);
    setCreateError(null);
    setRenaming(null);
    setRenameError(null);
    applyCriteria({ tab: next }, "push");
    return true;
  }

  // Normalize only to detect empty/equivalent queries; the server searches the full tag set.
  const { normalizedQuery, searchRef } = useSearchQuery(search);

  useEffect(() => {
    setSelected((current) => (current.size === 0 ? current : new Set()));
    void load({
      query: normalizedQuery,
      search: searchRef.current,
      tentativeOnly,
      unusedOnly,
      kind,
      sort,
    });
  }, [normalizedQuery, tentativeOnly, unusedOnly, kind, sort, load, searchRef]);

  useLayoutEffect(() => {
    filtersRef.current = { tentativeOnly, unusedOnly, kind };
  }, [tentativeOnly, unusedOnly, kind]);

  /** Remove unknown kind filters with history replacement while preserving other conditions. */
  const { kindName } = useTagKindFilter({
    kind,
    kinds: kindsTab.kinds,
    fresh: kindsTab.fresh,
    onGone: () => {
      applyCriteria({ kind: null }, "replace");
      toast(t.tags.kindFilter.gone);
    },
  });

  /** Refresh kind counts on entry and tag rows after kind edits; close transient editing/selection when leaving each tab. */
  function switchedTab(from: TagListTab, to: TagListTab) {
    if (to !== "tags") {
      setSelected((current) => (current.size === 0 ? current : new Set()));
      setCreating(false);
      setCreateError(null);
      setRenaming(null);
      setRenameError(null);
    }
    if (from === "kinds") {
      kindsTab.closeEdits();
      if (kindsTab.takeChanged()) void reload();
    }
    if (to === "kinds") kindsTab.reload();
  }
  const switchedTabRef = useRef(switchedTab);
  useLayoutEffect(() => {
    switchedTabRef.current = switchedTab;
  });

  // Pending mutations need their rows/dialogs for errors; if they fail, restore the URL to the still-visible tab.
  const editPending = createPending || renamePending || kindsTab.pending;
  const kindsEditing = kindsTab.editing;
  const heldTabRef = useRef(false);
  useLayoutEffect(() => {
    if (urlTab === tab) {
      heldTabRef.current = false;
      return;
    }
    if (editPending) {
      heldTabRef.current = true;
      return;
    }
    if (heldTabRef.current && (creating || renaming !== null || kindsEditing)) {
      heldTabRef.current = false;
      applyCriteria({ tab }, "replace");
      return;
    }
    heldTabRef.current = false;
    switchedTabRef.current(tab, urlTab);
    setTab(urlTab);
  }, [urlTab, tab, editPending, creating, renaming, kindsEditing, applyCriteria]);

  function shown(tag: Tag): boolean {
    const query = pageRef.current?.query;
    return query === undefined || matchesTagQuery(tag, query);
  }

  const rows = useMemo(() => page?.rows ?? [], [page]);

  /** Pin renaming rows to preserve drafts, but exclude them from result counts. */
  const visibleRows = useMemo(() => {
    if (renaming === null || page === undefined) return rows;
    if (rows.some((tag) => tag.id === renaming.id)) return rows;
    const index = insertionIndex(rows, renaming, page.query.sort);
    return [...rows.slice(0, index), renaming, ...rows.slice(index)];
  }, [rows, page, renaming]);

  useLayoutEffect(() => {
    setSelected((current) => keepLoaded(current, rows, renamingId));
  }, [rows, renamingId]);

  const selectableCount = countSelectable(rows, renamingId);
  /** Apply the API limit to loaded rows for Select all and selected rows for batch actions. */
  const selectAllOverLimit = rows.length > maxTagBatch;
  const selectionOverLimit = selected.size > maxTagBatch;
  const selection = useMemo(() => selectedKinds(rows, selected), [selected, rows]);
  const selectAllState = selectAllCheck(selected.size, selectableCount);

  const {
    listRef,
    registerRefs,
    visibleRowsRef,
    virtualizer,
    virtualItems,
    afterCommit,
    focusRow,
    focusAfterRemoval,
    releasePinIfFocusOutside,
    handleListFocus,
    handleListBlur,
    handleListKeyDown,
  } = useVirtualTagRows({
    visibleRows,
    renamingId,
    scrollMargin,
    rowsTop,
    fallbackFocus: () => fallbackFocus(),
  });

  // Load near the virtual tail, including when mutations remove every loaded row.
  const lastRendered = virtualItems.at(-1)?.index ?? -1;
  const nextCursor = page?.nextCursor;
  const onTagsTab = tab === "tags";
  useEffect(() => {
    if (nextCursor === undefined || !onTagsTab) return;
    if (lastRendered >= visibleRows.length - ROW_OVERSCAN) loadMore();
  }, [
    onTagsTab,
    lastRendered,
    visibleRows.length,
    nextCursor,
    more,
    firstPending,
    loadError,
    loadMore,
  ]);

  // Remeasure list offsets when creation rows or wrapped toolbars change document layout.
  const hasList = page !== undefined && visibleRows.length > 0 && onTagsTab;
  useListScrollMargin(listRef, hasList, setScrollMargin);

  /** Counts and empty states describe received rows, preserving prior state while new criteria load. */
  const applied = page?.query;
  const totalAll = page?.totalAll ?? 0;
  const countText = tagCountText(page);

  function openCreate() {
    // Guard keyboard/direct callers too: pending renames must block starting creation.
    if (renamePending) return;
    // Scroll to the top before inserting the creation input so it cannot appear out of view.
    scrollListToTop();
    setCreating(true);
    setCreateError(null);
    setRenaming(null);
  }

  function clearSearch() {
    applyCriteria({ query: "" }, "push");
    searchInputRef.current?.focus();
  }

  function changeTentativeOnly(value: boolean) {
    applyCriteria({ tentativeOnly: value }, "push");
  }

  function changeUnusedOnly(value: boolean) {
    applyCriteria({ unusedOnly: value }, "push");
  }

  function changeKind(next: TagKindFilter) {
    applyCriteria({ kind: next }, "push");
  }

  function removeFilterChip(which: TagFilter) {
    const next = nextChip(which, {
      tentative: tentativeOnly,
      unused: unusedOnly,
      kind: kind !== null,
    });
    applyCriteria(filterOff[which], "push");
    setTimeout(() => {
      if (next !== undefined) {
        chipRefs[next].current?.focus();
        return;
      }
      if (totalAll === 0) createButtonRef.current?.focus();
      else focusFilterButton();
    }, 0);
  }

  function clearFilters() {
    applyCriteria(noFilters, "push");
  }

  function showAllFromFilter() {
    applyCriteria(noFilters, "push");
    setTimeout(
      () => (totalAll === 0 ? createButtonRef.current?.focus() : focusFilterButton()),
      0,
    );
  }

  function showAllFromFilterSearch() {
    applyCriteria({ ...noFilters, query: "" }, "push");
    setTimeout(
      () => (totalAll === 0 ? createButtonRef : searchInputRef).current?.focus(),
      0,
    );
  }

  /** Use loaded or supplied prior state to update matching counts and sorted position. */
  function replaceRow(updated: Tag, before?: Tag) {
    applyLocal((current) => {
      if (current === undefined) return current;
      const previous = current.rows.find((item) => item.id === updated.id) ?? before;
      return replaceTag(current, previous, updated);
    });
  }

  function removeRows(removed: readonly Tag[]) {
    applyLocal((current) => {
      if (current === undefined) return current;
      const byId = new Map(current.rows.map((item) => [item.id, item]));
      return removeTags(
        current,
        removed.map((item) => byId.get(item.id) ?? item),
      );
    });
  }

  async function submitCreate(name: string) {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createTag(name);
      applyLocal((current) =>
        current === undefined ? current : addTag(current, created),
      );
      setCreating(false);
      rejected.reload();
      focusRow(created.id, "name");
    } catch (failure) {
      setCreateError(tagFieldError(failure, { submitted: name }));
    } finally {
      setCreatePending(false);
    }
  }

  async function submitRename(tag: Tag, name: string) {
    setRenameError(null);
    setRenamePending(true);
    const order = visibleRows;
    try {
      const updated = await renameTag(tag.id, name);
      // For unloaded renaming rows, use their original state or totals may falsely count a new match.
      replaceRow(updated, tag);
      setRenaming(null);
      rejected.reload();
      if (shown(updated)) focusRow(tag.id, "rename");
      else focusAfterRemoval(order, tag.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRenaming(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, tag.id);
        return;
      }
      setRenameError(tagFieldError(failure, { submitted: name, ownTagName: tag.name }));
    } finally {
      setRenamePending(false);
    }
  }

  async function performDelete() {
    if (deletingTag === null) return;
    const target = deletingTag;
    const order = visibleRows;
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteTag(target.id);
      removeRows([target]);
      tagKindCountsChanged();
      setDeletingTag(null);
      toast(t.tags.deleted(target.name));
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setDeletingTag(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, target.id);
        return;
      }
      setDeleteError(errorText(failure));
    } finally {
      setDeletePending(false);
    }
  }

  /** Block same-tick duplicate confirmation and focus the current or next surviving rename action. */
  async function submitConfirm(tag: Tag) {
    if (confirmingRef.current.has(tag.id)) return;
    confirmingRef.current.add(tag.id);
    setConfirming(new Set(confirmingRef.current));
    const order = visibleRows;
    try {
      const updated = await confirmTag(tag.id);
      replaceRow(updated);
      toast(t.tags.confirmed(updated.name));
      if (shown(updated)) focusRow(updated.id, "rename");
      else focusAfterRemoval(order, updated.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, tag.id);
        return;
      }
      // No inline error slot exists in the row; report through a toast.
      toast(errorText(failure));
    } finally {
      confirmingRef.current.delete(tag.id);
      setConfirming(new Set(confirmingRef.current));
    }
  }

  async function performReject() {
    if (rejectingTag === null) return;
    const target = rejectingTag;
    const order = visibleRows;
    setRejectError(null);
    setRejectPending(true);
    try {
      await rejectTag(target.id);
      removeRows([target]);
      tagKindCountsChanged();
      setRejectingTag(null);
      toast(t.tags.rejected(target.name));
      rejected.reload();
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isTagNotFound(failure)) {
        setRejectingTag(null);
        toast(t.tags.gone);
        await reload();
        focusAfterRemoval(order, target.id);
        return;
      }
      if (isTagNotTentative(failure)) {
        setRejectingTag(null);
        toast(t.tags.alreadyConfirmed);
        await reload();
        if (filtersRef.current.tentativeOnly) focusAfterRemoval(order, target.id);
        else focusRow(target.id, "menu");
        return;
      }
      setRejectError(errorText(failure));
    } finally {
      setRejectPending(false);
    }
  }

  function cancelReject() {
    if (rejectPending || rejectingTag === null) return;
    const target = rejectingTag;
    setRejectingTag(null);
    setRejectError(null);
    focusRow(target.id, "menu");
  }

  function cancelDelete() {
    if (deletePending || deletingTag === null) return;
    const target = deletingTag;
    setDeletingTag(null);
    setDeleteError(null);
    // The originating menu item no longer exists; restore its row's menu trigger explicitly.
    focusRow(target.id, "menu");
  }

  function cancelMerge() {
    if (merging === null) return;
    const { sources, fromSelection } = merging;
    setMerging(null);
    if (fromSelection) afterCommit(() => barMergeRef.current?.focus());
    else focusRow(sources[0]!.id, "menu");
  }

  /** Update sources/target locally, count only actual merges, and reload when any sources were missing. */
  function performMerge(
    merged: Tag,
    sourceIds: readonly number[],
    notFoundIds: readonly number[],
    target: Tag,
  ) {
    if (merging === null) return;
    const { sources, fromSelection } = merging;
    const order = visibleRows;
    const removed = new Set(sourceIds);
    removeRows(sources.filter((item) => removed.has(item.id)));
    replaceRow(merged, target);
    tagKindCountsChanged();
    if (fromSelection) setSelected(new Set());
    else setSelected((current) => withoutIds(current, removed));
    setMerging(null);
    const missing = new Set(notFoundIds);
    const done = sources.filter((item) => removed.has(item.id) && !missing.has(item.id));
    toast(
      done.length === 1
        ? t.tags.merged(done[0]!.name, merged.name)
        : t.tags.selection.merged(done.length, merged.name),
    );
    afterStale(notFoundIds);
    // Confirmation removes both source and target from tentative-only results; focus from the first source's old position.
    // 。
    if (shown(merged)) {
      focusRow(merged.id, "name");
      return;
    }
    const first = order.find((item) => removed.has(item.id));
    if (first === undefined) afterCommit(fallbackFocus);
    else focusAfterRemoval(order, first.id, new Set([...removed, merged.id]));
  }

  function staleMerge() {
    if (merging === null) return;
    const { sources, fromSelection } = merging;
    const order = visibleRows;
    setMerging(null);
    toast(fromSelection ? t.tags.selection.stale : t.tags.gone);
    void reload().then(() => {
      if (fromSelection) {
        const merge = barMergeRef.current;
        if (merge?.isConnected === true && !merge.disabled) merge.focus();
        else focusSelectAllOr(fallbackFocus);
        return;
      }
      focusAfterRemoval(order, sources[0]!.id);
    });
  }

  function openMergeSelected() {
    if (bulkPending !== null || selected.size === 0) return;
    setMerging({
      sources: visibleRows.filter((tag) => selected.has(tag.id)),
      fromSelection: true,
    });
  }

  /** Restore the synonym trigger, or use the original row order to find the next action if the row disappeared. */
  function cancelSynonyms() {
    if (synonymsTagId === null) return;
    const id = synonymsTagId;
    setSynonymsTag(null);
    if (rows.some((tag) => tag.id === id)) {
      focusRow(id, "synonyms");
      return;
    }
    const loadedIds = new Set(rows.map((tag) => tag.id));
    const order = synonymsOrderRef.current;
    const gone = new Set(
      order.filter((tag) => !loadedIds.has(tag.id)).map((tag) => tag.id),
    );
    focusAfterRemoval(order, id, gone);
  }

  const synonymsTagRef = useRef<Tag | null>(synonymsTag);
  useLayoutEffect(() => {
    synonymsTagRef.current = synonymsTag;
  });

  /** Reevaluate current filters after synonym edits/merges; subtract matching unloaded sources using their pre-merge state. */
  function updateSynonymsTag(updated: Tag, removed?: Tag) {
    const opened = synonymsTagRef.current;
    applyLocal((current) => {
      if (current === undefined) return current;
      let next = current;
      if (removed !== undefined) {
        const source = next.rows.find((item) => item.id === removed.id) ?? removed;
        next = removeTags(next, [source]);
      }
      const previous =
        next.rows.find((item) => item.id === updated.id) ??
        (opened?.id === updated.id ? opened : undefined);
      return replaceTag(next, previous, updated);
    });
    setSynonymsTag((current) => (current?.id === updated.id ? updated : current));
    //
    if (removed !== undefined) tagKindCountsChanged();
    rejected.reload();
  }

  /** Remove from latest state with a functional update so concurrent removals cannot overwrite each other. */
  function removeSynonymFromTag(tagId: number, name: string) {
    const without = (item: Tag): Tag => ({
      ...item,
      synonyms: item.synonyms.filter((synonym) => synonym !== name),
    });
    applyLocal((current) => {
      const previous = current?.rows.find((item) => item.id === tagId);
      if (current === undefined || previous === undefined) return current;
      return replaceTag(current, previous, without(previous));
    });
    setSynonymsTag((current) => (current?.id === tagId ? without(current) : current));
  }

  function staleSynonyms() {
    if (synonymsTagId === null) return;
    const id = synonymsTagId;
    const order = visibleRows;
    setSynonymsTag(null);
    toast(t.tags.gone);
    void reload().then(() => focusAfterRemoval(order, id));
  }

  function toggleSelectAll() {
    if (selectAllState === true) {
      setSelected(new Set());
      return;
    }
    setSelected(
      new Set(rows.filter((tag) => tag.id !== renamingId).map((tag) => tag.id)),
    );
  }

  function clearSelection() {
    setSelected(new Set());
    afterCommit(() => {
      const header = selectAllRef.current;
      if (header !== null && !header.disabled) header.focus();
      else fallbackFocus();
    });
  }

  function afterStale(notFoundIds: readonly number[]) {
    if (notFoundIds.length === 0) return;
    toast(t.tags.selection.stale);
    void reload();
  }

  /** Deselect only rows actually confirmed; already-confirmed or failed rows remain selected. */
  async function submitBulkConfirm() {
    if (bulkPending !== null || selected.size === 0) return;
    const ids = [...selected];
    setBulkPending("confirm");
    try {
      const result = await batchTags("confirm", ids);
      const applied = new Set(result.appliedIds);
      applyLocal((current) =>
        current === undefined ? current : confirmTags(current, applied),
      );
      setSelected((current) => withoutIds(current, applied));
      toast(t.tags.selection.confirmed(applied.size, result.notApplicableIds.length));
      afterStale(result.notFoundIds);
      afterCommit(focusAfterBulkConfirm);
    } catch (failure) {
      toast(errorText(failure));
    } finally {
      setBulkPending(null);
    }
  }

  function focusAfterBulkConfirm() {
    focusAfterBulkAction(filtersRef.current.tentativeOnly, barConfirmRef);
  }

  /** Choose an enabled action or header checkbox; empty filtered results return focus to Filter. */
  function focusAfterBulkAction(
    filteredOut: boolean,
    first: RefObject<HTMLButtonElement | null>,
  ) {
    if (filteredOut && visibleRowsRef.current.length === 0) {
      focusFilterButton();
      return;
    }
    for (const candidate of [first.current, barMergeRef.current]) {
      if (candidate?.isConnected === true && !candidate.disabled) {
        candidate.focus();
        return;
      }
    }
    focusSelectAllOr(fallbackFocus);
  }

  /** Deselect only changed rows; stale kinds refresh their list without changing the selection. */
  async function submitSetKind(choice: KindChoice) {
    if (bulkPending !== null || selected.size === 0) return;
    const ids = [...selected];
    const kindRef = choice === "none" ? null : { id: choice.id, name: choice.name };
    setBulkPending("kind");
    try {
      const result = await batchTags(
        kindRef === null ? "clearKind" : { action: "setKind", kindId: kindRef.id },
        ids,
      );
      const applied = new Set(result.appliedIds);
      applyLocal((current) =>
        current === undefined ? current : setTagsKind(current, applied, kindRef),
      );
      setSelected((current) => withoutIds(current, applied));
      const skipped = result.notApplicableIds.length;
      toast(
        kindRef === null
          ? t.tags.selection.kindCleared(applied.size, skipped)
          : t.tags.selection.kindSet(kindRef.name, applied.size, skipped),
      );
      if (applied.size > 0) tagKindCountsChanged();
      afterStale(result.notFoundIds);
      afterCommit(() =>
        focusAfterBulkAction(filtersRef.current.kind !== null, barSetKindRef),
      );
    } catch (failure) {
      if (isTagKindNotFound(failure)) {
        tagKindGone();
        toast(t.tags.selection.kindGone);
        return;
      }
      toast(errorText(failure));
    } finally {
      setBulkPending(null);
    }
  }

  function focusSelectAllOr(otherwise: () => void) {
    const header = selectAllRef.current;
    if (header?.isConnected === true && !header.disabled) header.focus();
    else otherwise();
  }

  function openBulkDialog(action: BulkTagAction) {
    if (bulkPending !== null || selected.size === 0) return;
    setBulkError(null);
    setBulkDialog({ action, ids: [...selected] });
  }

  /** Keep inapplicable rows selected and preserve both dialog and selection on failure. */
  async function performBulk() {
    if (bulkDialog === null || bulkPending !== null) return;
    const { action, ids } = bulkDialog;
    const order = visibleRows;
    setBulkError(null);
    setBulkPending("dialog");
    try {
      const result = await batchTags(action, ids);
      const applied = new Set(result.appliedIds);
      removeRows(rows.filter((item) => applied.has(item.id)));
      tagKindCountsChanged();
      setSelected((current) => withoutIds(current, applied));
      setBulkDialog(null);
      const skipped = result.notApplicableIds.length;
      toast(
        action === "reject"
          ? t.tags.selection.rejected(applied.size, skipped)
          : t.tags.selection.deleted(applied.size, skipped),
      );
      if (action === "reject") rejected.reload();
      afterStale(result.notFoundIds);
      const fallback = () => {
        if (filtersRef.current.tentativeOnly) focusFilterButton();
        else focusSelectAllOr(fallbackFocus);
      };
      const first = order.find((tag) => applied.has(tag.id));
      if (first === undefined) afterCommit(fallback);
      else focusAfterRemoval(order, first.id, applied, fallback);
    } catch (failure) {
      setBulkError(errorText(failure));
    } finally {
      setBulkPending(null);
    }
  }

  function cancelBulk() {
    if (bulkPending !== null || bulkDialog === null) return;
    const opener = bulkDialog.action === "reject" ? barRejectRef : barDeleteRef;
    setBulkDialog(null);
    setBulkError(null);
    afterCommit(() => (opener.current ?? barMergeRef.current)?.focus());
  }

  /** Stable wrapper callbacks read current handlers so React.memo rows avoid unrelated rerenders. */
  const rowHandlersRef = useRef<RowHandlers | null>(null);
  useLayoutEffect(() => {
    rowHandlersRef.current = {
      onStartRename: (target) => {
        // Guard direct callers as well as disabled controls against overlapping row edits.
        if (createPending || renamePending) return;
        setCreating(false);
        setRenameError(null);
        setRenaming(target);
        setSelected((current) => withoutIds(current, new Set([target.id])));
      },
      onCancelRename: (target) => {
        if (renamePending) return;
        setRenaming(null);
        setRenameError(null);
        focusRow(target.id, "rename");
      },
      onSubmitRename: (target, name) => void submitRename(target, name),
      onOpenSynonyms: (target) => {
        synonymsOrderRef.current = visibleRows;
        setSynonymsTag(target);
      },
      onOpenMerge: (target) => setMerging({ sources: [target], fromSelection: false }),
      onDelete: (target) => {
        setDeleteError(null);
        setDeletingTag(target);
      },
      onConfirm: (target) => void submitConfirm(target),
      onReject: (target) => {
        setRejectError(null);
        setRejectingTag(target);
      },
      onDraftChange: () => setRenameError(null),
      onSelect: (target, next) => {
        if (target.id === renamingId) return;
        setSelected((current) => {
          if (current.has(target.id) === next) return current;
          const updated = new Set(current);
          if (next) updated.add(target.id);
          else updated.delete(target.id);
          return updated;
        });
      },
    };
  });
  const rowHandlers = useMemo<RowHandlers>(
    () => ({
      onStartRename: (tag) => rowHandlersRef.current?.onStartRename(tag),
      onCancelRename: (tag) => rowHandlersRef.current?.onCancelRename(tag),
      onSubmitRename: (tag, name) => rowHandlersRef.current?.onSubmitRename(tag, name),
      onOpenSynonyms: (tag) => rowHandlersRef.current?.onOpenSynonyms(tag),
      onOpenMerge: (tag) => rowHandlersRef.current?.onOpenMerge(tag),
      onDelete: (tag) => rowHandlersRef.current?.onDelete(tag),
      onConfirm: (tag) => rowHandlersRef.current?.onConfirm(tag),
      onReject: (tag) => rowHandlersRef.current?.onReject(tag),
      onDraftChange: () => rowHandlersRef.current?.onDraftChange(),
      onSelect: (tag, next) => rowHandlersRef.current?.onSelect(tag, next),
    }),
    [],
  );

  const { empty, showRows, staleList, tail, noTags } = tagListView({
    page,
    loadError,
    more,
    visibleCount: visibleRows.length,
    creating,
  });
  const appliedTentative = applied?.tentativeOnly ?? false;
  const appliedKind = appliedKindOf(page);
  const sortDisabled = noTags;

  const selecting = onTagsTab && selected.size > 0;
  // Pending mutations block both pointer and keyboard tab changes.
  const tabsLocked = editPending;

  /** Preserve other conditions when linking to the kind-filtered Tags tab. */
  function kindHref(kind: TagKind): string {
    const next = { ...criteria, tab: "tags" as const, kind: String(kind.id) };
    return `?${serializeTagListCriteria(next).toString()}`;
  }

  function openKind(kind: TagKind) {
    applyCriteria({ tab: "tags", kind: String(kind.id) }, "push");
  }

  function headerAction() {
    if (tab === "kinds") {
      return (
        <Button
          ref={newKindButtonRef}
          size="sm"
          onClick={kindsTab.openCreate}
          disabled={
            kindsTab.kinds === undefined ||
            kindsTab.loadError !== null ||
            kindsTab.creating ||
            editPending
          }
        >
          <Plus aria-hidden="true" />
          {t.tags.kinds.newKind}
        </Button>
      );
    }
    if (!onTagsTab) return undefined;
    return (
      <Button
        ref={createButtonRef}
        size="sm"
        onClick={openCreate}
        disabled={page === undefined || creating || createPending || renamePending}
      >
        <Plus aria-hidden="true" />
        {t.tags.newTag}
      </Button>
    );
  }

  function otherPanel() {
    const value = tab === "kinds" ? "kinds" : "rejected";
    return (
      <div
        id={tagsPanelId(tabsId, value)}
        role="tabpanel"
        aria-labelledby={tagsTabId(tabsId, value)}
        className="flex flex-col gap-3"
      >
        {value === "kinds" ? (
          <TagKindsPanel state={kindsTab} hrefFor={kindHref} onOpenKind={openKind} />
        ) : (
          <RejectedNames
            page={rejected.page}
            error={rejected.error}
            onRetry={rejected.reload}
            morePending={rejected.morePending}
            moreError={rejected.moreError}
            onLoadMore={rejected.loadMore}
            resetKey={rejected.epoch}
            onForget={rejected.forget}
            onFocusFallback={() =>
              document.getElementById(tagsTabId(tabsId, "rejected"))?.focus()
            }
          />
        )}
      </div>
    );
  }

  const bodyRows = virtualBodyRows({
    items: virtualItems,
    scrollStart: virtualizer.options.scrollMargin,
    totalSize: virtualizer.getTotalSize(),
    columnCount: tagColumnCount,
    renderRow: (item) => {
      const tag = visibleRows[item.index]!;
      return (
        <TagRow
          key={item.key}
          index={item.index}
          measureRef={virtualizer.measureElement}
          tag={tag}
          renaming={renamingId === tag.id}
          pending={renamingId === tag.id && renamePending}
          blockStart={createPending || (renamePending && renamingId !== tag.id)}
          error={renamingId === tag.id ? renameError : null}
          registerRefs={registerRefs}
          confirming={confirming.has(tag.id)}
          selected={selected.has(tag.id)}
          {...rowHandlers}
        />
      );
    },
  });

  return (
    <AdminTablePage
      bandRef={bandRef}
      toolbar={
        onTagsTab && (
          <TopBarPortal>
            <TagToolbar
              query={search}
              onQueryCommit={commitQuery}
              searchRef={searchInputRef}
              searchDisabled={page !== undefined && page.totalAll === 0}
              tentativeOnly={tentativeOnly}
              onTentativeOnlyChange={changeTentativeOnly}
              unusedOnly={unusedOnly}
              onUnusedOnlyChange={changeUnusedOnly}
              kind={kind}
              kinds={kindsTab.kinds}
              onKindChange={changeKind}
              onClearFilters={clearFilters}
              filterDisabled={filterButtonDisabled(page, noTags, criteria)}
              filterRef={filterButtonRef}
              sort={sort}
              onSortChange={changeSort}
              sortDisabled={sortDisabled}
            />
          </TopBarPortal>
        )
      }
      header={
        selecting ? (
          <TagSelectionBar
            count={selected.size}
            hasTentative={selection.tentative}
            hasConfirmed={selection.confirmed}
            overLimit={selectionOverLimit}
            busy={bulkPending !== null}
            confirming={bulkPending === "confirm"}
            onConfirm={() => void submitBulkConfirm()}
            onReject={() => openBulkDialog("reject")}
            onDelete={() => openBulkDialog("delete")}
            onMerge={openMergeSelected}
            onClear={clearSelection}
            kinds={kindsTab.kinds}
            sharedKind={selection.kind}
            settingKind={bulkPending === "kind"}
            onSetKind={(choice) => void submitSetKind(choice)}
            setKindRef={barSetKindRef}
            confirmRef={barConfirmRef}
            mergeRef={barMergeRef}
            rejectRef={barRejectRef}
            deleteRef={barDeleteRef}
          />
        ) : (
          <PageHeader
            title={t.tags.title}
            count={
              onTagsTab ? (
                <span role="status" aria-live="polite" className="whitespace-nowrap">
                  {countText}
                </span>
              ) : undefined
            }
            actions={headerAction()}
          />
        )
      }
      band={
        <>
          <TagsTabs
            idPrefix={tabsId}
            tab={tab}
            onChange={changeTab}
            tagsCount={page?.totalAll}
            rejectedCount={rejected.page?.total}
            kindsCount={kindsTab.kinds?.length}
            locked={tabsLocked}
          />
          {onTagsTab && (
            <TagFilterChips
              tentativeOnly={tentativeOnly}
              unusedOnly={unusedOnly}
              kind={kind}
              kindName={kindName}
              onRemove={removeFilterChip}
              chipRefs={chipRefs}
            />
          )}
        </>
      }
    >
      {onTagsTab ? (
        <div
          ref={listBoxRef}
          id={tagsPanelId(tabsId, "tags")}
          role="tabpanel"
          aria-labelledby={tagsTabId(tabsId, "tags")}
          className="flex flex-col gap-3"
        >
          {staleList && loadError !== null && (
            <TagStaleList
              reason={loadError}
              pending={firstPending}
              onRetry={() => void reload()}
            />
          )}

          {page === undefined && loadError === null && (
            <LoadingState label={t.tags.loading} layout="table" count={6} />
          )}

          {page === undefined && loadError !== null && (
            <TagLoadFailed pending={firstPending} onRetry={() => void reload()} />
          )}

          {empty !== null && (
            <TagListEmptyState
              empty={empty}
              appliedSearch={appliedSearch}
              appliedTentative={appliedTentative}
              appliedKind={appliedKind}
              kindName={kindNameOf(kindsTab.kinds, appliedKind)}
              onCreate={openCreate}
              onShowAll={showAllFromFilter}
              onShowAllAndSearch={showAllFromFilterSearch}
              onClearSearch={clearSearch}
            />
          )}

          {showRows && (
            <DataTable label={t.tags.title} stickyHeaderTop={stuckBottom}>
              <TagListHead
                headRef={headRef}
                selectAllRef={selectAllRef}
                checked={selectAllState}
                selectableCount={selectableCount}
                overLimit={selectAllOverLimit}
                onToggle={toggleSelectAll}
              />
              {creating && (
                <TableBody>
                  <CreateTagRow
                    columnCount={tagColumnCount}
                    pending={createPending}
                    error={createError}
                    onCancel={() => {
                      // Guard closing here too; pending row responses still need their draft/error destination.
                      if (createPending) return;
                      setCreating(false);
                      setCreateError(null);
                      // Return to New tag, even under filters, once closing the creation row reenables it.
                      setTimeout(() => createButtonRef.current?.focus(), 0);
                    }}
                    onSubmit={(name) => void submitCreate(name)}
                    onDraftChange={() => setCreateError(null)}
                  />
                </TableBody>
              )}
              {/* Spacer rows preserve document height; tbody coordinates focus and Tab across virtual rows. */}
              <TableBody
                ref={listRef}
                aria-busy={tail.kind === "loading" ? true : undefined}
                onFocus={handleListFocus}
                onBlur={handleListBlur}
                onKeyDown={handleListKeyDown}
              >
                {bodyRows}
              </TableBody>
            </DataTable>
          )}
          {showRows && tail.kind === "loading" && <TagLoadingMore />}
          {showRows && tail.kind === "failed" && (
            <TagLoadMoreFailed reason={tail.error} onRetry={retryMore} />
          )}
          {showRows && tail.kind === "inconsistent" && (
            <TagListChanged onReload={reloadList} />
          )}
        </div>
      ) : (
        otherPanel()
      )}

      {bulkDialog !== null && (
        <BulkTagDialog
          action={bulkDialog.action}
          ids={bulkDialog.ids}
          pending={bulkPending === "dialog"}
          error={bulkError}
          onClose={cancelBulk}
          onSubmit={() => void performBulk()}
        />
      )}

      {deletingTag !== null && (
        <DeleteTagDialog
          tag={deletingTag}
          pending={deletePending}
          error={deleteError}
          onClose={cancelDelete}
          onDelete={() => void performDelete()}
        />
      )}

      {rejectingTag !== null && (
        <RejectTagDialog
          tag={rejectingTag}
          pending={rejectPending}
          error={rejectError}
          onClose={cancelReject}
          onReject={() => void performReject()}
        />
      )}

      {merging !== null && page !== undefined && (
        <MergeTagDialog
          sources={merging.sources}
          fromSelection={merging.fromSelection}
          onClose={cancelMerge}
          onMerged={performMerge}
          onStale={staleMerge}
        />
      )}

      {synonymsTag !== null && (
        <SynonymsDialog
          tag={synonymsTag}
          onClose={cancelSynonyms}
          onTagUpdated={updateSynonymsTag}
          onSynonymRemoved={removeSynonymFromTag}
          onStale={staleSynonyms}
        />
      )}
    </AdminTablePage>
  );
}

const tagColumnCount = 4;
