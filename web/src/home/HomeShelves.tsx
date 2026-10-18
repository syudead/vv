import { FolderSearch } from "lucide-react";
import { Link, useLocation } from "react-router";

import type { HomeShelf, HomeShelfItem } from "../api/home";
import { useAudience } from "../auth/audience";
import { t } from "../i18n";
import { GroupCard } from "../library/GroupCard";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadingState } from "../ui/patterns/loading-state";
import { ShelfRow, ShelfRows } from "../ui/patterns/shelf-row";
import { Toolbar } from "../ui/patterns/toolbar";
import { Button } from "../ui/shadcn/button";
import {
  usePreviewCoordination,
  type PreviewCardProps,
} from "../videoList/usePreviewCoordination";
import VideoCard from "../videoList/VideoCard";
import AddShelfButton from "./AddShelfButton";
import {
  type DrawnShelf,
  drawnShelves,
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

export default function HomeShelves() {
  const home = useHome();
  const owner = useAudience() === "owner";
  const writes = useShelfWrites(home.reload);
  const order =
    owner && home.list.status === "ready"
      ? folderShelfIds(home.list.home.shelves)
      : undefined;
  const editing = order === undefined ? undefined : { order, writes };
  return (
    <>
      {order !== undefined && (
        <Toolbar
          actions={
            <AddShelfButton
              onHome={order}
              disabled={writes.busy}
              onPick={(folder) => void writes.add(folder, order)}
            />
          }
        />
      )}
      <HomeBody {...home} editing={editing} />
    </>
  );
}

interface ShelfEditing {
  order: readonly number[];
  writes: ReturnType<typeof useShelfWrites>;
}

interface HomeBodyProps {
  list: HomeList;
  editing: ShelfEditing | undefined;
  shelves: ShelfStates;
  loadMore: () => void;
  retryList: () => void;
  retryShelf: (key: string) => void;
}

function HomeBody({
  list,
  editing,
  shelves,
  loadMore,
  retryList,
  retryShelf,
}: HomeBodyProps) {
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
  if (list.home.empty === "noVideos") return null;
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
  return <ShelfRows label={t.home.shelves}>{rows}</ShelfRows>;
}

function FolderShelfMenu({
  shelf,
  editing,
}: {
  shelf: HomeShelf;
  editing: ShelfEditing;
}) {
  const id = shelf.folderId;
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
      leading={<FolderSearch aria-hidden="true" />}
      href={shelfHeadingPath(shelf)}
      actions={
        <>
          <Button variant="link" size="sm" asChild>
            <Link to={all} aria-label={t.home.seeAllFor(title)}>
              {t.home.seeAll}
            </Link>
          </Button>
          {editing !== undefined && <FolderShelfMenu shelf={shelf} editing={editing} />}
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

function ShelfCards({
  shelf,
  items,
  preview,
}: {
  shelf: HomeShelf;
  items: readonly HomeShelfItem[];
  preview: PreviewCardProps;
}) {
  const backTo = useBackTo();
  return (
    <>
      {items.map((item) => {
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

function useBackTo(): string {
  const location = useLocation();
  return `${location.pathname}${location.search}`;
}
