import { Folder } from "lucide-react";
import {
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

/** noTrail は読めていない間の道筋（ライブラリへ移る）。描画ごとに作らない。 */
const noTrail: readonly GroupRef[] = [];

/** noItems は読めていない間の項目。描画ごとに作らない。 */
const noItems: readonly GroupItem[] = [];

/** itemVideo は項目が動画ならその動画である（ドラッグの落とし方を決める）。 */
function itemVideo(item: GroupItem): Video | undefined {
  return item.kind === "video" ? item.video : undefined;
}

/** itemGroup は項目が子グループならそのグループである。 */
function itemGroup(item: GroupItem): LibraryGroup | undefined {
  return item.kind === "group" ? item.group : undefined;
}

/** newGroupItem は「Make a group」の新しいグループを、仮の項目 `itemId` として置く。 */
function newGroupItem(group: LibraryGroup, itemId: number): GroupItem {
  return { id: itemId, kind: "group", group };
}

/**
 * GroupPage はグループ 1 つを開いた画面（`/groups/{id}?trail=`）である
 * （specs/048-groups/ui-design.md「Group page」）。上のバーの下に道筋の帯、見出しにグループの
 * 名前と本数と「Play」、本体にその中の子グループと動画を並び順どおりにライブラリのカードか
 * 行で並べる。所有者とゲストの両方が開く。
 *
 * React Router は `:id` や問い合わせだけが変わる移動で同じ部品を使い回すので、画面は経路の
 * id と道筋ごとに作り直す。前のグループの名前・項目・選択が、別のグループと組み合わさって
 * 残らない。
 */
export default function GroupPage() {
  const params = useParams();
  const location = useLocation();
  const trail = parseTrail(location.search);
  const id = parseGroupId(params.id);
  return (
    <GroupScreen key={`${params.id ?? ""}?${trail.join(",")}`} id={id} trail={trail} />
  );
}

/**
 * useGroupPageDrops は動画を動画に落としたときの「Make a group」と「Bundle as versions」である。
 * 新しいグループは落とした先の項目の位置を取る（specs/050-drag-organize/ui-design.md
 * 「Group page」、R-6）。`shownData` はサーバーが答える前の結果を重ねた中身である。
 */
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

/**
 * useGroupSelection はグループのページの選択である。ライブラリと同じく、動画の id の集合と、
 * グループとして選んだ子グループを持つ（ui-design.md「Body」）。
 */
function useGroupSelection(data: GroupPageData | null) {
  const [selection, setSelection] = useState<LibrarySelection>(emptySelection);

  // 読み直したグループにもう無い動画と子グループを選択から外し、選んだ子グループを今の
  // メンバーで覚え直す（ui-design.md「Edits that fail」）。読んでいないページの選んだ動画は残す。
  useEffect(() => {
    if (data === null) return;
    setSelection((current) =>
      reconcileGroupSelection(current, data.group.videoIds, data.items),
    );
  }, [data]);

  // 「Select all」が選ぶものは今のグループから決める。読み直しで動画や子グループが増えれば
  // 選択と一致しなくなり、ボタンはまた押せる。
  const all = useMemo(
    () => (data === null ? null : selectAllInGroup(data.group.videoIds, data.items)),
    [data],
  );

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

/** GroupScreen は経路の id と道筋 1 つのグループのページである。 */
function GroupScreen({ id, trail }: { id: number | null; trail: readonly number[] }) {
  const location = useLocation();
  const owner = useAudience() === "owner";
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
  } = useGroupPage(id, trail);
  const data = state.kind === "ready" ? state.data : null;
  // 解散、タグ化、最後の項目を外したときは、道筋の親かライブラリへ移ってトーストを出す
  // （ui-design.md「Dissolving」「Removing one item」）。
  const leave = useLeaveGroup(data?.trail ?? noTrail);
  const name = data?.group.name;
  const dissolvedByRemoval = useCallback(() => {
    if (name !== undefined) leave(t.groups.dissolved(name));
  }, [leave, name]);
  // 取り消しの通知は画面を離れると閉じる。取り消したら静かに読み直す
  // （specs/050-drag-organize/ui-design.md「Undo toast」）。
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
  const selection = useGroupSelection(data);
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
  // カードのタグを押すと、そのタグで絞ったライブラリを開く（ui-design.md「Body」）。
  const pressTag = usePressTagToLibrary();

  const selectionMode = selection.selection.ids.size > 0;
  const { favorites, selectedIds } = useSelectionTargets(selection.selection);

  const { clear } = selection;

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

  // 「Remove from group」で外す項目（ui-design.md「Body」）。
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
  // 子グループ・祖先・ライブラリへ移したかマージした項目は画面から消えるので、選択を解く
  // （specs/050-drag-organize/ui-design.md「Group page」）。
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

  // 足した先がこのグループのときだけ、項目が末尾に増えるので読み直す。新しく作ったグループは
  // いちばん上のグループで、このグループの項目は変わらない（ui-design.md「Add to group」の
  // Result、「New group」）。
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
      {shownData !== null && id !== null && shownData.items.length > 0 && (
        <div ref={listRef}>
          <GroupItems
            id={id}
            data={shownData}
            view={view}
            zoom={zoom}
            listUrl={listUrl}
            owner={owner}
            selection={selection}
            selectionMode={selectionMode}
            preview={preview.cardProps}
            onPressTag={pressTag}
            editing={
              owner
                ? {
                    removing: edits.removing,
                    onRemove: edits.removeOne,
                    removeRef: edits.removeRef,
                  }
                : undefined
            }
            draggable={owner}
            nameFieldFor={nameFieldFor}
          />
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
    </ListPage>
  );

  // 所有者の画面は道筋の帯から本体までをドラッグの層で包む。帯の段も落とす先になる
  // （specs/050-drag-organize/ui-design.md「Group page」）。
  return (
    <GroupChangedContext value={groupChanged}>
      {owner ? (
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

/**
 * GroupHeader は道筋の帯と見出しである（ui-design.md「Trail band」「Header」「States」）。帯は
 * フォルダ画面の Breadcrumbs を共有し、上のバーの下に貼り付く。
 */
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
  /** 所有者なら道筋の段を落とす先にする。 */
  owner: boolean;
  listUrl: string;
  /** 所有者の「More」。ゲストには無い。 */
  menu: ReactNode;
  titleRef: Ref<HTMLHeadingElement>;
}) {
  if (state.kind === "loading") {
    return <LoadingTrailHeader label={t.groups.trail} crumbs={loadingCrumbs(trail)} />;
  }
  if (state.kind !== "ready" || id === null) {
    // 読めなかったときと無くなったときは、見せられないものの名前を出さない。
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
        // 長い名前は見出しの中で 2 行まで折り返し、全体は title に入れる。
        title={
          <span title={group.name} className="line-clamp-2 whitespace-normal break-words">
            {group.name}
          </span>
        }
        count={t.groups.detail.count(group.videoCount, formatDuration(group.durationMs))}
        description={group.folder !== undefined && <FolderLine folder={group.folder} />}
        // 「Play」を先に、所有者の「More」を後に置く。項目の無い応答には「Play」を出さない
        // （最後の項目を外したグループは解散されるので、所有者の画面には普段出ない）。
        actions={
          <>
            {state.data.items.length > 0 && (
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

/**
 * crumbTarget は所有者の道筋の段を落とす先にする口である（ui-design.md「Indicators」）。
 * 「Library」と祖先の段は受け、開いているグループの段 `current` と畳んだ「…」は拒否する。
 */
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

/**
 * FolderLine は自動グループの見出しの下の 1 行「From the folder {path}」である。パスはその
 * フォルダのページへのリンクで、手で作ったグループには出さない（ui-design.md「Header」）。
 */
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

/**
 * GroupItems はグループの項目を並び順どおりに、格子ならライブラリのカード、リストなら
 * プレイリストの画面の行で並べる（ui-design.md「Body」）。動画は `?group` 付きで、子グループは
 * 道筋を 1 段伸ばして開く。
 */
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
  /** 所有者のリスト表示の `×`。ゲストには無い。 */
  editing?: GroupItemEditing;
  /** 所有者ならカードを持ち上げられる（GroupDragLayer の中で描く）。ゲストには無い。 */
  draggable: boolean;
  /** 作ったばかりのグループの名前の欄（useVideoDrops）。所有者だけが渡す。 */
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
    // タグの行の幅の見張りは一覧に1つだけ（ライブラリと同じ）。
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
