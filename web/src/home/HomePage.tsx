import { Film, FolderSearch } from "lucide-react";
import { type KeyboardEvent, useRef } from "react";
import { Link, useLocation, useNavigate } from "react-router";

import type { LibraryItem } from "../api/client";
import type { HomeShelf } from "../api/home";
import { ONBOARDING_PATH } from "../api/onboarding";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { GroupCard } from "../library/GroupCard";
import { LIBRARY_PATH } from "../library/libraryPath";
import { isComposingNativeKeyEvent } from "../lib/ime";
import TopBarPortal from "../shell/TopBarPortal";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { ShelfRow, ShelfRows } from "../ui/patterns/shelf-row";
import { Toolbar } from "../ui/patterns/toolbar";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { normalizeQuery } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { EmptyState } from "../videoList/states";
import {
  usePreviewCoordination,
  type PreviewCardProps,
} from "../videoList/usePreviewCoordination";
import VideoCard from "../videoList/VideoCard";
import AddShelfButton from "./AddShelfButton";
import {
  type DrawnShelf,
  drawnShelves,
  organizingLineIndex,
  seeAllPath,
  shelfHeadingPath,
  shelfItemPath,
  type ShelfStates,
  shelfTitle,
} from "./homeShelves";
import ShelfMenu from "./ShelfMenu";
import { folderShelfIds, shelfMoves } from "./shelfOrder";
import { type HomeList, useHome } from "./useHome";
import { useInView } from "./useInView";
import { useShelfWrites } from "./useShelfWrites";

/**
 * HomePage は `/` のホームである（specs/057-home/ui-design.md「Home」）。棚を上から 1 行ずつ
 * 並べ、カードを 1 回押すと再生する。トップバーには検索欄だけを置き、送るとライブラリで
 * 探す。
 */
export default function HomePage() {
  const home = useHome();
  const owner = useAudience() === "owner";
  const writes = useShelfWrites(home.reload);
  const order =
    owner && home.list.status === "ready"
      ? folderShelfIds(home.list.home.shelves)
      : undefined;
  const editing = order === undefined ? undefined : { order, writes };
  return (
    <ListPage
      header={
        <PageHeader
          title={t.home.title}
          titleHidden
          actions={
            order !== undefined && (
              <AddShelfButton
                onHome={order}
                disabled={writes.busy}
                onPick={(folder) => void writes.add(folder, order)}
              />
            )
          }
        />
      }
      toolbar={
        <TopBarPortal>
          <Toolbar placement="topBar" search={<HomeSearch />} />
        </TopBarPortal>
      }
    >
      <HomeBody {...home} editing={editing} />
    </ListPage>
  );
}

/**
 * HomeSearch はトップバーの検索欄である。入力してもホームは絞らず、Enter で
 * `/library?q=` を開く（ui-design.md「Why this shape」）。
 */
function HomeSearch() {
  const navigate = useNavigate();
  const field = useRef<HTMLInputElement | null>(null);
  const submit = (event: KeyboardEvent<HTMLDivElement>) => {
    // 送るのは入力欄での Enter だけである。クリアのボタンや検索の書き方の案内で押した Enter は
    // そのボタンの操作に任せる。
    if (event.target !== field.current) return;
    if (event.key !== "Enter" || isComposingNativeKeyEvent(event.nativeEvent)) return;
    const query = normalizeQuery(field.current?.value ?? "");
    if (query === "") return;
    event.preventDefault();
    navigate(`${LIBRARY_PATH}?${new URLSearchParams({ q: query }).toString()}`);
  };
  return (
    <div role="search" className="w-full" onKeyDown={submit}>
      <SearchBox
        query=""
        onCommit={ignoreCommit}
        inputRef={field}
        label={t.home.search}
        placeholder={t.home.search}
      />
    </div>
  );
}

/** ignoreCommit は検索欄の打鍵の確定を捨てる。ホームは Enter で送るまで何もしない。 */
function ignoreCommit() {}

/** ShelfEditing は所有者の棚の操作である。見えているフォルダの棚の並びと、その書き込み。 */
interface ShelfEditing {
  order: readonly number[];
  writes: ReturnType<typeof useShelfWrites>;
}

interface HomeBodyProps {
  list: HomeList;
  /** 所有者だけが受け取る。 */
  editing: ShelfEditing | undefined;
  shelves: ShelfStates;
  loadMore: () => void;
  retryList: () => void;
  retryShelf: (key: string) => void;
}

/** HomeBody は本体である。一覧の状態ごとに、棚の行か状態のブロックを 1 つ出す。 */
function HomeBody({
  list,
  editing,
  shelves,
  loadMore,
  retryList,
  retryShelf,
}: HomeBodyProps) {
  const owner = useAudience() === "owner";
  const preview = usePreviewCoordination();
  const frontier = useInView(loadMore);
  if (list.status === "loading") {
    return <LoadingState label={t.home.loading} layout="shelves" />;
  }
  if (list.status === "failed") {
    return (
      <ErrorState
        title={<h2>{t.home.loadFailed(list.reason)}</h2>}
        retryLabel={t.common.retry}
        onRetry={retryList}
      />
    );
  }
  if (list.home.empty === "noVideos") return <EmptyHome owner={owner} />;
  const drawn = drawnShelves(list.home.shelves, shelves);
  const rows = drawn.map((entry) => (
    <HomeShelfRow
      key={entry.shelf.key}
      drawn={entry}
      frontierRef={entry.frontier ? frontier : undefined}
      preview={preview.cardProps}
      onRetry={retryShelf}
      editing={editing}
    />
  ));
  if (editing !== undefined && list.home.organizer?.running === true) {
    rows.splice(organizingLineIndex(drawn), 0, <OrganizingLine key="organizing" />);
  }
  return <ShelfRows label={t.home.shelves}>{rows}</ShelfRows>;
}

/** OrganizingLine は整理役が走っている間、所有者に最後のフォルダの棚の下で見せる行である。 */
function OrganizingLine() {
  return (
    <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
      <Spinner aria-hidden="true" role="presentation" className="size-3" />
      {t.home.organizing}
    </p>
  );
}

/** FolderShelfMenu はフォルダの棚の所有者の「More」である。並びは見えている並びから作る。 */
function FolderShelfMenu({ shelf, editing }: { shelf: HomeShelf; editing: ShelfEditing }) {
  const id = shelf.folderId;
  if (id === undefined) return null;
  const { order, writes } = editing;
  return (
    <ShelfMenu
      name={shelf.name}
      moves={shelfMoves(order, id)}
      disabled={writes.busy}
      onMove={(direction) => void writes.move(id, direction, order)}
      onRemove={() => void writes.remove({ id, name: shelf.name }, order)}
    />
  );
}

/** EmptyHome は見る人に見える動画が 1 本も無いときの本体である。所有者には取り込みへの入口を置く。 */
function EmptyHome({ owner }: { owner: boolean }) {
  return (
    <EmptyState
      icon={Film}
      title={t.home.empty.title}
      description={owner ? t.home.empty.owner : t.home.empty.guest}
      action={
        owner && (
          <Button asChild size="sm">
            <Link to={ONBOARDING_PATH}>{t.home.empty.setUp}</Link>
          </Button>
        )
      }
    />
  );
}

/** HomeShelfRow は棚 1 つの行である。見出し、「See all」、カードか状態を出す。 */
function HomeShelfRow({
  drawn,
  frontierRef,
  preview,
  onRetry,
  editing,
}: {
  drawn: DrawnShelf;
  frontierRef: ((element: HTMLElement | null) => void) | undefined;
  preview: PreviewCardProps;
  onRetry: (key: string) => void;
  editing: ShelfEditing | undefined;
}) {
  const { shelf, state } = drawn;
  const title = shelfTitle(shelf);
  const all = seeAllPath(shelf);
  return (
    <ShelfRow
      ref={frontierRef}
      title={title}
      leading={shelf.kind === "folder" ? <FolderSearch aria-hidden="true" /> : undefined}
      href={shelfHeadingPath(shelf)}
      actions={
        <>
          {all !== undefined && (
            <Button variant="link" size="sm" asChild>
              <Link to={all} aria-label={t.home.seeAllFor(title)}>
                {t.home.seeAll}
              </Link>
            </Button>
          )}
          {editing !== undefined && shelf.kind === "folder" && (
            <FolderShelfMenu shelf={shelf} editing={editing} />
          )}
        </>
      }
      previousLabel={t.home.previous(title)}
      nextLabel={t.home.next(title)}
      loading={state.status === "loading" ? t.home.loadingShelf(title) : undefined}
      failed={
        state.status === "failed"
          ? {
              message: t.home.shelfFailed(state.reason),
              retryLabel: t.common.retry,
              onRetry: () => onRetry(shelf.key),
            }
          : undefined
      }
    >
      {state.status === "ready" && (
        <ShelfCards shelf={shelf} items={state.items} preview={preview} />
      )}
    </ShelfRow>
  );
}

/**
 * ShelfCards は棚のカードである。ライブラリの動画とグループのカードを、タグの行・場所・選択の
 * チェックなしで並べ、押すと再生画面を autoplay で開く（research.md R-14）。
 */
function ShelfCards({
  shelf,
  items,
  preview,
}: {
  shelf: HomeShelf;
  items: readonly LibraryItem[];
  preview: PreviewCardProps;
}) {
  const backTo = useBackTo();
  return (
    <>
      {items.map((item) => {
        if (item.kind === "smartFolder") return null;
        const common = {
          backTo,
          to: shelfItemPath(shelf, item),
          selected: false,
          selectionMode: false,
          autoplay: true,
          favoriteOnly: true,
          ...preview,
        };
        return item.kind === "video" ? (
          <VideoCard
            key={`video:${String(item.video.id)}`}
            video={item.video}
            {...common}
          />
        ) : (
          <GroupCard
            key={`group:${String(item.group.id)}`}
            group={item.group}
            {...common}
          />
        );
      })}
    </>
  );
}

/** useBackTo は再生画面の「戻る」が帰る場所（今のホームの URL）である。 */
function useBackTo(): string {
  const location = useLocation();
  return `${location.pathname}${location.search}`;
}
