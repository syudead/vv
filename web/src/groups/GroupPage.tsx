import { Folder, FolderX, Play } from "lucide-react";
import {
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";

import type { LibraryGroup, TagRef, Video, VideoFolder } from "../api/client";
import type { GroupItem, GroupRef } from "../api/groups";
import { useRootFolderName } from "../api/useFolderListing";
import { useAudience } from "../auth/audience";
import Breadcrumbs from "../folders/Breadcrumbs";
import { folderUrl } from "../folders/folderPath";
import { t } from "../i18n";
import { formatDuration } from "../lib/format";
import CardTagRow from "../library/CardTagRow";
import { GroupCard } from "../library/GroupCard";
import { viewControls } from "../library/LibraryToolbar";
import {
  emptySelection,
  favoriteTargets,
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
import { TAG_PARAM } from "../library/tagCriteria";
import { TagRowMeasureProvider } from "../library/TagRowMeasure";
import { useEscapeClearsSelection } from "../library/useEscapeClearsSelection";
import {
  readViewPreferences,
  type ViewMode,
  type ViewPreferences,
  writeViewPreferences,
  type Zoom,
} from "../preferences/viewPreferences";
import TopBarPortal from "../shell/TopBarPortal";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Toolbar } from "../ui/patterns/toolbar";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Grid } from "../videoList/Grid";
import { usePreviewCoordination } from "../videoList/usePreviewCoordination";
import { useZoomAnchor } from "../videoList/useZoomAnchor";
import VideoCard from "../videoList/VideoCard";
import { GroupChangedContext } from "./groupChanges";
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

/** useDocumentTitle は名前が分かっている間だけ、窓の題をその名前にする。 */
function useDocumentTitle(name: string | undefined) {
  useEffect(() => {
    if (name === undefined) return;
    const previous = document.title;
    document.title = name;
    return () => {
      document.title = previous;
    };
  }, [name]);
}

/** useViewPreferences はライブラリと共有する表示形式と大きさである。 */
function useViewPreferences() {
  const [preferences, setPreferences] = useState(readViewPreferences);
  const save = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);
  return { preferences, save };
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

/** useSentinel は番兵が見える範囲の 600px 以内に来たら `onNear` を呼ぶ（ライブラリと同じ）。 */
function useSentinel(enabled: boolean, onNear: () => void) {
  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !enabled) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onNear();
      },
      { rootMargin: "600px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [enabled, onNear]);
  return sentinel;
}

/** GroupScreen は経路の id と道筋 1 つのグループのページである。 */
function GroupScreen({ id, trail }: { id: number | null; trail: readonly number[] }) {
  const location = useLocation();
  const navigate = useNavigate();
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
  const edits = useGroupEdits({
    id,
    update,
    confirmed,
    moves,
    movesSent,
    refresh,
    onDissolved: dissolvedByRemoval,
  });
  const { preferences, save } = useViewPreferences();
  const { view, zoom } = preferences;
  const preview = usePreviewCoordination();
  const { resetPreview } = preview;
  const { listRef, capture } = useZoomAnchor(zoom);
  const selection = useGroupSelection(data);
  const listUrl = `${location.pathname}${location.search}`;

  useDocumentTitle(data?.group.name);
  useEffect(() => {
    resetPreview();
  }, [data, resetPreview, view, zoom]);

  const changeView = useCallback(
    (next: ViewMode) => {
      resetPreview();
      save({ ...preferences, view: next });
    },
    [preferences, resetPreview, save],
  );
  const changeZoom = useCallback(
    (next: Zoom) => {
      capture();
      resetPreview();
      save({ ...preferences, zoom: next });
    },
    [capture, preferences, resetPreview, save],
  );

  const near = useCallback(() => {
    resetPreview();
    loadMore();
  }, [loadMore, resetPreview]);
  const sentinel = useSentinel(data?.nextCursor !== undefined && more === "idle", near);

  // カードのタグを押すと、そのタグで絞ったライブラリを開く。グループのページは自分の並びを
  // 見せる画面で、絞り込みを持たない（ui-design.md「Body」）。
  const pressTag = useCallback(
    (tag: TagRef) => void navigate(`/?${TAG_PARAM}=${String(tag.id)}`),
    [navigate],
  );

  const selectionMode = selection.selection.ids.size > 0;
  const favorites = useMemo(
    () => favoriteTargets(selection.selection),
    [selection.selection],
  );
  const selectedIds = useMemo(
    () => Array.from(selection.selection.ids),
    [selection.selection.ids],
  );

  const { clear } = selection;
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

  const { focus } = edits;
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

  return (
    <GroupChangedContext value={groupChanged}>
      <ListPage
        header={
          <GroupHeader
            state={state}
            trail={trail}
            id={id}
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
        {state.kind === "gone" && <GoneState />}
        {data !== null && id !== null && data.items.length > 0 && (
          <div ref={listRef}>
            <GroupItems
              id={id}
              data={data}
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
                      onMove: edits.move,
                      onRemove: edits.removeOne,
                      removeRef: edits.removeRef,
                    }
                  : undefined
              }
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
    </GroupChangedContext>
  );
}

/** GoneState はグループがもう無いとき（404）の本体である。 */
function GoneState() {
  return (
    <EmptyState
      icon={<FolderX aria-hidden="true" />}
      title={t.groups.detail.gone.title}
      description={t.groups.detail.gone.description}
      action={
        <Button size="sm" asChild>
          <Link to="/">{t.groups.detail.gone.action}</Link>
        </Button>
      }
    />
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
  listUrl,
  menu,
  titleRef,
}: {
  state: ReturnType<typeof useGroupPage>["state"];
  trail: readonly number[];
  id: number | null;
  listUrl: string;
  /** 所有者の「More」。ゲストには無い。 */
  menu: ReactNode;
  titleRef: Ref<HTMLHeadingElement>;
}) {
  if (state.kind === "loading") {
    return (
      <>
        <Breadcrumbs label={t.groups.trail} crumbs={loadingCrumbs(trail)} />
        <PageHeader
          title={
            <>
              <span className="sr-only">{t.list.loading}</span>
              <Skeleton aria-hidden="true" className="h-6 w-1/2" />
            </>
          }
        />
      </>
    );
  }
  if (state.kind !== "ready" || id === null) {
    // 読めなかったときと無くなったときは、見せられないものの名前を出さない。
    return (
      <>
        <Breadcrumbs label={t.groups.trail} crumbs={[{ label: t.library.title }]} />
        <PageHeader title={t.library.title} />
      </>
    );
  }
  const { group } = state.data;
  return (
    <>
      <Breadcrumbs
        label={t.groups.trail}
        crumbs={trailCrumbs(state.data.trail, group.name)}
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
              <Button size="sm" asChild>
                <Link
                  to={videoInGroupPath(group.openVideoId, id)}
                  state={{ from: listUrl, autoplay: true }}
                  aria-label={t.groups.detail.playFor(group.name)}
                >
                  <Play aria-hidden="true" />
                  {t.groups.detail.play}
                </Link>
              </Button>
            )}
            {menu}
          </>
        }
      />
    </>
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
  /** 所有者のリスト表示の並べ替えと `×`。ゲストには無い。 */
  editing?: GroupItemEditing;
}) {
  const ids = selection.selection.ids;
  const groupSelected = (group: LibraryGroup) =>
    isGroupSelected(selection.selection, group.id);
  const { toggleVideoSelection, toggleGroupSelection } = selection;

  const videoTags = useCallback(
    (video: Video) => (
      <CardTagRow
        tags={video.tags}
        selectionMode={selectionMode}
        onPress={onPressTag}
        onToggleSelection={() => toggleVideoSelection(video.id)}
      />
    ),
    [onPressTag, selectionMode, toggleVideoSelection],
  );
  const groupTags = useCallback(
    (group: LibraryGroup) => (
      <CardTagRow
        tags={group.tags}
        selectionMode={selectionMode}
        onPress={onPressTag}
        onToggleSelection={() => toggleGroupSelection(group)}
      />
    ),
    [onPressTag, selectionMode, toggleGroupSelection],
  );

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
          selected:
            item.group !== undefined
              ? groupSelected(item.group)
              : item.video !== undefined && ids.has(item.video.id),
          selectionMode,
          onSelect: owner ? selection.changeItem : undefined,
        })}
      />
    );
  }
  return (
    // タグの行の幅の見張りは一覧に1つだけ（ライブラリと同じ）。
    <TagRowMeasureProvider>
      <Grid zoom={zoom}>
        {data.items.map((item) =>
          item.kind === "group" && item.group !== undefined ? (
            <GroupCard
              key={item.id}
              group={item.group}
              to={groupItemPath(item, data.trail, id)}
              backTo={listUrl}
              selected={groupSelected(item.group)}
              selectionMode={selectionMode}
              onSelect={owner ? selection.changeGroup : undefined}
              tagsRow={groupTags}
              {...preview}
            />
          ) : item.video !== undefined ? (
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
            />
          ) : null,
        )}
      </Grid>
    </TagRowMeasureProvider>
  );
}
