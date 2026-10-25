import {
  type ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useLocation } from "react-router";

import {
  type LibraryGroup,
  type LibraryItem,
  type SmartFolderSummary,
  type Video,
  type VideoSort,
} from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import {
  groupKey,
  itemKey,
  itemVideo,
  smartFolderKey,
  videoKey,
} from "../api/libraryItems";
import { useVideos } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { GroupChangedContext } from "../groups/groupChanges";
import { t } from "../i18n";
import {
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import { useScanControls } from "../shell/ScanProvider";
import { SmartFolderCardsProvider } from "../smartFolders/cardValues";
import { SmartFolderCard, SmartFolderRow } from "../smartFolders/SmartFolderCard";
import { useWatchScanRefresh } from "../shell/useWatchScanRefresh";
import TopBarPortal from "../shell/TopBarPortal";
import { DataTable } from "../ui/patterns/data-table";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { TableBody, TableHead, TableHeader, TableRow } from "../ui/shadcn/table";
import {
  criteriaKey,
  type HistoryMode,
  type ListCriteriaChange,
  newSeed,
} from "../videoList/listCriteria";
import { DraggableCard } from "../videoList/cardDrag";
import { Grid } from "../videoList/Grid";
import { useUndoToast } from "../videoList/undoToast";
import { useVideoDrops } from "../videoList/useVideoDrops";
import { useScrollTopOnChange } from "../videoList/useScrollTopOnChange";
import { GuestEmpty, LoadFailed, LoadMoreFailed, NoMatches } from "../videoList/states";
import { useListActions } from "../videoList/useListActions";
import { useListCriteria } from "../videoList/useListCriteria";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import SaveSmartFolderButton from "../smartFolders/SaveSmartFolderButton";
import type { LibraryFilter } from "../smartFolders/smartRule";
import CardTagRow from "./CardTagRow";
import EmptyLibrary from "./EmptyLibrary";
import { GroupCard, GroupRow } from "./GroupCard";
import {
  DraggableGroupRow,
  DraggableVideoRow,
  LibraryDragLayer,
  useLibraryMoves,
} from "./LibraryDrag";
import LibraryToolbar from "./LibraryToolbar";
import { MarqueeBox } from "./MarqueeBox";
import {
  emptySelection,
  favoriteTargets,
  selectionTargets,
  type LibrarySelection,
  reconcileGroups,
  selectedCount,
  selectedGroupOf,
  isGroupSelected,
  setGroup,
  setSmartFolder,
  setVideo,
} from "./selection";
import SelectionBar from "./SelectionBar";
import { TagRowMeasureProvider } from "./TagRowMeasure";
import { useEscapeClearsSelection } from "./useEscapeClearsSelection";
import { useFolderTagRow } from "./useCardTagRows";
import {
  libraryPageParams,
  useLibraryFilters,
  useLibraryFilterValues,
} from "./useLibraryFilters";
import { useMarqueeSelection } from "./useMarqueeSelection";
import { useRangeSelection } from "./useRangeSelection";
import { hasConditions as hasTagConditions } from "./tagCriteria";

const skeletonCount = 12;

function libraryGroupItem(group: LibraryGroup): LibraryItem {
  return { kind: "group", group };
}

function itemGroup(item: LibraryItem): LibraryGroup | undefined {
  return item.kind === "group" ? item.group : undefined;
}

function withSmartFolderCards(content: ReactNode) {
  return <SmartFolderCardsProvider>{content}</SmartFolderCardsProvider>;
}

/** Box selection is the owner's, over loaded grid cards. */
function drawsBoxes(owner: boolean, view: ViewPreferences["view"], loading: boolean) {
  return owner && view === "grid" && !loading;
}

function libraryConditioned(filter: LibraryFilter): boolean {
  return hasTagConditions(
    filter.criteria,
    filter.tagIds,
    filter.credits,
    filter.conditions,
  );
}

function saveSmartFolderAction(
  owner: boolean,
  conditioned: boolean,
  filter: LibraryFilter,
  sort: VideoSort,
): ReactNode {
  return owner && conditioned ? (
    <SaveSmartFolderButton filter={filter} sort={sort} />
  ) : undefined;
}

/** Retired parameters (versions, addedWithin, minDuration, maxDuration) are dropped on the next write. */
const libraryParams = libraryPageParams;

export default function LibraryPage() {
  const location = useLocation();
  // Hide owner-data actions from guests.
  //
  const owner = useAudience() === "owner";
  const [preferences, setPreferences] = useState(readViewPreferences);
  // URL owns criteria; only sort is persisted on the device.
  // Pass library-specific filters through useListCriteria's extension so every URL update preserves them.
  const { criteria, apply } = useListCriteria(preferences.sort, libraryParams);
  const { query, favorite, sort } = criteria;
  const { tagIds, credits, conditions } = useLibraryFilterValues(owner);
  const { zoom, view } = preferences;
  const searchField = useRef<HTMLInputElement | null>(null);
  const { resetPreview, cardProps: preview } = usePreviewCoordination();

  const savePreferences = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  const update = useCallback(
    (next: ListCriteriaChange, mode: HistoryMode = "push") => {
      resetPreview();
      apply(next, mode);
    },
    [apply, resetPreview],
  );

  const changeSort = useCallback(
    (next: VideoSort) => {
      update((current) => ({
        ...current,
        sort: next,
        seed: next === "random" ? newSeed(current.seed) : undefined,
      }));
      savePreferences({ ...preferences, sort: next });
    },
    [preferences, savePreferences, update],
  );
  const { shuffle, commitQuery } = useListActions(update);
  // Read current tags through a ref to keep bulk-edit callbacks stable across unrelated renders.
  const latestTagIds = useRef(tagIds);
  latestTagIds.current = tagIds;
  const { listRef: list, capture: captureAnchor } = useZoomAnchor(zoom);

  const changeZoom = useCallback(
    (next: Zoom) => {
      captureAnchor();
      resetPreview();
      savePreferences({ ...preferences, zoom: next });
    },
    [captureAnchor, preferences, resetPreview, savePreferences],
  );

  const [restored] = useState(() =>
    takeListSnapshot({ ...criteria, tags: tagIds, ...credits, ...conditions }),
  );
  const {
    items,
    total,
    cursor,
    hasMore,
    loading,
    loadingMore,
    error,
    missingTagIds,
    loadMore,
    retryLoadMore,
    reload,
    refreshInPlace,
    staleGroups,
  } = useVideos(
    { ...criteria, tag: tagIds, ...credits, ...conditions },
    restored,
    "library",
  );
  const filters = useLibraryFilters({
    owner,
    criteria,
    apply,
    tagIds,
    credits,
    conditions,
    missingTagIds,
    restored: restored !== undefined,
    resetPreview,
    searchField,
  });
  const { pressTag, clearAll } = filters;

  // Keep newly nonmatching watched rows in place until the next reload.
  useEffect(() => {
    resetPreview();
  }, [favorite, items, query, resetPreview, sort, view, zoom]);

  // Selection retains video IDs and group identity; video mutations expand groups, while favorites target groups themselves.
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  const selectedIds = selection.ids;
  // Removing any member also removes the group's selected identity.
  const displayed = useRef<readonly LibraryItem[]>(items);
  const { select: selectRange, point: pointRange } = useRangeSelection(
    displayed,
    setSelection,
  );
  // Toggles read the rendered selection, so a Shift toggle can extend a range like a check.
  const selectionNow = useRef(selection);
  selectionNow.current = selection;
  const changeSelection = useCallback(
    (id: number, selected: boolean) =>
      selectRange(videoKey(id), selected, (current) => setVideo(current, id, selected)),
    [selectRange],
  );
  const toggleSelection = useCallback(
    (id: number) => changeSelection(id, !selectionNow.current.ids.has(id)),
    [changeSelection],
  );
  const changeGroupSelection = useCallback(
    (group: LibraryGroup, selected: boolean) =>
      selectRange(groupKey(group.id), selected, (current) =>
        setGroup(current, selectedGroupOf(group), selected),
      ),
    [selectRange],
  );
  const toggleGroupSelection = useCallback(
    (group: LibraryGroup) =>
      changeGroupSelection(group, !isGroupSelected(selectionNow.current, group.id)),
    [changeGroupSelection],
  );
  const changeSmartFolderSelection = useCallback(
    (id: number, selected: boolean) =>
      selectRange(smartFolderKey(id), selected, (current) =>
        setSmartFolder(current, id, selected),
      ),
    [selectRange],
  );
  const toggleSmartFolderSelection = useCallback(
    (id: number) =>
      changeSmartFolderSelection(id, !selectionNow.current.smartFolders.has(id)),
    [changeSmartFolderSelection],
  );
  const clearSelection = useCallback(() => {
    pointRange(null);
    setSelection(emptySelection);
  }, [pointRange]);

  const moves = useLibraryMoves(items, refreshInPlace, clearSelection);
  const undo = useUndoToast(refreshInPlace);
  const drops = useVideoDrops({
    items: moves.visible,
    keyOf: itemKey,
    videoOf: itemVideo,
    groupOf: itemGroup,
    from: null,
    groupItem: libraryGroupItem,
    reread: refreshInPlace,
    onAccepted: clearSelection,
    undo,
  });
  const shown = drops.shown;
  useLayoutEffect(() => {
    displayed.current = shown;
  });

  // Removing an active filter tag in bulk clears selection and reloads matches, but retains the valid filter itself.
  const onTagRemoved = useCallback(
    (tagId: number) => {
      if (!latestTagIds.current.includes(tagId)) return;
      clearSelection();
      clearListSnapshot();
      reload();
    },
    [clearSelection, reload],
  );

  // Deleted groups and smart folders leave the list, and a dissolved group's videos join it.
  const onCollectionsDeleted = useCallback(() => {
    clearSelection();
    refreshInPlace();
  }, [clearSelection, refreshInPlace]);

  // Reconcile selected group identity after refresh without removing selected video IDs.
  useEffect(() => {
    setSelection((current) => reconcileGroups(current, items));
  }, [items]);

  // Clear selection whenever conditions change, even when some videos match both views.
  const conditionsSignature = [criteriaKey(criteria), filters.signature].join("\u0000");
  const knownConditionsSignature = useRef(conditionsSignature);
  useEffect(() => {
    if (knownConditionsSignature.current === conditionsSignature) return;
    knownConditionsSignature.current = conditionsSignature;
    clearSelection();
  }, [clearSelection, conditionsSignature]);
  useScrollTopOnChange(conditionsSignature);

  useEscapeClearsSelection(selectedCount(selection) > 0, clearSelection);

  const pendingScroll = useRef(restored?.scrollY);
  useEffect(() => {
    const previous = history.scrollRestoration;
    history.scrollRestoration = "manual";
    return () => {
      history.scrollRestoration = previous;
    };
  }, []);
  useLayoutEffect(() => {
    const top = pendingScroll.current;
    if (top === undefined || items.length === 0) return;
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
    // Restore scroll again before the next paint because image/row heights may settle a frame later.
    requestAnimationFrame(() => window.scrollTo({ top, behavior: "auto" }));
  }, [items.length]);

  // Bundling removes nonrepresentatives; reload while retaining grid scroll position.
  // The selection bar disappears; focus the first visible refreshed item without scrolling.
  const focusFirstItem = useRef<"idle" | "requested" | "loading">("idle");
  const onBundled = useCallback(() => {
    clearSelection();
    pendingScroll.current = window.scrollY;
    focusFirstItem.current = "requested";
    reload();
  }, [clearSelection, reload]);
  useEffect(() => {
    if (focusFirstItem.current === "idle") return;
    if (loading) {
      focusFirstItem.current = "loading";
      return;
    }
    if (focusFirstItem.current !== "loading") return;
    focusFirstItem.current = "idle";
    const links = Array.from(
      list.current?.querySelectorAll<HTMLElement>("a[href]") ?? [],
    );
    const target =
      links.find((link) => link.getBoundingClientRect().top >= 0) ?? links[0];
    target?.focus({ preventScroll: true });
  }, [items, list, loading]);

  const listUrl = `${location.pathname}${location.search}`;

  const scan = useScanControls();
  const { refresh: refreshScan } = scan;
  const knownScanId = useRef(restored?.scanId);
  useWatchScanRefresh(refreshInPlace);
  useEffect(() => refreshScan(), [refreshScan]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    clearListSnapshot();
    reload();
  }, [reload, scan.finished]);

  const saveSnapshot = useCallback(() => {
    // Do not snapshot temporary drag overlays or restore stale order after successful mutations invalidated it.
    if (shown !== items) return;
    saveListSnapshot(
      { ...criteria, tags: tagIds, ...credits, ...conditions },
      {
        items,
        total,
        cursor,
        hasMore,
        scrollY: window.scrollY,
        scanId: knownScanId.current,
        staleGroups: staleGroups(),
      },
    );
  }, [
    conditions,
    credits,
    criteria,
    cursor,
    hasMore,
    items,
    shown,
    staleGroups,
    tagIds,
    total,
  ]);

  const results = useRef<HTMLElement | null>(null);
  const marqueeBox = useRef<HTMLDivElement | null>(null);
  useMarqueeSelection({
    container: results,
    box: marqueeBox,
    enabled: drawsBoxes(owner, view, loading),
    items: shown,
    selection,
    setSelection,
  });

  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !hasMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          resetPreview();
          loadMore();
        }
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, loadMore, resetPreview]);

  const selectedIdsArray = useMemo(() => Array.from(selectedIds), [selectedIds]);
  const favoriteSelection = useMemo(() => favoriteTargets(selection), [selection]);
  const editTargets = useMemo(
    () => selectionTargets(selection, items, t.library.selection.unknownTarget),
    [selection, items],
  );

  const empty = !loading && error === null && items.length === 0;
  const initialLoadFailed = !loading && error !== null && items.length === 0;
  const smartFolderFilter = { criteria, tagIds, credits, conditions };
  const conditioned = libraryConditioned(smartFolderFilter);
  const selectionMode = selectedCount(selection) > 0;
  const resultStatus = loading ? t.list.loading : t.library.resultCount(total);

  // Stable tag-row render callbacks preserve memoized cards across preview and search updates.
  const renderTagsRow = useCallback(
    (video: Video) =>
      view === "grid" ? (
        <CardTagRow
          tags={video.tags}
          selectionMode={selectionMode}
          onPress={pressTag}
          onToggleSelection={() => toggleSelection(video.id)}
        />
      ) : undefined,
    [pressTag, selectionMode, toggleSelection, view],
  );

  const renderGroupTagsRow = useCallback(
    (group: LibraryGroup) =>
      view === "grid" ? (
        <CardTagRow
          tags={group.tags}
          selectionMode={selectionMode}
          onPress={pressTag}
          onToggleSelection={() => toggleGroupSelection(group)}
        />
      ) : undefined,
    [pressTag, selectionMode, toggleGroupSelection, view],
  );

  const renderFolderTagsRow = useFolderTagRow(
    pressTag,
    selectionMode,
    toggleSmartFolderSelection,
  );

  const groupProps = (group: LibraryGroup) => ({
    group,
    backTo: listUrl,
    selected: isGroupSelected(selection, group.id),
    selectionMode,
    onSelect: owner ? changeGroupSelection : undefined,
    ...preview,
    tagsRow: renderGroupTagsRow,
    nameField: drops.nameFieldFor(group.id),
  });

  const rowProps = (video: Video) => ({
    video,
    backTo: listUrl,
    selected: selectedIds.has(video.id),
    selectionMode,
    onSelect: owner ? changeSelection : undefined,
    ...preview,
    tagsRow: renderTagsRow,
  });

  const smartFolderProps = (folder: SmartFolderSummary) => ({
    folder,
    selected: selection.smartFolders.has(folder.id),
    selectionMode,
    onSelect: owner ? changeSmartFolderSelection : undefined,
    backTo: listUrl,
    ...preview,
    tagsRow: view === "grid" ? renderFolderTagsRow : undefined,
  });

  // Only owner video/group cards are draggable; smart-folder cards reject drops.
  const card = (item: LibraryItem) => {
    if (item.kind === "smartFolder") {
      return (
        <SmartFolderCard
          key={itemKey(item)}
          {...smartFolderProps(item.smartFolder)}
          refusesDrop={owner}
        />
      );
    }
    const node =
      item.kind === "video" ? (
        <VideoCard key={itemKey(item)} {...rowProps(item.video)} />
      ) : (
        <GroupCard key={itemKey(item)} {...groupProps(item.group)} />
      );
    return owner ? (
      <DraggableCard key={itemKey(item)} id={itemKey(item)}>
        {node}
      </DraggableCard>
    ) : (
      node
    );
  };
  const row = (item: LibraryItem) => {
    if (item.kind === "smartFolder") {
      return (
        <SmartFolderRow
          key={itemKey(item)}
          {...smartFolderProps(item.smartFolder)}
          owner={owner}
        />
      );
    }
    if (item.kind === "video") {
      const Row = owner ? DraggableVideoRow : VideoRow;
      return <Row key={itemKey(item)} {...rowProps(item.video)} />;
    }
    const Row = owner ? DraggableGroupRow : GroupRow;
    return <Row key={itemKey(item)} {...groupProps(item.group)} />;
  };
  const withDrag = (content: ReactNode) =>
    owner ? (
      <LibraryDragLayer
        items={shown}
        list={view === "list"}
        selection={selection}
        onto={drops.onto}
        onDrop={drops.hold}
        onMove={moves.move}
        onMerge={moves.merge}
        onLift={resetPreview}
        held={drops.held}
        attached={drops.attached}
      >
        {content}
      </LibraryDragLayer>
    ) : (
      content
    );

  const body = initialLoadFailed ? (
    <LoadFailed reason={error} onRetry={reload} />
  ) : empty ? (
    <LibraryEmpty
      conditioned={conditioned}
      owner={owner}
      onSearch={() => searchField.current?.focus()}
    />
  ) : loading ? (
    <LoadingState
      label={t.list.loading}
      layout={view === "grid" ? "grid" : "table"}
      size={zoom}
      count={view === "grid" ? skeletonCount : 6}
    />
  ) : (
    <div ref={list} onClick={saveSnapshot}>
      {withSmartFolderCards(
        withDrag(
          view === "grid" ? (
            // Share one width observer for all tag rows in the list.
            <TagRowMeasureProvider>
              <Grid zoom={zoom}>
                {}
                {shown.map(card)}
              </Grid>
            </TagRowMeasureProvider>
          ) : (
            <DataTable label={t.library.title}>
              <LibraryTableHeader owner={owner} />
              <TableBody>{shown.map(row)}</TableBody>
            </DataTable>
          ),
        ),
      )}
    </div>
  );

  return (
    // Group mutations can remove top-level items; refresh the list in place.
    <GroupChangedContext value={refreshInPlace}>
      <ListPage
        header={<PageHeader title={t.home.title} titleHidden />}
        toolbar={
          <TopBarPortal>
            <LibraryToolbar
              query={query}
              onQueryCommit={commitQuery}
              searchRef={searchField}
              sort={sort}
              onSortChange={changeSort}
              onShuffle={shuffle}
              favorite={favorite}
              onFilterApply={filters.toolbar.onFilterApply}
              canClear={conditioned}
              onClear={clearAll}
              view={view}
              onViewChange={(next) => {
                resetPreview();
                savePreferences({ ...preferences, view: next });
              }}
              zoom={zoom}
              onZoomChange={changeZoom}
              extraFilter={filters.toolbar.extraFilter}
              extraFilterCount={filters.toolbar.extraFilterCount}
              onFilterOpen={filters.toolbar.onFilterOpen}
              suggestions={filters.toolbar.suggestions}
            />
          </TopBarPortal>
        }
        selectionBar={
          owner && (
            <SelectionBar
              count={selectedIds.size}
              targets={editTargets}
              selectedIds={selectedIdsArray}
              favoriteVideoIds={favoriteSelection.videoIds}
              favoriteGroupIds={favoriteSelection.groupIds}
              smartFolderIds={favoriteSelection.smartFolderIds}
              onClear={clearSelection}
              onTagRemoved={onTagRemoved}
              onBundled={onBundled}
              onCollectionsDeleted={onCollectionsDeleted}
            />
          )
        }
      >
        <section
          ref={results}
          aria-label={conditioned ? t.library.resultsLabel : undefined}
          className="flex min-w-0 flex-col gap-3"
        >
          <MarqueeBox ref={marqueeBox} />
          {/* Home itself needs no heading or count; only filtered results get them. */}
          {conditioned && (
            <PageHeader
              title={t.library.resultsLabel}
              headingLevel={2}
              actions={saveSmartFolderAction(owner, conditioned, smartFolderFilter, sort)}
              count={
                initialLoadFailed ? undefined : (
                  <span
                    role="status"
                    aria-label={t.library.resultsLabel}
                    aria-live="polite"
                  >
                    {resultStatus}
                  </span>
                )
              }
            />
          )}
          {filters.activeFilters}
          {body}
          {loadingMore && <LoadMoreRow status="loading" label={t.list.loading} />}
          {error !== null && items.length > 0 && (
            <LoadMoreFailed reason={error} onRetry={retryLoadMore} />
          )}
          <div ref={sentinel} aria-hidden="true" className="h-px" />
        </section>
      </ListPage>
    </GroupChangedContext>
  );
}

function LibraryEmpty({
  conditioned,
  owner,
  onSearch,
}: {
  conditioned: boolean;
  owner: boolean;
  onSearch: () => void;
}) {
  if (conditioned) return <NoMatches onSearch={onSearch} />;
  return owner ? <EmptyLibrary /> : <GuestEmpty />;
}

export function LibraryTableHeader({ owner }: { owner: boolean }) {
  return (
    <TableHeader>
      <TableRow className="hover:bg-transparent">
        {owner && <TableHead className="w-10" />}
        <TableHead className="w-list-thumb-cell" />
        <TableHead className="pr-4">{t.library.columns.title}</TableHead>
        {owner && <TableHead className="w-8" />}
        {owner && <TableHead className="hidden w-8 sm:table-cell" />}
        {owner && <TableHead className="hidden w-8 sm:table-cell" />}
        <TableHead className="hidden w-16 pr-4 sm:table-cell" />
        <TableHead className="w-list-number pr-4 text-right">
          {t.library.columns.duration}
        </TableHead>
        <TableHead className="hidden w-list-number pr-4 text-right md:table-cell">
          {t.library.columns.quality}
        </TableHead>
        <TableHead className="hidden w-list-number-wide pr-4 text-right md:table-cell">
          {t.library.columns.size}
        </TableHead>
        <TableHead className="hidden w-list-date pr-3 text-right lg:table-cell">
          {t.library.columns.added}
        </TableHead>
      </TableRow>
    </TableHeader>
  );
}
