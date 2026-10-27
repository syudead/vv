import { FolderOpen } from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Link, useLocation } from "react-router";

import type { FolderListing, FolderRef, FolderScope } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { type FolderListingState, useFolderListing } from "../api/useFolderListing";
import { useVideos, type VideosState } from "../api/useVideos";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { useScanControls } from "../shell/ScanProvider";
import { useWatchScanRefresh } from "../shell/useWatchScanRefresh";
import { ListPage } from "../ui/patterns/list-page";
import { Button } from "../ui/shadcn/button";
import { criteriaKey } from "../videoList/listCriteria";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import { useScrollTopOnChange } from "../videoList/useScrollTopOnChange";
import { useVideoSelection } from "../videoList/useVideoSelection";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import FolderContents from "./FolderContents";
import FolderHeader from "./FolderHeader";
import FolderGroupingMenu from "./FolderGroupingMenu";
import FolderSearchResults from "./FolderSearchResults";
import TopBarPortal from "../shell/TopBarPortal";
import LibraryToolbar from "../library/LibraryToolbar";
import {
  libraryFilterParams,
  useLibraryFilters,
  useLibraryFilterValues,
} from "../library/useLibraryFilters";
import { folderKey } from "./folderPath";
import { FolderEmpty, FolderNotFound, LoadFailed } from "./states";
import { useArrival } from "./useArrival";
import { useConditions } from "./useConditions";
import { useFolderNames } from "./useFolderNames";
import { useFolderTagsRow } from "./useFolderTagsRow";

export default function FolderView({ folder }: { folder: FolderRef }) {
  const location = useLocation();
  const {
    criteria,
    zoom,
    searchField,
    changeSort,
    shuffle,
    commitQuery,
    apply,
    view,
    changeView,
    changeZoom: saveZoom,
  } = useConditions(libraryFilterParams);
  const scan = useScanControls();
  const owner = useAudience() === "owner";
  // Folder listings take no video conditions, so the folder page ignores them.
  const { tagIds, credits } = useLibraryFilterValues(owner);
  const filterKey = useMemo(() => ({ tags: tagIds, ...credits }), [tagIds, credits]);
  const { refresh: refreshScan } = scan;
  const searching = criteria.query !== "";
  // Search recursively when a query exists; otherwise filter only direct children.
  // Omit the default direct scope to preserve URL and request shapes.
  const scope: FolderScope | undefined = searching ? "subtree" : undefined;

  // Restore the cached list when returning from playback.
  const key = folderKey(folder);
  const [restored] = useState(() => {
    const held = takeListSnapshot({ ...criteria, ...filterKey, folder: key });
    return held?.folderListing === undefined ? undefined : held;
  });
  const heading = useArrival<HTMLHeadingElement>(restored !== undefined);
  const listing = useFolderListing(folder, restored?.folderListing);
  const videos = useVideos(
    {
      query: criteria.query,
      favorite: criteria.favorite,
      sort: criteria.sort,
      seed: criteria.seed,
      scope,
      tag: tagIds,
      ...credits,
    },
    restored,
    folder,
  );

  // Allow one preview at a time; stop it when the list or zoom changes.
  const { resetPreview, cardProps: preview } = usePreviewCoordination();
  useEffect(() => {
    resetPreview();
  }, [resetPreview, videos.items, zoom]);
  const filters = useLibraryFilters({
    owner,
    criteria,
    apply,
    tagIds,
    credits,
    missingTagIds: videos.missingTagIds,
    restored: restored !== undefined,
    resetPreview,
    searchField,
  });
  const filtered = filters.conditioned;
  // useConditions resets scroll for the shared criteria; the tag and credit filters need the same.
  useScrollTopOnChange(filters.signature);
  const { listRef, capture } = useZoomAnchor(zoom);
  const changeZoom = useCallback(
    (next: typeof zoom) => {
      capture();
      saveZoom(next);
    },
    [capture, saveZoom],
  );

  const tagsRow = useFolderTagsRow();

  const { summary, children, rootName, rootNameFailed, name } = useFolderNames(
    folder,
    listing,
  );
  const backTo = `${location.pathname}${location.search}`;

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
    if (top === undefined || (videos.items.length === 0 && children.length === 0)) return;
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
    // Realign before paint: TopBarPortal initially occupies body space, then moves and shifts scroll anchoring.
    requestAnimationFrame(() => window.scrollTo({ top, behavior: "auto" }));
  }, [children.length, videos.items.length]);

  // Imports completed before mounting are already reflected in loaded content.
  const knownScanId = useRef(
    restored === undefined ? scan.finished?.id : restored.scanId,
  );
  // Do not snapshot until both folders and videos finish refreshing; intermediate data predates the completed import.
  const reloadPending = useRef(false);
  useEffect(() => {
    if (!listing.loading && !videos.loading) reloadPending.current = false;
  }, [listing.loading, videos.loading]);
  const { reload: reloadListing } = listing;
  const { reload: reloadVideos, refreshInPlace: refreshVideos } = videos;
  const { refresh: refreshListing } = listing;
  const selection = useVideoSelection(
    videos.items,
    owner,
    refreshVideos,
    view === "grid",
  );
  const { clear: clearSelection } = selection;
  const listKey = `${criteriaKey(criteria)}\u0000${filters.signature}`;
  useEffect(() => clearSelection(), [clearSelection, listKey]);
  // Refresh in place after automatic imports, retaining visible rows.
  const refreshOpenLists = useCallback(() => {
    refreshListing();
    refreshVideos();
  }, [refreshListing, refreshVideos]);
  useWatchScanRefresh(refreshOpenLists);
  useEffect(() => refreshScan(), [refreshScan]);
  useEffect(() => {
    const finished = scan.finished;
    if (finished === null || knownScanId.current === finished.id) return;
    knownScanId.current = finished.id;
    reloadPending.current = true;
    clearListSnapshot();
    reloadListing();
    reloadVideos();
  }, [reloadListing, reloadVideos, scan.finished]);

  // Reload after tagging because existing cards carry stale tags.
  // Restore scroll after skeletons shrink the list during refresh.
  // Fetch successive pages until the previous position is reachable, pagination ends, or a page fails.
  const scrollAfterReload = useRef<number | undefined>(undefined);
  const { hasMore, loadMore } = videos;
  useLayoutEffect(() => {
    const top = scrollAfterReload.current;
    if (top === undefined || videos.loading || videos.loadingMore) return;
    window.scrollTo({ top, behavior: "auto" });
    const reachable = document.documentElement.scrollHeight - window.innerHeight;
    if (top <= reachable || !hasMore || videos.error !== null) {
      scrollAfterReload.current = undefined;
      return;
    }
    loadMore();
  }, [
    hasMore,
    loadMore,
    videos.error,
    // Track list identity, not length: an immediate response may skip loading and preserve the length.
    videos.items,
    videos.loading,
    videos.loadingMore,
  ]);
  const { update: updateListing } = listing;
  const grouping = summary?.grouping;
  const groupingMenu =
    // Guest responses omit grouping, so hide the trigger.
    owner && grouping !== undefined ? (
      <FolderGroupingMenu
        folder={folder}
        name={name ?? summary?.name ?? ""}
        grouping={grouping}
        onGroupingChange={(next) =>
          updateListing((data) => ({
            ...data,
            folder: { ...data.folder, grouping: next },
          }))
        }
        onTagged={() => {
          scrollAfterReload.current = window.scrollY;
          // Do not cache an empty intermediate refresh state.
          reloadPending.current = true;
          reloadVideos();
        }}
        onStale={reloadListing}
      />
    ) : undefined;

  const saveSnapshot = useCallback(() => {
    if (listing.data === null || reloadPending.current) return;
    saveListSnapshot(
      { ...criteria, ...filterKey, folder: key },
      {
        items: videos.items,
        total: videos.total,
        cursor: videos.cursor,
        hasMore: videos.hasMore,
        scrollY: window.scrollY,
        scanId: knownScanId.current,
        folderListing: listing.data,
      },
    );
  }, [
    criteria,
    filterKey,
    key,
    listing.data,
    videos.cursor,
    videos.hasMore,
    videos.items,
    videos.total,
  ]);

  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !hasMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasMore, loadMore]);

  const noVideosAtAll = isEmptyFolder({
    listing,
    videos,
    loaded: summary !== undefined,
    searching,
    filtered,
    childCount: children.length,
  });

  let body: ReactNode;
  if (listing.notFound || videos.notFound) {
    // A search 404 means the folder is missing, not that no videos match.
    body = <FolderNotFound />;
  } else if (listing.error !== null) {
    body = <LoadFailed reason={listing.error} onRetry={listing.reload} />;
  } else if (searching) {
    body = (
      <FolderSearchResults
        folder={folder}
        videos={videos}
        view={view}
        zoom={zoom}
        backTo={backTo}
        preview={preview}
        selection={selection}
        tagsRow={tagsRow}
      />
    );
  } else if (noVideosAtAll) {
    body = (
      <FolderEmpty
        icon={<FolderOpen aria-hidden="true" />}
        title={
          owner && folder.path === "" ? t.folders.empty.rootTitle : t.folders.empty.title
        }
        description={owner ? t.folders.empty.description : undefined}
        action={
          owner ? (
            <Button asChild size="sm">
              <Link to="/settings/scan#scan-status">{t.list.scan}</Link>
            </Button>
          ) : undefined
        }
      />
    );
  } else {
    body = (
      <FolderContents
        filtered={filtered}
        listingLoading={listing.loading}
        childFolders={children}
        videos={videos}
        view={view}
        zoom={zoom}
        backTo={backTo}
        preview={preview}
        selection={selection}
        tagsRow={tagsRow}
        groupingMenu={groupingMenu}
      />
    );
  }

  return (
    <ListPage
      header={
        <FolderHeader
          folder={folder}
          rootName={rootName}
          listingNotFound={listing.notFound}
          videosNotFound={videos.notFound}
          listingFailed={listing.error !== null}
          rootNameFailed={rootNameFailed}
          searching={searching}
          name={name}
          heading={heading}
        />
      }
      toolbar={
        <TopBarPortal>
          <LibraryToolbar
            query={criteria.query}
            onQueryCommit={commitQuery}
            searchRef={searchField}
            searchLabel={t.folders.searchIn(name ?? t.folders.title)}
            searchPlaceholder={t.folders.searchInPlaceholder}
            sort={criteria.sort}
            onSortChange={changeSort}
            onShuffle={shuffle}
            favorite={criteria.favorite}
            {...filters.toolbar}
            canClear={filtered}
            onClear={filters.clearAll}
            view={view}
            onViewChange={(next) => {
              resetPreview();
              changeView(next);
            }}
            zoom={zoom}
            onZoomChange={changeZoom}
          />
        </TopBarPortal>
      }
      selectionBar={selection.bar}
    >
      {/* Snapshot immediately before navigation; display: contents preserves the layout's spacing. */}
      <div ref={listRef} onClick={saveSnapshot} className="contents">
        <div ref={selection.container} className="contents">
          {filters.activeFilters}
          {body}
        </div>
      </div>
      {selection.marqueeBox}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </ListPage>
  );
}

/** The folder has nothing to list: it loaded with no child folders or videos and no search or filter narrows it. */
function isEmptyFolder({
  listing,
  videos,
  loaded,
  searching,
  filtered,
  childCount,
}: {
  listing: FolderListingState<FolderListing>;
  videos: VideosState;
  loaded: boolean;
  searching: boolean;
  filtered: boolean;
  childCount: number;
}): boolean {
  return (
    !listing.loading &&
    !videos.loading &&
    loaded &&
    !searching &&
    !filtered &&
    childCount === 0 &&
    videos.items.length === 0 &&
    videos.error === null
  );
}
