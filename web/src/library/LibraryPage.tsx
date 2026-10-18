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
  listLibraryIds,
  type SmartFolderSummary,
  type TagRef,
  type Video,
  type VideoSort,
} from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import type { CreditKind } from "../api/credits";
import { itemKey, itemVideo } from "../api/libraryItems";
import { refreshTags, revalidateTags } from "../api/tags";
import { useVideos } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { GroupChangedContext } from "../groups/groupChanges";
import { errorText, t } from "../i18n";
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
import { useToast } from "../ui/Toast";
import {
  criteriaKey,
  type HistoryMode,
  type ListCriteria,
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
import ActiveTagFilters from "./ActiveTagFilters";
import {
  addedLengthCount,
  addedLengthParams,
  addedLengthSignature,
  clearedAddedLength,
  hasAddedLength,
  useAddedLength,
} from "./addedLengthCriteria";
import { AddedSelect, LengthSelect } from "./AddedLengthSelects";
import {
  clearedCredits,
  creditSignature,
  hasFilterChips,
  ORGANIZATION_PARAM,
  PERSON_PARAM,
  TAG_KIND_PARAM,
  useCreditFilter,
  withoutCredit,
} from "./creditCriteria";
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
import {
  emptySelection,
  favoriteTargets,
  fromSelectAll,
  type LibrarySelection,
  reconcileGroups,
  sameSelection,
  selectedGroupOf,
  isGroupSelected,
  setGroup,
  setVideo,
  toggleGroup,
  toggleVideo,
} from "./selection";
import SelectionBar from "./SelectionBar";
import TagKindSelect from "./TagKindSelect";
import { TagRowMeasureProvider } from "./TagRowMeasure";
import { useAddedLengthControls } from "./useAddedLengthControls";
import { useEscapeClearsSelection } from "./useEscapeClearsSelection";
import { useFolderTagRow } from "./useCardTagRows";
import { type ApplyTagKind, useLibraryTagKinds } from "./useLibraryTagKinds";
import {
  addTagId,
  clearConditions as clearTagConditions,
  hasConditions as hasTagConditions,
  MAX_TAG_COUNT,
  parseTagParam,
  removeTagId,
  serializeTagIds,
  TAG_PARAM,
} from "./tagCriteria";

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

function libraryConditioned(filter: LibraryFilter): boolean {
  return (
    hasTagConditions(filter.criteria, filter.tagIds, filter.credits) ||
    hasAddedLength(filter.addedLength)
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

const libraryParams = [
  TAG_PARAM,
  PERSON_PARAM,
  ORGANIZATION_PARAM,
  TAG_KIND_PARAM,
  ...addedLengthParams,
];

/** Added-date and length filters are available to guests. */
const libraryGuestParams = addedLengthParams;

export default function LibraryPage() {
  const location = useLocation();
  const toast = useToast();
  // Hide owner-data actions from guests.
  //
  const owner = useAudience() === "owner";
  const [preferences, setPreferences] = useState(readViewPreferences);
  // URL owns criteria; only sort is persisted on the device.
  // Pass library-specific filters through useListCriteria's extension so every URL update preserves them.
  // Added-date and length filters are also available to guests.
  const { criteria, apply } = useListCriteria(
    preferences.sort,
    libraryParams,
    libraryGuestParams,
  );
  const { query, watch, playable, favorite, sort } = criteria;
  // Preserve tag-array identity across unrelated URL changes so card callbacks and memoization remain stable.
  const rawTagKey = JSON.stringify(
    new URLSearchParams(location.search).getAll(TAG_PARAM),
  );
  const tagIds = useMemo(
    () => (owner ? parseTagParam(JSON.parse(rawTagKey) as string[]) : []),
    [owner, rawTagKey],
  );
  const credits = useCreditFilter(location.search, owner);
  const parsedAddedLength = useAddedLength(location.search);
  const addedLength = parsedAddedLength.filter;
  const libraryFilter = useMemo(
    () => ({ ...credits, ...addedLength }),
    [credits, addedLength],
  );
  const { zoom, view } = preferences;
  const searchField = useRef<HTMLInputElement | null>(null);
  const { activePreviewId, previewResetEpoch, startPreview, resetPreview } =
    usePreviewCoordination();

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
  const { shuffle, changeWatch, changePlayable, changeFavorite, commitQuery } =
    useListActions(update);
  const clearAll = useCallback(() => {
    resetPreview();
    const cleared = clearTagConditions(criteria);
    apply(cleared.criteria, "push", {
      [TAG_PARAM]: serializeTagIds(cleared.tag),
      ...clearedCredits,
      ...clearedAddedLength,
    });
  }, [apply, criteria, resetPreview]);

  // Read current criteria through refs to keep card callbacks stable across unrelated renders.
  const latestConditions = useRef({ criteria, tagIds });
  latestConditions.current = { criteria, tagIds };
  const applyTags = useCallback(
    (next: ListCriteria, mode: HistoryMode, ids: readonly number[]) =>
      apply(next, mode, { [TAG_PARAM]: serializeTagIds(ids) }),
    [apply],
  );

  const removeCredit = useCallback(
    (kind: CreditKind) => {
      resetPreview();
      apply(latestConditions.current.criteria, "push", withoutCredit(kind));
    },
    [apply, resetPreview],
  );
  // Remove missing credits with history replacement and notify; changed criteria reload the list.
  const creditGone = useCallback(
    (kind: CreditKind) => {
      toast(
        kind === "person"
          ? t.library.activeTags.personGone
          : t.library.activeTags.organizationGone,
      );
      apply(latestConditions.current.criteria, "replace", withoutCredit(kind));
    },
    [apply, toast],
  );
  const applyTagKind = useCallback<ApplyTagKind>(
    (extra, mode) => {
      resetPreview();
      apply(latestConditions.current.criteria, mode, extra);
    },
    [apply, resetPreview],
  );
  const tagKinds = useLibraryTagKinds(credits.tagKind, applyTagKind);
  const { changeAdded, changeLength } = useAddedLengthControls(
    parsedAddedLength,
    applyTagKind,
  );
  const changeTagKind = tagKinds.change;
  const removeTagKind = useCallback(() => changeTagKind(undefined), [changeTagKind]);

  const pressTag = useCallback(
    (tag: TagRef) => {
      const { criteria: current, tagIds: currentTagIds } = latestConditions.current;
      if (currentTagIds.includes(tag.id)) return;
      if (currentTagIds.length >= MAX_TAG_COUNT) {
        toast(t.errors.reason.too_many_tag_filters({ limit: MAX_TAG_COUNT }));
        return;
      }
      resetPreview();
      applyTags(current, "push", addTagId(currentTagIds, tag.id));
    },
    [applyTags, resetPreview, toast],
  );
  const removeActiveTag = useCallback(
    (id: number) => {
      resetPreview();
      applyTags(criteria, "push", removeTagId(tagIds, id));
    },
    [applyTags, criteria, resetPreview, tagIds],
  );
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
    takeListSnapshot({ ...criteria, tags: tagIds, ...libraryFilter }),
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
  } = useVideos({ ...criteria, tag: tagIds, ...libraryFilter }, restored, "library");

  // Keep newly nonmatching watched rows in place until the next reload.
  useEffect(() => {
    resetPreview();
  }, [favorite, items, playable, query, resetPreview, sort, view, watch, zoom]);

  // Selection retains video IDs and group identity; video mutations expand groups, while favorites target groups themselves.
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  const selectedIds = selection.ids;
  // Disable Select all only while both IDs and group identity still match its last response.
  const [selectAllSelection, setSelectAllSelection] = useState<LibrarySelection | null>(
    null,
  );
  const [selectingAll, setSelectingAll] = useState(false);
  // Invalidate Select all on manual selection or condition changes so late responses cannot overwrite newer intent.
  const selectAllSeq = useRef(0);
  // Reset selectingAll immediately: the invalidated request's finally skips state changes.
  const invalidateSelectAll = useCallback(() => {
    selectAllSeq.current += 1;
    setSelectingAll(false);
  }, []);
  // Removing any member also removes the group's selected identity.
  const changeSelection = useCallback(
    (id: number, selected: boolean) => {
      invalidateSelectAll();
      setSelection((current) => setVideo(current, id, selected));
    },
    [invalidateSelectAll],
  );
  // Functional updates preserve stable chip callbacks without capturing the current selection.
  const toggleSelection = useCallback(
    (id: number) => {
      invalidateSelectAll();
      setSelection((current) => toggleVideo(current, id));
    },
    [invalidateSelectAll],
  );
  const changeGroupSelection = useCallback(
    (group: LibraryGroup, selected: boolean) => {
      invalidateSelectAll();
      setSelection((current) => setGroup(current, selectedGroupOf(group), selected));
    },
    [invalidateSelectAll],
  );
  const toggleGroupSelection = useCallback(
    (group: LibraryGroup) => {
      invalidateSelectAll();
      setSelection((current) => toggleGroup(current, selectedGroupOf(group)));
    },
    [invalidateSelectAll],
  );
  const clearSelection = useCallback(() => {
    invalidateSelectAll();
    setSelection(emptySelection);
    // Clear the Select all comparison when selection empties; manually reselecting IDs may still omit unloaded items.
    //
    setSelectAllSelection(null);
  }, [invalidateSelectAll]);

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

  const selectAll = useCallback(() => {
    const seq = (selectAllSeq.current += 1);
    setSelectingAll(true);
    listLibraryIds({ query, watch, playable, favorite, tag: tagIds, ...libraryFilter })
      .then((response) => {
        // Ignore Select all responses superseded by manual selection, another request, or condition changes.
        if (selectAllSeq.current !== seq) return;
        const missing = response.missingTagIds ?? [];
        if (missing.length > 0) {
          // Apply missing-filter removals to current criteria, not the criteria captured when sending.
          const { criteria: latestCriteria, tagIds: latestTagIds } =
            latestConditions.current;
          const remaining = latestTagIds.filter((id) => !missing.includes(id));
          toast(t.library.deletedTagsRemoved);
          refreshTags().catch(() => undefined);
          applyTags(latestCriteria, "replace", remaining);
          return;
        }
        const selected = fromSelectAll(response.ids, response.groups ?? []);
        setSelection(selected);
        setSelectAllSelection(selected);
      })
      .catch((failure: unknown) => {
        if (selectAllSeq.current !== seq) return;
        toast(t.library.selectAllFailed(errorText(failure)));
      })
      .finally(() => {
        if (selectAllSeq.current === seq) setSelectingAll(false);
      });
  }, [applyTags, libraryFilter, favorite, playable, query, tagIds, toast, watch]);

  // Removing an active filter tag in bulk clears selection and reloads matches, but retains the valid filter itself.
  const onTagRemoved = useCallback(
    (tagId: number) => {
      if (!latestConditions.current.tagIds.includes(tagId)) return;
      clearSelection();
      clearListSnapshot();
      reload();
    },
    [clearSelection, reload],
  );

  // Never prune selected IDs to loaded items; Select all includes unloaded pages.

  // Reconcile selected group identity after refresh without removing selected video IDs.
  useEffect(() => {
    setSelection((current) => reconcileGroups(current, items));
  }, [items]);

  // Clear selection whenever conditions change, even when some videos match both views.
  const conditionsSignature = [
    criteriaKey(criteria),
    tagIds.join(","),
    creditSignature(credits),
    addedLengthSignature(addedLength),
  ].join("\u0000");
  const knownConditionsSignature = useRef(conditionsSignature);
  useEffect(() => {
    if (knownConditionsSignature.current === conditionsSignature) return;
    knownConditionsSignature.current = conditionsSignature;
    clearSelection();
  }, [clearSelection, conditionsSignature]);
  useScrollTopOnChange(conditionsSignature);

  useEscapeClearsSelection(selectedIds.size > 0, clearSelection);

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
      { ...criteria, tags: tagIds, ...libraryFilter },
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
    libraryFilter,
    criteria,
    cursor,
    hasMore,
    items,
    shown,
    staleGroups,
    tagIds,
    total,
  ]);

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

  // Remove missing tag filters once per response content; URL replacement can cause repeated intermediate renders.
  const handledMissingTagIds = useRef<string | null>(null);
  useEffect(() => {
    if (missingTagIds.length === 0) {
      handledMissingTagIds.current = null;
      return;
    }
    const signature = [...missingTagIds].sort((a, b) => a - b).join(",");
    if (handledMissingTagIds.current === signature) return;
    handledMissingTagIds.current = signature;
    const remaining = tagIds.filter((id) => !missingTagIds.includes(id));
    if (remaining.length === tagIds.length) return;
    toast(t.library.deletedTagsRemoved);
    refreshTags().catch(() => undefined);
    applyTags(criteria, "replace", remaining);
  }, [applyTags, criteria, missingTagIds, tagIds, toast]);

  // Restored snapshots skip list requests, so validate URL tags against the refreshed shared tag list on mount.
  const mountRef = useRef({ criteria, tagIds, apply: applyTags, toast });
  mountRef.current = { criteria, tagIds, apply: applyTags, toast };
  useEffect(() => {
    if (restored === undefined || mountRef.current.tagIds.length === 0) return;
    let alive = true;
    revalidateTags()
      .then((tags) => {
        if (!alive) return;
        const known = new Set(tags.map((tag) => tag.id));
        const {
          tagIds: current,
          criteria: currentCriteria,
          apply: currentApply,
        } = mountRef.current;
        const remaining = current.filter((id) => known.has(id));
        if (remaining.length === current.length) return;
        mountRef.current.toast(t.library.deletedTagsRemoved);
        currentApply(currentCriteria, "replace", remaining);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [restored]);

  const selectedIdsArray = useMemo(() => Array.from(selectedIds), [selectedIds]);
  const allSelected = useMemo(
    () => selectAllSelection !== null && sameSelection(selectAllSelection, selection),
    [selectAllSelection, selection],
  );
  const favoriteSelection = useMemo(() => favoriteTargets(selection), [selection]);

  const empty = !loading && error === null && items.length === 0;
  const initialLoadFailed = !loading && error !== null && items.length === 0;
  const smartFolderFilter = { criteria, tagIds, credits, addedLength };
  const conditioned = libraryConditioned(smartFolderFilter);
  const selectionMode = selectedIds.size > 0;
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

  // Smart folders have no selection state, so their tag filters remain usable during other card selection.
  const renderFolderTagsRow = useFolderTagRow(pressTag);

  const groupProps = (group: LibraryGroup) => ({
    group,
    backTo: listUrl,
    selected: isGroupSelected(selection, group.id),
    selectionMode,
    onSelect: owner ? changeGroupSelection : undefined,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart: startPreview,
    onPreviewReset: resetPreview,
    tagsRow: renderGroupTagsRow,
    nameField: drops.nameFieldFor(group.id),
  });

  const rowProps = (video: Video) => ({
    video,
    backTo: listUrl,
    selected: selectedIds.has(video.id),
    selectionMode,
    onSelect: owner ? changeSelection : undefined,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart: startPreview,
    onPreviewReset: resetPreview,
    tagsRow: renderTagsRow,
  });

  const smartFolderProps = (folder: SmartFolderSummary) => ({
    folder,
    selectionMode,
    backTo: listUrl,
    activePreviewId,
    previewResetEpoch,
    onPreviewStart: startPreview,
    onPreviewReset: resetPreview,
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
              watch={watch}
              onWatchChange={changeWatch}
              playable={playable}
              onPlayableChange={changePlayable}
              favorite={favorite}
              onFavoriteChange={changeFavorite}
              canClear={conditioned}
              onClear={clearAll}
              view={view}
              onViewChange={(next) => {
                resetPreview();
                savePreferences({ ...preferences, view: next });
              }}
              zoom={zoom}
              onZoomChange={changeZoom}
              extraFilter={
                <>
                  {owner && (
                    <TagKindSelect
                      value={credits.tagKind}
                      kinds={tagKinds.kinds}
                      onValueChange={changeTagKind}
                    />
                  )}
                  <AddedSelect
                    value={addedLength.addedWithin}
                    onValueChange={changeAdded}
                  />
                  <LengthSelect value={addedLength} onValueChange={changeLength} />
                </>
              }
              extraFilterCount={
                (credits.tagKind === undefined ? 0 : 1) + addedLengthCount(addedLength)
              }
              onFilterOpen={owner ? tagKinds.load : undefined}
            />
          </TopBarPortal>
        }
        selectionBar={
          owner && (
            <SelectionBar
              count={selectedIds.size}
              allSelected={allSelected}
              selectedIds={selectedIdsArray}
              favoriteVideoIds={favoriteSelection.videoIds}
              favoriteGroupIds={favoriteSelection.groupIds}
              selectingAll={selectingAll}
              onSelectAll={selectAll}
              onClear={clearSelection}
              onTagRemoved={onTagRemoved}
              onBundled={onBundled}
            />
          )
        }
      >
        <section aria-label={t.library.title} className="flex min-w-0 flex-col gap-3">
          <PageHeader
            title={t.library.title}
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
          {hasFilterChips(tagIds, credits) && (
            <ActiveTagFilters
              tagIds={tagIds}
              onRemove={removeActiveTag}
              credits={credits}
              onRemoveCredit={removeCredit}
              onCreditGone={creditGone}
              tagKindName={tagKinds.name}
              onRemoveTagKind={removeTagKind}
              searchFieldRef={searchField}
            />
          )}
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
