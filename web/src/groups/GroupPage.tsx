import { Folder } from "lucide-react";
import {
  type ComponentProps,
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { Link, useLocation, useParams } from "react-router";

import type { LibraryGroup, TagRef, Video, VideoFolder } from "../api/client";
import type { GroupItem, GroupRef } from "../api/groups";
import { useRootFolderName } from "../api/useFolderListing";
import { useAudience } from "../auth/audience";
import Breadcrumbs from "../folders/Breadcrumbs";
import { folderUrl } from "../folders/folderPath";
import { t } from "../i18n";
import { formatDuration } from "../lib/format";
import { GroupCard } from "../library/GroupCard";
import { viewControls } from "../library/LibraryToolbar";
import {
  emptySelection,
  type LibrarySelection,
  sameSelection,
  selectedGroupOf,
  isGroupSelected,
  setGroup,
  setVideo,
  toggleGroup,
  toggleVideo,
} from "../library/selection";
import SelectionBar from "../library/SelectionBar";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import { useCardTagRows } from "../library/useCardTagRows";
import { useEscapeClearsSelection } from "../library/useEscapeClearsSelection";
import type { ViewMode, Zoom } from "../preferences/viewPreferences";
import TopBarPortal from "../shell/TopBarPortal";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import SearchBox from "../videoList/SearchBox";
import { useSearchQuery } from "../videoList/useSearchQuery";
import { NoMatches } from "../videoList/states";
import { Toolbar } from "../ui/patterns/toolbar";
import { Button } from "../ui/shadcn/button";
import { crumbKey, CrumbDropTarget, DraggableCard } from "../videoList/cardDrag";
import { Grid } from "../videoList/Grid";
import {
  useDocumentTitle,
  useLoadMoreSentinel,
  usePressTagToLibrary,
  useSelectionTargets,
  useViewControls,
} from "../videoList/pageHooks";
import {
  GoneState,
  LoadingTrailHeader,
  PlayLink,
  UnavailableTrailHeader,
} from "../videoList/trailHeader";
import { type UndoToast, useUndoToast } from "../videoList/undoToast";
import { useVideoDrops } from "../videoList/useVideoDrops";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import VideoCard from "../videoList/VideoCard";
import { GroupChangedContext } from "./groupChanges";
import {
  type GroupDestination,
  GroupDragLayer,
  type GroupDropOnto,
  itemKey,
  type MoveDestination,
} from "./GroupDrag";
import { type GroupItemEditing, GroupItemList } from "./GroupItemList";
import { reconcileGroupSelection, selectAllInGroup, selectedItemIds } from "./groupItems";
import GroupMenu from "./GroupMenu";
import {
  folderDisplayPath,
  groupItemPath,
  loadingCrumbs,
  parseGroupId,
  parseTrail,
  trailCrumbs,
  videoInGroupPath,
} from "./groupPaths";
import { useGroupEdits } from "./useGroupEdits";
import { type GroupPageData, useGroupPage } from "./useGroupPage";
import { useLeaveGroup } from "./useLeaveGroup";

const skeletonCount = 12;

/** Keep the empty trail identity stable across renders. */
const noTrail: readonly GroupRef[] = [];

/** Keep the empty items identity stable across renders. */
const noItems: readonly GroupItem[] = [];

function itemVideo(item: GroupItem): Video | undefined {
  return item.kind === "video" ? item.video : undefined;
}

function itemGroup(item: GroupItem): LibraryGroup | undefined {
  return item.kind === "group" ? item.group : undefined;
}

function newGroupItem(group: LibraryGroup, itemId: number): GroupItem {
  return { id: itemId, kind: "group", group };
}

/** Key by route ID and trail because React Router otherwise reuses state across group navigation. */
export default function GroupPage() {
  const params = useParams();
  const location = useLocation();
  const trail = parseTrail(location.search);
  const id = parseGroupId(params.id);
  return (
    <GroupScreen key={`${params.id ?? ""}?${trail.join(",")}`} id={id} trail={trail} />
  );
}

/** Place a new group at the drop target's position; shownData overlays optimistic changes. */
function useGroupPageDrops({
  data,
  id,
  refresh,
  clear,
  undo,
}: {
  data: GroupPageData | null;
  id: number | null;
  refresh: () => void;
  clear: () => void;
  undo: UndoToast;
}) {
  const drops = useVideoDrops({
    items: data?.items ?? noItems,
    keyOf: itemKey,
    videoOf: itemVideo,
    groupOf: itemGroup,
    from: id,
    groupItem: newGroupItem,
    reread: refresh,
    onAccepted: clear,
    undo,
  });
  const shownItems = drops.shown;
  const shownData = useMemo(
    () => (data === null ? null : { ...data, items: shownItems as GroupItem[] }),
    [data, shownItems],
  );
  const dropOnto = useMemo(
    (): GroupDropOnto => ({
      onto: drops.onto,
      onDrop: drops.hold,
      held: drops.held,
      attached: drops.attached,
    }),
    [drops.attached, drops.held, drops.hold, drops.onto],
  );
  return { shownData, dropOnto, nameFieldFor: drops.nameFieldFor };
}

function useGroupSelection(data: GroupPageData | null, searching: boolean) {
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  const videoIds = useMemo(() => {
    if (data === null) return [];
    return searching
      ? data.items.flatMap(
          (item) => item.group?.videoIds ?? (item.video ? [item.video.id] : []),
        )
      : data.group.videoIds;
  }, [data, searching]);

  // Reconcile refreshed members while retaining selected videos on unloaded pages.
  useEffect(() => {
    if (data === null) return;
    setSelection((current) => reconcileGroupSelection(current, videoIds, data.items));
  }, [data, videoIds]);

  // Recompute Select all from current membership so newly added items make it available again.
  const all = useMemo(() => {
    if (data === null) return null;
    return selectAllInGroup(videoIds, data.items);
  }, [data, videoIds]);

  const clear = useCallback(() => setSelection(emptySelection), []);
  const changeVideo = useCallback(
    (videoId: number, selected: boolean) =>
      setSelection((current) => setVideo(current, videoId, selected)),
    [],
  );
  const changeGroup = useCallback(
    (group: LibraryGroup, selected: boolean) =>
      setSelection((current) => setGroup(current, selectedGroupOf(group), selected)),
    [],
  );
  const toggleVideoSelection = useCallback(
    (videoId: number) => setSelection((current) => toggleVideo(current, videoId)),
    [],
  );
  const toggleGroupSelection = useCallback(
    (group: LibraryGroup) =>
      setSelection((current) => toggleGroup(current, selectedGroupOf(group))),
    [],
  );
  const changeItem = useCallback(
    (item: GroupItem, selected: boolean) => {
      if (item.kind === "group" && item.group !== undefined) {
        changeGroup(item.group, selected);
      } else if (item.video !== undefined) {
        changeVideo(item.video.id, selected);
      }
    },
    [changeGroup, changeVideo],
  );
  const selectAll = useCallback(() => {
    if (all !== null) setSelection(all);
  }, [all]);

  useEscapeClearsSelection(selection.ids.size > 0, clear);

  return {
    selection,
    allSelected: all !== null && sameSelection(all, selection),
    clear,
    changeVideo,
    changeGroup,
    changeItem,
    toggleVideoSelection,
    toggleGroupSelection,
    selectAll,
  };
}

function GroupScreen({ id, trail }: { id: number | null; trail: readonly number[] }) {
  const location = useLocation();
  const owner = useAudience() === "owner";
  const { query, commitQuery, searchRef } = useSearchQuery();
  const {
    state,
    more,
    reload,
    refresh,
    loadMore,
    update,
    confirmed,
    moves,
    movesSent,
    markGone,
  } = useGroupPage(id, trail, query);
  const data = state.kind === "ready" ? state.data : null;
  //
  const leave = useLeaveGroup(data?.trail ?? noTrail);
  const name = data?.group.name;
  const dissolvedByRemoval = useCallback(() => {
    if (name !== undefined) leave(t.groups.dissolved(name));
  }, [leave, name]);
  // Dismiss undo notifications on leaving; successful undo refreshes silently.
  //
  const undo = useUndoToast(refresh);
  const edits = useGroupEdits({
    id,
    update,
    confirmed,
    moves,
    movesSent,
    refresh,
    onDissolved: dissolvedByRemoval,
    undo,
  });
  const preview = usePreviewCoordination();
  const { resetPreview } = preview;
  const { view, zoom, changeView, changeZoom, listRef } = useViewControls(resetPreview);
  const selection = useGroupSelection(data, query !== "");
  const listUrl = `${location.pathname}${location.search}`;

  useDocumentTitle(data?.group.name);
  useEffect(() => {
    resetPreview();
  }, [data, resetPreview, view, zoom]);

  const sentinel = useLoadMoreSentinel(
    data?.nextCursor !== undefined && more === "idle",
    loadMore,
    resetPreview,
  );
  const pressTag = usePressTagToLibrary();

  const selectionMode = selection.selection.ids.size > 0;
  const { favorites, selectedIds } = useSelectionTargets(selection.selection);

  const { clear } = selection;
  useEffect(() => {
    clear();
  }, [query, clear]);

  const { shownData, dropOnto, nameFieldFor } = useGroupPageDrops({
    data,
    id,
    refresh,
    clear,
    undo,
  });

  const afterBulk = useCallback(() => {
    clear();
    refresh();
  }, [clear, refresh]);

  const removable = useMemo(
    () => (data === null ? [] : selectedItemIds(selection.selection, data.items)),
    [data, selection.selection],
  );
  const { removeSelected } = edits;
  const removeChosen = useCallback(() => {
    if (data !== null) removeSelected(removable, data.group.name, clear);
  }, [clear, data, removable, removeSelected]);

  const { focus, reorder, move, merge } = edits;
  const reorderItems = useCallback(
    (itemIds: readonly number[], after: number | null) => {
      if (data !== null) reorder(itemIds, after, data.group.name);
    },
    [data, reorder],
  );
  // Clear selection for moved or merged items because they disappear from this page.
  //
  const moveItems = useCallback(
    (moved: readonly GroupItem[], to: MoveDestination) => {
      if (data === null) return;
      move(moved, to, data.group.name);
      clear();
    },
    [clear, data, move],
  );
  const mergeItems = useCallback(
    (merged: readonly GroupItem[], into: GroupDestination) => {
      merge(merged, into);
      clear();
    },
    [clear, merge],
  );

  const renamed = useCallback(
    (group: LibraryGroup) => {
      update((current) => ({ ...current, group }));
      focus("more");
    },
    [focus, update],
  );

  // Reload only when this group receives items; creating a top-level group does not change its contents.
  const groupChanged = useCallback(
    (targetId: number | null) => {
      if (targetId !== null && targetId === id) refresh();
    },
    [id, refresh],
  );

  const page = (
    <ListPage
      header={
        <GroupHeader
          state={state}
          trail={trail}
          id={id}
          owner={owner}
          listUrl={listUrl}
          menu={
            owner &&
            data !== null && (
              <GroupMenu
                group={data.group}
                moreRef={edits.moreRef}
                onRenamed={renamed}
                onLeave={leave}
                onGone={markGone}
              />
            )
          }
          titleRef={edits.titleRef}
        />
      }
      toolbar={
        <TopBarPortal>
          <Toolbar
            placement="topBar"
            search={
              <SearchBox query={query} onCommit={commitQuery} inputRef={searchRef} />
            }
            view={viewControls(view, changeView, zoom, changeZoom)}
            viewLabel={t.library.view.label}
          />
        </TopBarPortal>
      }
      selectionBar={
        owner &&
        data !== null && (
          <SelectionBar
            count={selection.selection.ids.size}
            allSelected={selection.allSelected}
            selectedIds={selectedIds}
            favoriteVideoIds={favorites.videoIds}
            favoriteGroupIds={favorites.groupIds}
            selectingAll={false}
            onSelectAll={selection.selectAll}
            onClear={selection.clear}
            onTagRemoved={refresh}
            onBundled={afterBulk}
            removeFromGroup={{
              count: removable.length,
              pending: edits.removingSelected,
              onRemove: removeChosen,
            }}
          />
        )
      }
    >
      <GroupBody
        state={state}
        more={more}
        reload={reload}
        loadMore={loadMore}
        shownData={shownData}
        searching={query !== ""}
        id={id}
        listRef={listRef}
        sentinel={sentinel}
        items={{
          view,
          zoom,
          listUrl,
          owner,
          selection,
          selectionMode,
          preview: preview.cardProps,
          onPressTag: pressTag,
          editing:
            owner && query === ""
              ? {
                  removing: edits.removing,
                  onRemove: edits.removeOne,
                  removeRef: edits.removeRef,
                }
              : undefined,
          draggable: owner && query === "",
          nameFieldFor,
        }}
      />
    </ListPage>
  );

  // Include breadcrumbs in the owner drag layer so ancestor crumbs accept drops.
  //
  return (
    <GroupChangedContext value={groupChanged}>
      {owner && query === "" ? (
        <GroupDragLayer
          items={shownData?.items ?? noItems}
          list={view === "list"}
          trail={data?.trail ?? noTrail}
          name={data?.group.name ?? ""}
          selectedItems={removable}
          onReorder={reorderItems}
          onMove={moveItems}
          onMerge={mergeItems}
          onLift={resetPreview}
          dropOnto={dropOnto}
        >
          {page}
        </GroupDragLayer>
      ) : (
        page
      )}
    </GroupChangedContext>
  );
}

function GroupBody({
  state,
  more,
  reload,
  loadMore,
  shownData,
  searching,
  id,
  listRef,
  sentinel,
  items,
}: {
  state: ReturnType<typeof useGroupPage>["state"];
  more: ReturnType<typeof useGroupPage>["more"];
  reload: () => void;
  loadMore: () => void;
  shownData: GroupPageData | null;
  searching: boolean;
  id: number | null;
  listRef: Ref<HTMLDivElement>;
  sentinel: Ref<HTMLDivElement>;
  items: Omit<ComponentProps<typeof GroupItems>, "id" | "data">;
}) {
  return (
    <>
      {state.kind === "loading" && (
        <LoadingState
          label={t.list.loading}
          layout={items.view === "grid" ? "grid" : "table"}
          size={items.zoom}
          count={items.view === "grid" ? skeletonCount : 6}
        />
      )}
      {state.kind === "failed" && (
        <ErrorState
          title={t.groups.detail.loadFailed}
          retryLabel={t.common.retry}
          onRetry={reload}
        />
      )}
      {state.kind === "gone" && (
        <GoneState
          title={t.groups.detail.gone.title}
          description={t.groups.detail.gone.description}
        />
      )}
      {shownData !== null && shownData.items.length === 0 && searching && <NoMatches />}
      {shownData !== null && id !== null && shownData.items.length > 0 && (
        <div ref={listRef}>
          <GroupItems id={id} data={shownData} {...items} />
        </div>
      )}
      {more === "loading" && <LoadMoreRow status="loading" label={t.list.loading} />}
      {more === "failed" && (
        <LoadMoreRow
          status="failed"
          title={t.player.related.moreFailed}
          retryLabel={t.common.retry}
          onRetry={loadMore}
        />
      )}
      <div ref={sentinel} aria-hidden="true" className="h-px" />
    </>
  );
}

function GroupHeader({
  state,
  trail,
  id,
  owner,
  listUrl,
  menu,
  titleRef,
}: {
  state: ReturnType<typeof useGroupPage>["state"];
  trail: readonly number[];
  id: number | null;
  owner: boolean;
  listUrl: string;
  menu: ReactNode;
  titleRef: Ref<HTMLHeadingElement>;
}) {
  if (state.kind === "loading") {
    return <LoadingTrailHeader label={t.groups.trail} crumbs={loadingCrumbs(trail)} />;
  }
  if (state.kind !== "ready" || id === null) {
    // Do not expose names when loading failed or the group disappeared.
    return <UnavailableTrailHeader label={t.groups.trail} />;
  }
  const { group } = state.data;
  const crumbs = trailCrumbs(state.data.trail, group.name);
  return (
    <>
      <Breadcrumbs
        label={t.groups.trail}
        crumbs={crumbs}
        keepFirst
        target={owner ? crumbTarget(crumbs.length - 1) : undefined}
      />
      <PageHeader
        titleRef={titleRef}
        title={
          <span title={group.name} className="line-clamp-2 whitespace-normal break-words">
            {group.name}
          </span>
        }
        count={t.groups.detail.count(group.videoCount, formatDuration(group.durationMs))}
        description={group.folder !== undefined && <FolderLine folder={group.folder} />}
        actions={
          <>
            {group.videoCount > 0 && (
              <PlayLink
                to={videoInGroupPath(group.openVideoId, id)}
                from={listUrl}
                name={group.name}
              />
            )}
            {menu}
          </>
        }
      />
    </>
  );
}

/** Accept Library and ancestor crumbs; reject the current group and collapsed ellipsis. */
function crumbTarget(current: number) {
  return (index: number | "ellipsis", content: ReactNode) => (
    <CrumbDropTarget
      id={crumbKey(index)}
      refused={index === "ellipsis" || index === current}
    >
      {content}
    </CrumbDropTarget>
  );
}

function FolderLine({ folder }: { folder: VideoFolder }) {
  const root = useRootFolderName(folder.rootId, true);
  const path = folderDisplayPath(folder, root.status === "ready" ? root.name : undefined);
  return (
    <span className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
      <Folder aria-hidden="true" className="size-3 shrink-0" />
      <span className="shrink-0">{t.groups.detail.fromFolder}</span>
      <Button variant="link" size="sm" asChild className="h-auto min-w-0 p-0 text-xs">
        <Link to={folderUrl({ rootId: folder.rootId, path: folder.path })}>
          <span className="truncate">{path}</span>
        </Link>
      </Button>
    </span>
  );
}

type Selection = ReturnType<typeof useGroupSelection>;

function GroupItems({
  id,
  data,
  view,
  zoom,
  listUrl,
  owner,
  selection,
  selectionMode,
  preview,
  onPressTag,
  editing,
  draggable,
  nameFieldFor,
}: {
  id: number;
  data: GroupPageData;
  view: ViewMode;
  zoom: Zoom;
  listUrl: string;
  owner: boolean;
  selection: Selection;
  selectionMode: boolean;
  preview: ReturnType<typeof usePreviewCoordination>["cardProps"];
  onPressTag: (tag: TagRef) => void;
  editing?: GroupItemEditing;
  draggable: boolean;
  nameFieldFor?: (groupId: number) => ReactNode;
}) {
  const ids = selection.selection.ids;
  const groupSelected = (group: LibraryGroup) =>
    isGroupSelected(selection.selection, group.id);
  const itemSelected = useCallback(
    (item: GroupItem) =>
      item.group !== undefined
        ? isGroupSelected(selection.selection, item.group.id)
        : item.video !== undefined && selection.selection.ids.has(item.video.id),
    [selection.selection],
  );
  const cell = (item: GroupItem, card: ReactNode) =>
    !draggable ? (
      card
    ) : (
      <DraggableCard key={item.id} id={itemKey(item)}>
        {card}
      </DraggableCard>
    );
  const { videoTags, groupTags } = useCardTagRows({
    selectionMode,
    onPressTag,
    toggleVideo: selection.toggleVideoSelection,
    toggleGroup: selection.toggleGroupSelection,
  });

  if (view === "list") {
    return (
      <GroupItemList
        items={data.items}
        editing={editing}
        row={(item, index) => ({
          item,
          position: index + 1,
          to: groupItemPath(item, data.trail, id),
          backTo: listUrl,
          selected: itemSelected(item),
          selectionMode,
          onSelect: owner ? selection.changeItem : undefined,
          nameField: item.group === undefined ? undefined : nameFieldFor?.(item.group.id),
        })}
      />
    );
  }
  return (
    // Share one tag-row width observer across the list.
    <TagRowMeasureProvider>
      <Grid zoom={zoom}>
        {data.items.map((item) =>
          item.kind === "group" && item.group !== undefined
            ? cell(
                item,
                <GroupCard
                  key={item.id}
                  group={item.group}
                  to={groupItemPath(item, data.trail, id)}
                  backTo={listUrl}
                  selected={groupSelected(item.group)}
                  selectionMode={selectionMode}
                  onSelect={owner ? selection.changeGroup : undefined}
                  tagsRow={groupTags}
                  nameField={nameFieldFor?.(item.group.id)}
                  {...preview}
                />,
              )
            : item.video !== undefined
              ? cell(
                  item,
                  <VideoCard
                    key={item.id}
                    video={item.video}
                    to={groupItemPath(item, data.trail, id)}
                    backTo={listUrl}
                    selected={ids.has(item.video.id)}
                    selectionMode={selectionMode}
                    onSelect={owner ? selection.changeVideo : undefined}
                    tagsRow={videoTags}
                    {...preview}
                  />,
                )
              : null,
        )}
      </Grid>
    </TagRowMeasureProvider>
  );
}
