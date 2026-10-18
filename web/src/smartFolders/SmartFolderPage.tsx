import { FolderSearch, TriangleAlert } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { useLocation, useParams } from "react-router";

import type { LibraryGroup, LibraryItem, TagRef, Video } from "../api/client";
import { itemKey } from "../api/libraryItems";
import { useAudience } from "../auth/audience";
import Breadcrumbs from "../folders/Breadcrumbs";
import { loadingCrumbs } from "../groups/groupPaths";
import { useFocusAfterEdit } from "../groups/useGroupEdits";
import { useLeavePage } from "../groups/useLeaveGroup";
import { t } from "../i18n";
import { formatDuration } from "../lib/format";
import { GroupCard, GroupRow } from "../library/GroupCard";
import { LibraryTableHeader } from "../library/LibraryPage";
import { viewControls } from "../library/LibraryToolbar";
import {
  emptySelection,
  isGroupSelected,
  keepMembers,
  type LibrarySelection,
  reconcileGroups,
  sameSelection,
  selectedGroupOf,
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
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { Toolbar } from "../ui/patterns/toolbar";
import { TableBody } from "../ui/shadcn/table";
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
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import VideoCard, { VideoRow } from "../videoList/VideoCard";
import { SmartFolderCardsProvider } from "./cardValues";
import ConditionLines from "./ConditionLines";
import { type AncestorRule, bodyKind, missingParent, selectAllLoaded } from "./pageRules";
import { SmartFolderCard, SmartFolderRow } from "./SmartFolderCard";
import SmartFolderMenu, { type SmartFolderMenuDialog } from "./SmartFolderMenu";
import {
  parseSmartFolderId,
  smartFolderCrumbs,
  smartFolderParentPath,
  videoInSmartFolderPath,
} from "./smartFolderPaths";
import {
  type SmartFolderPageData,
  type SmartFolderPageState,
  useAncestorRules,
  useSmartFolderPage,
} from "./useSmartFolderPage";

const skeletonCount = 12;

/** Keep the guest empty-missing-reference value stable across renders. */
const noMissing: readonly never[] = [];

/** Key by route ID so React Router cannot retain another folder's names, items or selection. */
export default function SmartFolderPage() {
  const params = useParams();
  const id = parseSmartFolderId(params.id);
  return <SmartFolderScreen key={params.id ?? ""} id={id} />;
}

/**
 * Select only loaded videos and groups; exclude child folders. Remove unmatched videos on refresh
 * so bulk actions cannot affect hidden members.
 */
function useFolderSelection(items: readonly LibraryItem[]) {
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);
  const all = useMemo(() => selectAllLoaded(items), [items]);
  useEffect(() => {
    setSelection((current) => reconcileGroups(keepMembers(current, all.ids), items));
  }, [all, items]);
  const clear = useCallback(() => setSelection(emptySelection), []);
  useEscapeClearsSelection(selection.ids.size > 0, clear);
  return {
    selection,
    allSelected: all.ids.size > 0 && sameSelection(all, selection),
    clear,
    selectAll: useCallback(() => setSelection(all), [all]),
    changeVideo: useCallback(
      (videoId: number, selected: boolean) =>
        setSelection((current) => setVideo(current, videoId, selected)),
      [],
    ),
    changeGroup: useCallback(
      (group: LibraryGroup, selected: boolean) =>
        setSelection((current) => setGroup(current, selectedGroupOf(group), selected)),
      [],
    ),
    toggleVideo: useCallback(
      (videoId: number) => setSelection((current) => toggleVideo(current, videoId)),
      [],
    ),
    toggleGroup: useCallback(
      (group: LibraryGroup) =>
        setSelection((current) => toggleGroup(current, selectedGroupOf(group))),
      [],
    ),
  };
}

type FolderSelection = ReturnType<typeof useFolderSelection>;

/** Keep the empty items identity stable across renders. */
const noItems: readonly LibraryItem[] = [];

function SmartFolderScreen({ id }: { id: number | null }) {
  const location = useLocation();
  const owner = useAudience() === "owner";
  const { state, more, reload, refresh, loadMore, markGone } = useSmartFolderPage(id);
  const data = state.kind === "ready" ? state.data : null;
  // Reload ancestor rules only on folder refresh, never when adding a page.
  const ancestors = useAncestorRules(data?.trail ?? noTrail, owner, data?.folder);
  const { menu, editConditions } = useFolderMenu(data, owner, refresh, markGone);
  const preview = usePreviewCoordination();
  const { resetPreview } = preview;
  const { view, zoom, changeView, changeZoom, listRef } = useViewControls(resetPreview);
  const selection = useFolderSelection(data?.items ?? noItems);
  const listUrl = `${location.pathname}${location.search}`;

  useDocumentTitle(data?.folder.name);
  useEffect(() => {
    resetPreview();
  }, [data, resetPreview, view, zoom]);

  const sentinel = useLoadMoreSentinel(
    data?.nextCursor !== undefined && more === "idle",
    loadMore,
    resetPreview,
  );
  const pressTag = usePressTagToLibrary();
  const { favorites, selectedIds } = useSelectionTargets(selection.selection);
  const { clear } = selection;
  const afterBulk = useCallback(() => {
    clear();
    reload();
  }, [clear, reload]);

  return (
    <SmartFolderCardsProvider>
      <ListPage
        header={
          <SmartFolderHeader
            state={state}
            listUrl={listUrl}
            owner={owner}
            ancestors={ancestors}
            menu={menu}
          />
        }
        toolbar={
          <TopBarPortal>
            <Toolbar
              placement="topBar"
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
              onTagRemoved={reload}
              onBundled={afterBulk}
            />
          )
        }
      >
        {state.kind === "loading" && (
          <LoadingState
            label={t.list.loading}
            layout={view === "grid" ? "grid" : "table"}
            size={zoom}
            count={view === "grid" ? skeletonCount : 6}
          />
        )}
        {state.kind === "failed" && (
          <ErrorState
            title={t.smartFolders.detail.loadFailed}
            retryLabel={t.common.retry}
            onRetry={reload}
          />
        )}
        {state.kind === "gone" && (
          <GoneState
            title={t.smartFolders.detail.gone.title}
            description={t.smartFolders.detail.gone.description}
          />
        )}
        {data !== null && id !== null && (
          <div ref={listRef} className="flex flex-col gap-6">
            {data.items.length > 0 && (
              <FolderItems
                id={id}
                items={data.items}
                view={view}
                zoom={zoom}
                listUrl={listUrl}
                owner={owner}
                selection={selection}
                preview={preview.cardProps}
                onPressTag={pressTag}
              />
            )}
            <EmptyBody data={data} ancestors={ancestors} onEdit={editConditions} />
          </div>
        )}
        {more === "loading" && <LoadMoreRow status="loading" label={t.list.loading} />}
        {more === "failed" && (
          <LoadMoreRow
            status="failed"
            title={t.smartFolders.detail.moreFailed}
            retryLabel={t.common.retry}
            onRetry={loadMore}
          />
        )}
        <div ref={sentinel} aria-hidden="true" className="h-px" />
      </ListPage>
    </SmartFolderCardsProvider>
  );
}

/**
 * Share dialog state with the empty-state action; refresh after edits and creation, restoring
 * focus to More. Deletion returns to the parent or library; missing edited folders become not-
 * found.
 */
function useFolderMenu(
  data: SmartFolderPageData | null,
  owner: boolean,
  refresh: () => void,
  markGone: () => void,
) {
  const [dialog, setDialog] = useState<SmartFolderMenuDialog>(null);
  const leave = useLeavePage(smartFolderParentPath(data?.trail ?? noTrail));
  const name = data?.folder.name ?? "";
  const deleted = useCallback(
    (announce: boolean) => leave(announce ? t.smartFolders.deleted(name) : undefined),
    [leave, name],
  );
  const editConditions = useCallback(() => setDialog("edit"), []);
  const { moreRef, focus } = useFocusAfterEdit();
  const saved = useCallback(() => {
    refresh();
    focus("more");
  }, [focus, refresh]);
  if (!owner || data?.settings === undefined) {
    return { menu: null, editConditions: undefined };
  }
  const menu = (
    <SmartFolderMenu
      folder={{ id: data.folder.id, name, parentId: data.trail.at(-1)?.id }}
      settings={data.settings}
      dialog={dialog}
      onDialogChange={setDialog}
      moreRef={moreRef}
      onSaved={saved}
      onCreatedChild={refresh}
      onDeleted={deleted}
      onGone={markGone}
    />
  );
  return { menu, editConditions };
}

const noTrail: SmartFolderPageData["trail"] = [];

/** Missing ancestor references add their ancestor name; show Edit conditions only to owners. */
function EmptyBody({
  data,
  ancestors,
  onEdit,
}: {
  data: SmartFolderPageData;
  ancestors: readonly AncestorRule[];
  onEdit?: () => void;
}) {
  const missing = data.settings?.missing ?? noMissing;
  const kind = bodyKind(data.items, data.nextCursor, missing);
  if (kind === "content") return null;
  const action = onEdit !== undefined && (
    <Button variant="outline" size="sm" onClick={onEdit}>
      {t.smartFolders.detail.editConditions}
    </Button>
  );
  if (kind === "empty") {
    return (
      <EmptyState
        icon={<FolderSearch aria-hidden="true" />}
        title={t.smartFolders.detail.empty.title}
        description={t.smartFolders.detail.empty.description}
        action={action}
      />
    );
  }
  const parent = missingParent(data.settings?.rule, ancestors, missing);
  return (
    <EmptyState
      icon={<TriangleAlert aria-hidden="true" />}
      title={t.smartFolders.detail.missing.title}
      description={
        parent === undefined
          ? t.smartFolders.detail.missing.description
          : `${t.smartFolders.detail.missing.description} ${t.smartFolders.detail.missing.inParent(parent.name)}`
      }
      action={action}
    />
  );
}

/** Hide names on failure or not-found, when their contents cannot be shown. */
function SmartFolderHeader({
  state,
  listUrl,
  owner,
  ancestors,
  menu,
}: {
  state: SmartFolderPageState;
  listUrl: string;
  owner: boolean;
  ancestors: readonly AncestorRule[];
  menu: ReactNode;
}) {
  if (state.kind === "loading") {
    return <LoadingTrailHeader label={t.smartFolders.trail} crumbs={loadingCrumbs([])} />;
  }
  if (state.kind !== "ready") {
    return <UnavailableTrailHeader label={t.smartFolders.trail} />;
  }
  const { folder, settings, trail } = state.data;
  return (
    <>
      <Breadcrumbs
        label={t.smartFolders.trail}
        crumbs={smartFolderCrumbs(trail, folder.name)}
        keepFirst
      />
      <PageHeader
        title={
          <span
            title={folder.name}
            className="line-clamp-2 whitespace-normal break-words"
          >
            {folder.name}
          </span>
        }
        count={t.groups.detail.count(
          folder.videoCount,
          formatDuration(folder.durationMs),
        )}
        description={
          owner &&
          settings !== undefined && (
            <ConditionLines
              rule={settings.rule}
              ancestors={ancestors}
              missing={settings.missing}
            />
          )
        }
        actions={
          <>
            {folder.openVideoId !== undefined && (
              <PlayLink
                to={videoInSmartFolderPath(folder.openVideoId, folder.id)}
                from={listUrl}
                name={folder.name}
              />
            )}
            {menu}
          </>
        }
      />
    </>
  );
}

interface ItemsProps {
  id: number;
  items: readonly LibraryItem[];
  view: ViewMode;
  zoom: Zoom;
  listUrl: string;
  owner: boolean;
  selection: FolderSelection;
  preview: ReturnType<typeof usePreviewCoordination>["cardProps"];
  onPressTag: (tag: TagRef) => void;
}

function FolderItems(props: ItemsProps) {
  const { id, items, view, zoom, listUrl, owner, selection, preview } = props;
  const selectionMode = selection.selection.ids.size > 0;
  const { videoTags, groupTags, folderTags } = useCardTagRows({
    selectionMode,
    onPressTag: props.onPressTag,
    toggleVideo: selection.toggleVideo,
    toggleGroup: selection.toggleGroup,
  });
  const selected = selection.selection;
  const common = { backTo: listUrl, selectionMode, ...preview };
  const videoProps = (video: Video) => ({
    ...common,
    video,
    to: videoInSmartFolderPath(video.id, id),
    selected: selected.ids.has(video.id),
    onSelect: owner ? selection.changeVideo : undefined,
    tagsRow: view === "grid" ? videoTags : undefined,
  });
  const groupProps = (group: LibraryGroup) => ({
    ...common,
    group,
    selected: isGroupSelected(selected, group.id),
    onSelect: owner ? selection.changeGroup : undefined,
    tagsRow: view === "grid" ? groupTags : undefined,
  });
  const folderProps = (item: Extract<LibraryItem, { kind: "smartFolder" }>) => ({
    ...preview,
    folder: item.smartFolder,
    backTo: listUrl,
    tagsRow: view === "grid" ? folderTags : undefined,
  });

  const cell = (item: LibraryItem): ReactNode => {
    const key = itemKey(item);
    if (item.kind === "smartFolder") {
      return view === "grid" ? (
        <SmartFolderCard key={key} {...folderProps(item)} />
      ) : (
        <SmartFolderRow key={key} {...folderProps(item)} owner={owner} />
      );
    }
    if (item.kind === "video") {
      const Card = view === "grid" ? VideoCard : VideoRow;
      return <Card key={key} {...videoProps(item.video)} />;
    }
    const Card = view === "grid" ? GroupCard : GroupRow;
    return <Card key={key} {...groupProps(item.group)} />;
  };

  if (view === "list") {
    return (
      <DataTable label={t.groups.detail.table}>
        <LibraryTableHeader owner={owner} />
        <TableBody>{items.map(cell)}</TableBody>
      </DataTable>
    );
  }
  return (
    // Share one tag-row width observer across the list.
    <TagRowMeasureProvider>
      <Grid zoom={zoom}>{items.map(cell)}</Grid>
    </TagRowMeasureProvider>
  );
}
