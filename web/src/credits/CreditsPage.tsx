import { Plus } from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";

import {
  type Credit,
  type CreditKind,
  createCredit,
  deleteCredit,
  isCreditGone,
  updateCredit,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { writeListSortPreferences } from "../preferences/tagListPreferences";
import TopBarPortal from "../shell/TopBarPortal";
import CreateTagRow from "../tags/CreateTagRow";
import type { TagListSort } from "../tags/tagListOrder";
import { useUrlCriteria } from "../tags/tagListUrl";
import { tagListView } from "../tags/tagListView";
import type { TagFieldError } from "../tags/tagNameField";
import {
  addRow,
  compareTagsForSort,
  insertionIndex,
  type ListRowsQuery,
  removeRows,
  replaceRow,
} from "../tags/tagPageRows";
import { AdminListToolbar } from "../tags/TagToolbar";
import { useAdminTableBand, useListScrollMargin } from "../tags/useAdminTableBand";
import { usePagedRows } from "../tags/useTagPage";
import { ROW_OVERSCAN, useVirtualTagRows } from "../tags/useVirtualTagRows";
import { virtualBodyRows } from "../tags/virtualBodyRows";
import { creditNameRules } from "../ui/CreditCommand";
import { AdminTablePage } from "../ui/patterns/admin-table-page";
import { DataTable } from "../ui/patterns/data-table";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { TableBody } from "../ui/shadcn/table";
import { useToast } from "../ui/Toast";
import type { HistoryMode } from "../videoList/listCriteria";
import CreditDetailsDialog from "./CreditDetailsDialog";
import { CreditListHead, CreditListStates, CreditListTail } from "./CreditListStates";
import CreditRow, { type CreditRowHandlers, creditRowIdAttribute } from "./CreditRow";
import {
  creditCountText,
  creditNameError,
  type CreditPageRows,
  fetchCreditPage,
  kindTexts,
  matchesCreditQuery,
  parseCreditListCriteria,
  readCreditSort,
  reparentRows,
  serializeCreditListCriteria,
  sortStorageKey,
} from "./creditRows";
import DeleteCreditDialog from "./DeleteCreditDialog";
import MergeCreditDialog from "./MergeCreditDialog";

const columnCount = 3;

/**
 * Share paging and virtualization with tags; reflect mutations in loaded rows without refetching.
 * Missing records close the dialog and reload from the first page.
 */
export default function CreditsPage({ kind }: { kind: CreditKind }) {
  const toast = useToast();
  const texts = kindTexts(kind);
  const [fetchPage] = useState(() => fetchCreditPage(kind));
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
  } = usePagedRows<Credit, ListRowsQuery>(
    fetchPage,
    { query: "", search: "", sort: "name" },
    () => scrollListToTop(),
  );
  const [preferredSort] = useState(() => readCreditSort(kind));
  const { criteria, apply: applyCriteria } = useUrlCriteria(
    (params) => parseCreditListCriteria(params, preferredSort),
    serializeCreditListCriteria,
  );
  const { query: search, sort } = criteria;

  const [creating, setCreating] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState<TagFieldError | null>(null);
  /** Preserve the initial renaming row even if it disappears from refreshed results. */
  const [renaming, setRenaming] = useState<Credit | null>(null);
  const renamingId = renaming?.id ?? null;
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);
  const [details, setDetails] = useState<Credit | null>(null);
  const [merging, setMerging] = useState<Credit | null>(null);
  const [deleting, setDeleting] = useState<Credit | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState<UiText | null>(null);

  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const createButtonRef = useRef<HTMLButtonElement | null>(null);
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

  function scrollListToTop() {
    scrollBoxToTop(listBoxRef.current);
  }

  function fallbackFocus() {
    const create = createButtonRef.current;
    if (create?.isConnected === true && !create.disabled) create.focus();
    releasePinIfFocusOutside();
  }

  // Normalize queries to ignore whitespace-only filters and avoid reloading equivalent queries.
  const normalizedQuery = foldForMatch(search).trim();
  const searchRef = useRef(search);
  useLayoutEffect(() => {
    searchRef.current = search;
  });
  useEffect(() => {
    void load({ query: normalizedQuery, search: searchRef.current, sort });
  }, [normalizedQuery, sort, load]);

  function shown(credit: Credit): boolean {
    const query = pageRef.current?.query;
    return query === undefined || matchesCreditQuery(credit, query);
  }

  const rows = useMemo(() => page?.rows ?? [], [page]);
  /** Keep a renaming row in sort order even when refreshed results exclude it. */
  const visibleRows = useMemo(() => {
    if (renaming === null || page === undefined) return rows;
    if (rows.some((credit) => credit.id === renaming.id)) return rows;
    const index = insertionIndex(rows, renaming, page.query.sort);
    return [...rows.slice(0, index), renaming, ...rows.slice(index)];
  }, [rows, page, renaming]);

  const {
    listRef,
    registerRefs,
    virtualizer,
    virtualItems,
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
    rowIdAttribute: creditRowIdAttribute,
  });

  // Prefetch when the final rendered row reaches the overscan boundary.
  const lastRendered = virtualItems.at(-1)?.index ?? -1;
  const nextCursor = page?.nextCursor;
  useEffect(() => {
    if (nextCursor === undefined) return;
    if (lastRendered >= visibleRows.length - ROW_OVERSCAN) loadMore();
  }, [
    lastRendered,
    visibleRows.length,
    nextCursor,
    more,
    firstPending,
    loadError,
    loadMore,
  ]);

  useListScrollMargin(
    listRef,
    page !== undefined && visibleRows.length > 0,
    setScrollMargin,
  );

  function changeSort(next: TagListSort) {
    applyCriteria({ sort: next }, "push");
    writeListSortPreferences(sortStorageKey(kind), { sort: next });
  }

  function commitQuery(next: string, mode: HistoryMode) {
    applyCriteria({ query: next }, mode);
  }

  function clearSearch() {
    applyCriteria({ query: "" }, "push");
    searchInputRef.current?.focus();
  }

  function openCreate() {
    if (renamePending) return;
    scrollListToTop();
    setCreating(true);
    setCreateError(null);
    setRenaming(null);
  }

  function replaceLoaded(updated: Credit, before?: Credit) {
    applyLocal((current) => {
      if (current === undefined) return current;
      const previous = current.rows.find((item) => item.id === updated.id) ?? before;
      return replaceRow(current, previous, updated, matchesCreditQuery);
    });
  }

  function removeLoaded(removed: Credit) {
    applyLocal((current) => {
      if (current === undefined) return current;
      const previous = current.rows.find((item) => item.id === removed.id) ?? removed;
      return removeRows(current, [previous], matchesCreditQuery);
    });
  }

  /** Mirror server reparenting after rename, delete or merge; undefined removes the parent. */
  function reparentLoaded(fromId: number, to?: Credit) {
    if (kind !== "organization") return;
    const parent = to === undefined ? undefined : { id: to.id, name: to.name };
    applyLocal((current) => {
      if (current === undefined) return current;
      const rows = reparentRows(current.rows, fromId, parent);
      return rows === current.rows ? current : { ...current, rows };
    });
  }

  /** Call after closing the dialog; reload and focus the row after the missing record. */
  async function gone(id: number) {
    const order = visibleRows;
    toast(texts.gone);
    await reload();
    focusAfterRemoval(order, id);
  }

  async function submitCreate(name: string) {
    setCreateError(null);
    setCreatePending(true);
    try {
      const created = await createCredit(kind, { name });
      applyLocal((current) =>
        current === undefined
          ? current
          : (addRow(current, created, matchesCreditQuery) as CreditPageRows),
      );
      setCreating(false);
      focusRow(created.id, "name");
    } catch (failure) {
      setCreateError(creditNameError(failure));
    } finally {
      setCreatePending(false);
    }
  }

  async function submitRename(credit: Credit, name: string) {
    setRenameError(null);
    setRenamePending(true);
    const order = visibleRows;
    try {
      const updated = await updateCredit(kind, credit.id, { name });
      replaceLoaded(updated, credit);
      reparentLoaded(updated.id, updated);
      setRenaming(null);
      if (shown(updated)) focusRow(credit.id, "rename");
      else focusAfterRemoval(order, credit.id);
    } catch (failure) {
      if (isCreditGone(failure)) {
        setRenaming(null);
        await gone(credit.id);
        return;
      }
      setRenameError(creditNameError(failure));
    } finally {
      setRenamePending(false);
    }
  }

  async function performDelete() {
    if (deleting === null) return;
    const target = deleting;
    const order = visibleRows;
    setDeleteError(null);
    setDeletePending(true);
    try {
      await deleteCredit(kind, target.id);
      removeLoaded(target);
      reparentLoaded(target.id);
      setDeleting(null);
      toast(t.credits.deleted(target.name));
      focusAfterRemoval(order, target.id);
    } catch (failure) {
      if (isCreditGone(failure)) {
        setDeleting(null);
        await gone(target.id);
        return;
      }
      setDeleteError(errorText(failure));
    } finally {
      setDeletePending(false);
    }
  }

  function cancelDelete() {
    if (deletePending || deleting === null) return;
    const target = deleting;
    setDeleting(null);
    setDeleteError(null);
    // Return focus explicitly to the row's menu trigger.
    focusRow(target.id, "menu");
  }

  function cancelMerge() {
    if (merging === null) return;
    const source = merging;
    setMerging(null);
    focusRow(source.id, "menu");
  }

  /**
   * Place the merge target in loaded sort order, or omit it outside the loaded range. Focus its
   * name, then the source's next row, previous row, or create button.
   */
  function performMerge(merged: Credit, target: Credit) {
    if (merging === null) return;
    const source = merging;
    const order = visibleRows;
    removeLoaded(source);
    replaceLoaded(merged, target);
    reparentLoaded(source.id, merged);
    setMerging(null);
    toast(t.credits.merged(source.name, merged.name));
    const current = pageRef.current;
    const placed =
      current?.boundary === undefined ||
      compareTagsForSort(merged, current.boundary, current.query.sort) <= 0;
    if (shown(merged) && placed) {
      focusRow(merged.id, "name");
      return;
    }
    focusAfterRemoval(order, source.id, new Set([merged.id]));
  }

  function staleMerge() {
    if (merging === null) return;
    const source = merging;
    setMerging(null);
    void gone(source.id);
  }

  function closeDetails() {
    if (details === null) return;
    const id = details.id;
    setDetails(null);
    focusRow(id, "synonyms");
  }

  function savedDetails(updated: Credit) {
    if (details === null) return;
    const order = visibleRows;
    replaceLoaded(updated, details);
    setDetails(null);
    if (shown(updated)) focusRow(updated.id, "synonyms");
    else focusAfterRemoval(order, updated.id);
  }

  function staleDetails() {
    if (details === null) return;
    const id = details.id;
    setDetails(null);
    void gone(id);
  }

  /** Stable wrappers read current handlers so memoized rows do not rerender on every operation. */
  const rowHandlersRef = useRef<CreditRowHandlers | null>(null);
  useLayoutEffect(() => {
    rowHandlersRef.current = {
      onStartRename: (target) => {
        if (createPending || renamePending) return;
        setCreating(false);
        setRenameError(null);
        setRenaming(target);
      },
      onCancelRename: (target) => {
        if (renamePending) return;
        setRenaming(null);
        setRenameError(null);
        focusRow(target.id, "rename");
      },
      onSubmitRename: (target, name) => void submitRename(target, name),
      onOpenDetails: (target) => setDetails(target),
      onOpenMerge: (target) => setMerging(target),
      onDelete: (target) => {
        setDeleteError(null);
        setDeleting(target);
      },
      onDraftChange: () => setRenameError(null),
    };
  });
  const rowHandlers = useMemo<CreditRowHandlers>(
    () => ({
      onStartRename: (credit) => rowHandlersRef.current?.onStartRename(credit),
      onCancelRename: (credit) => rowHandlersRef.current?.onCancelRename(credit),
      onSubmitRename: (credit, name) =>
        rowHandlersRef.current?.onSubmitRename(credit, name),
      onOpenDetails: (credit) => rowHandlersRef.current?.onOpenDetails(credit),
      onOpenMerge: (credit) => rowHandlersRef.current?.onOpenMerge(credit),
      onDelete: (credit) => rowHandlersRef.current?.onDelete(credit),
      onDraftChange: () => rowHandlersRef.current?.onDraftChange(),
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
  const countText = creditCountText(kind, page);

  const bodyRows = virtualBodyRows({
    items: virtualItems,
    scrollStart: virtualizer.options.scrollMargin,
    totalSize: virtualizer.getTotalSize(),
    columnCount,
    renderRow: (item) => {
      const credit = visibleRows[item.index]!;
      return (
        <CreditRow
          key={item.key}
          kind={kind}
          index={item.index}
          measureRef={virtualizer.measureElement}
          credit={credit}
          renaming={renamingId === credit.id}
          pending={renamingId === credit.id && renamePending}
          blockStart={createPending || (renamePending && renamingId !== credit.id)}
          error={renamingId === credit.id ? renameError : null}
          registerRefs={registerRefs}
          {...rowHandlers}
        />
      );
    },
  });

  const createButton = (
    <Button
      ref={createButtonRef}
      size="sm"
      onClick={openCreate}
      disabled={page === undefined || creating || createPending || renamePending}
    >
      <Plus aria-hidden="true" />
      {texts.newButton}
    </Button>
  );

  return (
    <AdminTablePage
      bandRef={bandRef}
      toolbar={
        <TopBarPortal>
          <AdminListToolbar
            query={search}
            onQueryCommit={commitQuery}
            searchRef={searchInputRef}
            searchDisabled={page !== undefined && page.totalAll === 0}
            searchLabel={texts.search}
            searchPlaceholder={texts.search}
            sort={sort}
            onSortChange={changeSort}
            sortDisabled={noTags}
            compactSortName={`${kind}-compact-sort`}
          />
        </TopBarPortal>
      }
      header={
        <PageHeader
          title={texts.title}
          count={
            <span role="status" aria-live="polite" className="whitespace-nowrap">
              {countText}
            </span>
          }
          actions={createButton}
        />
      }
      band={null}
    >
      <div ref={listBoxRef} className="flex flex-col gap-3">
        <CreditListStates
          kind={kind}
          loaded={page !== undefined}
          loadError={loadError}
          firstPending={firstPending}
          staleList={staleList}
          empty={empty}
          appliedSearch={appliedSearch}
          onRetry={() => void reload()}
          onCreate={openCreate}
          onClearSearch={clearSearch}
        />
        {showRows && (
          <DataTable label={texts.title} stickyHeaderTop={stuckBottom}>
            <CreditListHead headRef={headRef} />
            {creating && (
              <TableBody>
                <CreateTagRow
                  columnCount={columnCount}
                  pending={createPending}
                  error={createError}
                  rules={creditNameRules}
                  label={t.credits.create.label}
                  placeholder={t.credits.create.placeholder}
                  onCancel={() => {
                    if (createPending) return;
                    setCreating(false);
                    setCreateError(null);
                    setTimeout(() => createButtonRef.current?.focus(), 0);
                  }}
                  onSubmit={(name) => void submitCreate(name)}
                  onDraftChange={() => setCreateError(null)}
                />
              </TableBody>
            )}
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
        {showRows && (
          <CreditListTail
            kind={kind}
            tail={tail}
            onRetryMore={retryMore}
            onReload={() => void reload()}
          />
        )}
      </div>

      {details !== null && (
        <CreditDetailsDialog
          kind={kind}
          credit={details}
          onClose={closeDetails}
          onSaved={savedDetails}
          onStale={staleDetails}
        />
      )}
      {merging !== null && (
        <MergeCreditDialog
          kind={kind}
          source={merging}
          onClose={cancelMerge}
          onMerged={performMerge}
          onStale={staleMerge}
        />
      )}
      {deleting !== null && (
        <DeleteCreditDialog
          kind={kind}
          credit={deleting}
          pending={deletePending}
          error={deleteError}
          onClose={cancelDelete}
          onDelete={() => void performDelete()}
        />
      )}
    </AdminTablePage>
  );
}
