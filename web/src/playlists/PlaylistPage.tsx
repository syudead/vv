import { closestCenter, DndContext, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import {
  CircleAlert,
  Ellipsis,
  ListVideo,
  ListX,
  Pencil,
  Play,
  Trash2,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { isAborted, RequestFailed } from "../api/client";
import {
  deletePlaylist,
  getPlaylist,
  movePlaylistItem,
  type PlaylistItem,
  removePlaylistItem,
} from "../api/playlists";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { DataTable } from "../ui/patterns/data-table";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
import { TableBody } from "../ui/shadcn/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import {
  reorderAnnouncements,
  useReorderSensors,
  verticalOnly,
} from "../videoList/reorder";
import PlaylistItemRow from "./PlaylistItemRow";
import {
  dropItem,
  focusAfterRemove,
  itemTitle,
  moveItem,
  playlistDuration,
  firstPlayTarget,
} from "./playlistItems";
import RenamePlaylistDialog from "./RenamePlaylistDialog";

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "gone" }
  | { kind: "ready"; name: string; items: readonly PlaylistItem[] };

/** parseId は経路の id を正の整数として読む。読めなければ null（無いプレイリストとして扱う）。 */
function parseId(raw: string | undefined): number | null {
  if (raw === undefined || !/^[1-9]\d*$/.test(raw)) return null;
  const id = Number(raw);
  return Number.isSafeInteger(id) ? id : null;
}

/**
 * PlaylistPage はプレイリスト 1 つの項目を持ち主の決めた順に並べ、そこから再生を始め、並べ替え、
 * 外し、名前を変え、消す画面である（specs/044-playlists/ui-design.md「Playlist screen」）。
 *
 * 開いたとき、移動か外すの `404` の後に読み、自分では取り直さない（research.md R-10）。
 * 並べ替えは置いたときに番号を変えてから `move` を 1 回送る。送る要求は置いた順に 1 つずつ
 * 送り、失敗したらサーバーが最後に認めた並びへ戻す（ui-design.md「Reordering」）。
 *
 * 読み直しは移動と同じ鎖に並べる。送った移動が答えるまで読まず、読み終えるまで次の移動を
 * 送らないので、読んだ並びがどの移動より古いということがない。読み直しを決めた時点で
 * 待っている移動は古い並びに対するものなので捨てる。
 *
 * React Router は `:id` だけが変わる移動で同じ部品を使い回すので、画面は経路の id ごとに
 * 作り直す。前のプレイリストの名前・行・操作が、別の id と組み合わさって残らない。
 */
export default function PlaylistPage() {
  const params = useParams();
  return <PlaylistScreen key={params.id ?? ""} id={parseId(params.id)} />;
}

/** PlaylistScreen は経路の id 1 つのプレイリスト画面である。id が変われば作り直される。 */
function PlaylistScreen({ id }: { id: number | null }) {
  const navigate = useNavigate();
  const toast = useToast();
  const [page, setPage] = useState<Page>(
    id === null ? { kind: "gone" } : { kind: "loading" },
  );
  const [removing, setRemoving] = useState<ReadonlySet<number>>(new Set());
  const [dialog, setDialog] = useState<"rename" | "delete" | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteFailure, setDeleteFailure] = useState<UiText | null>(null);
  /** 外した後にフォーカスを移す先（行の id、見出しは "title"）。 */
  const [focusTarget, setFocusTarget] = useState<number | "title" | "more" | null>(null);
  const pageRef = useRef(page);
  /** サーバーが最後に認めた並び。移動の失敗でここへ戻す。 */
  const confirmed = useRef<readonly PlaylistItem[]>([]);
  /** 送る移動を置いた順に 1 つずつ流す鎖と、失敗で残りを捨てるための世代。 */
  const moves = useRef<Promise<void>>(Promise.resolve());
  const generation = useRef(0);
  const fetching = useRef<AbortController | null>(null);
  /** 読み直しを送った後に外せた項目。読んだ応答がその削除より古くても行に戻さない。 */
  const removedSinceRead = useRef(new Set<number>());
  const titleRef = useRef<HTMLHeadingElement>(null);
  const moreRef = useRef<HTMLButtonElement>(null);
  const removeButtons = useRef(new Map<number, HTMLButtonElement>());

  const showPage = useCallback((next: Page) => {
    pageRef.current = next;
    setPage(next);
  }, []);

  const load = useCallback((): Promise<void> => {
    if (id === null) return Promise.resolve();
    fetching.current?.abort();
    const controller = new AbortController();
    fetching.current = controller;
    removedSinceRead.current = new Set();
    return getPlaylist(id, controller.signal).then(
      (playlist) => {
        if (controller.signal.aborted) return;
        fetching.current = null;
        // 読み直した並びが新しい基準になる。待っている移動は古い並びに対するものなので捨てる。
        generation.current += 1;
        const items = playlist.items.filter(
          (item) => !removedSinceRead.current.has(item.id),
        );
        confirmed.current = items;
        showPage({ kind: "ready", name: playlist.name, items });
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        fetching.current = null;
        generation.current += 1;
        if (error instanceof RequestFailed && error.status === 404) {
          showPage({ kind: "gone" });
          return;
        }
        // 読み直しの失敗も読み込みの失敗として Retry を出す（ui-design.md「States」）。
        // 古いかもしれない行を残すと、サーバーが退けた並びを認めたように見せる。
        showPage({ kind: "failed" });
      },
    );
  }, [id, showPage]);

  /** reread は待っている移動を捨て、送った移動が答えてから読み直す。 */
  const reread = useCallback(() => {
    generation.current += 1;
    const ticket = generation.current;
    moves.current = moves.current.then(() =>
      ticket === generation.current ? load() : undefined,
    );
  }, [load]);

  useEffect(() => {
    void load();
    return () => {
      // 鎖で待っている移動と読み直しも、画面を離れたら送らない。
      generation.current += 1;
      fetching.current?.abort();
    };
  }, [load]);

  /** 画面が出ている間だけ true。離れた後に答えた削除は、移った先の画面を動かさない。 */
  const shown = useRef(true);
  useEffect(() => {
    shown.current = true;
    return () => {
      shown.current = false;
    };
  }, []);

  const documentName = page.kind === "ready" ? page.name : t.playlists.documentTitle;
  useEffect(() => {
    const previous = document.title;
    document.title = documentName;
    return () => {
      document.title = previous;
    };
  }, [documentName]);

  // 窓やメニューが閉じてフォーカスを戻した後（setTimeout 0）に移す。
  useEffect(() => {
    if (focusTarget === null) return;
    const timer = setTimeout(() => {
      if (focusTarget === "title") titleRef.current?.focus();
      else if (focusTarget === "more") moreRef.current?.focus();
      else removeButtons.current.get(focusTarget)?.focus();
      setFocusTarget(null);
    }, 0);
    return () => clearTimeout(timer);
  }, [focusTarget]);

  const sensors = useReorderSensors();

  function setItems(items: readonly PlaylistItem[]) {
    const current = pageRef.current;
    if (current.kind !== "ready") return;
    showPage({ ...current, items });
  }

  function retry() {
    showPage({ kind: "loading" });
    reread();
  }

  function dragEnd(event: DragEndEvent) {
    const current = pageRef.current;
    if (id === null || current.kind !== "ready" || event.over === null) return;
    const itemId = Number(event.active.id);
    const drop = dropItem(current.items, itemId, Number(event.over.id));
    if (drop === null) return;
    setItems(drop.items);
    const ticket = generation.current;
    const { afterItemId } = drop;
    moves.current = moves.current.then(async () => {
      if (ticket !== generation.current) return;
      try {
        await movePlaylistItem(id, itemId, afterItemId);
        if (ticket !== generation.current) return;
        confirmed.current = moveItem(confirmed.current, itemId, afterItemId);
      } catch (error) {
        if (ticket !== generation.current) return;
        if (error instanceof RequestFailed && error.status === 404) {
          reread();
          return;
        }
        generation.current += 1;
        setItems(confirmed.current);
        toast(t.playlists.item.moveFailed(errorText(error)));
      }
    });
  }

  function remove(item: PlaylistItem) {
    if (id === null || removing.has(item.id)) return;
    setRemoving((previous) => new Set(previous).add(item.id));
    const done = () =>
      setRemoving((previous) => {
        const next = new Set(previous);
        next.delete(item.id);
        return next;
      });
    removePlaylistItem(id, item.id).then(
      () => {
        done();
        removedSinceRead.current.add(item.id);
        const current = pageRef.current;
        if (current.kind !== "ready") return;
        const target = focusAfterRemove(current.items, item.id);
        confirmed.current = confirmed.current.filter((each) => each.id !== item.id);
        setItems(current.items.filter((each) => each.id !== item.id));
        setFocusTarget(target ?? "title");
      },
      (error: unknown) => {
        done();
        if (error instanceof RequestFailed && error.status === 404) {
          reread();
          return;
        }
        toast(t.playlists.item.removeFailed(errorText(error)));
      },
    );
  }

  function confirmDelete() {
    if (id === null || deleting || page.kind !== "ready") return;
    const name = page.name;
    setDeleting(true);
    setDeleteFailure(null);
    deletePlaylist(id).then(
      () => {
        setDeleting(false);
        toast(t.playlists.deleted(name));
        if (shown.current) void navigate("/playlists");
      },
      (error: unknown) => {
        setDeleting(false);
        if (error instanceof RequestFailed && error.status === 404) {
          if (shown.current) void navigate("/playlists");
          return;
        }
        setDeleteFailure(t.playlists.deleteDialog.failed(errorText(error)));
      },
    );
  }

  const ready = page.kind === "ready" ? page : null;
  // 「Play」は始める動画のある最初の項目から始める。動画の無いスマートフォルダの項目は飛ばす
  // （specs/052-smart-folders/ui-design.md「Playlist screen row」）。
  const firstTarget = id === null ? null : firstPlayTarget(id, ready?.items ?? []);

  // dnd-kit の読み上げ。位置は今の並びでの 1 始まり。
  const announcements = reorderAnnouncements(
    ready?.items ?? [],
    itemTitle,
    t.playlists.item,
  );

  const header = (() => {
    if (page.kind === "loading") {
      return (
        <PageHeader
          title={
            <>
              <span className="sr-only">{t.list.loading}</span>
              <Skeleton aria-hidden="true" className="h-6 w-1/2" />
            </>
          }
        />
      );
    }
    if (ready === null) return <PageHeader title={t.playlists.title} />;
    return (
      <PageHeader
        titleRef={titleRef}
        // 長い名前は見出しの中で 2 行まで折り返す（ui-design.md「Responsive behaviour」）。
        title={
          <span title={ready.name} className="line-clamp-2 whitespace-normal break-words">
            {ready.name}
          </span>
        }
        count={t.playlists.detail.count(
          ready.items.length,
          playlistDuration(ready.items),
        )}
        actions={
          <>
            {firstTarget !== null && (
              <Button size="sm" asChild>
                <Link
                  to={firstTarget.to}
                  state={firstTarget.state}
                  aria-label={t.playlists.detail.playFor(ready.name)}
                >
                  <Play aria-hidden="true" />
                  {t.playlists.detail.play}
                </Link>
              </Button>
            )}
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button
                      ref={moreRef}
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t.common.more}
                    >
                      <Ellipsis aria-hidden="true" />
                    </Button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent>{t.common.more}</TooltipContent>
              </Tooltip>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setDialog("rename")}>
                  <Pencil aria-hidden="true" />
                  {t.playlists.detail.rename}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={() => {
                    setDeleteFailure(null);
                    setDialog("delete");
                  }}
                >
                  <Trash2 aria-hidden="true" />
                  {t.playlists.detail.delete}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </>
        }
      />
    );
  })();

  return (
    <ListPage header={header}>
      {page.kind === "loading" && <LoadingState label={t.list.loading} layout="table" />}
      {page.kind === "failed" && (
        <ErrorState
          title={t.playlists.detail.loadFailed}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      )}
      {page.kind === "gone" && (
        <EmptyState
          icon={<ListX aria-hidden="true" />}
          title={t.playlists.detail.gone.title}
          description={t.playlists.detail.gone.description}
          action={
            <Button size="sm" asChild>
              <Link to="/playlists">{t.playlists.detail.gone.action}</Link>
            </Button>
          }
        />
      )}
      {ready !== null && ready.items.length === 0 && (
        <EmptyState
          icon={<ListVideo aria-hidden="true" />}
          title={t.playlists.detail.empty.title}
          description={t.playlists.detail.empty.description}
          action={
            <Button size="sm" asChild>
              <Link to="/">{t.playlists.detail.empty.action}</Link>
            </Button>
          }
        />
      )}
      {ready !== null && id !== null && ready.items.length > 0 && (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          modifiers={[verticalOnly]}
          onDragEnd={dragEnd}
          accessibility={{
            announcements,
            screenReaderInstructions: {
              draggable: t.playlists.item.reorderInstructions,
            },
          }}
        >
          <SortableContext
            items={ready.items.map((item) => item.id)}
            strategy={verticalListSortingStrategy}
          >
            <DataTable label={t.playlists.detail.table}>
              <TableBody>
                {ready.items.map((item, index) => (
                  <PlaylistItemRow
                    key={item.id}
                    playlistId={id}
                    item={item}
                    position={index + 1}
                    removing={removing.has(item.id)}
                    onRemove={remove}
                    removeRef={(node) => {
                      if (node === null) removeButtons.current.delete(item.id);
                      else removeButtons.current.set(item.id, node);
                    }}
                  />
                ))}
              </TableBody>
            </DataTable>
          </SortableContext>
        </DndContext>
      )}
      {dialog === "rename" && ready !== null && id !== null && (
        <RenamePlaylistDialog
          playlistId={id}
          currentName={ready.name}
          onClose={() => setDialog(null)}
          onRenamed={(playlist) => {
            const current = pageRef.current;
            if (current.kind === "ready") showPage({ ...current, name: playlist.name });
            setDialog(null);
            setFocusTarget("more");
          }}
          onGone={() => {
            setDialog(null);
            showPage({ kind: "gone" });
          }}
        />
      )}
      {ready !== null && (
        <ConfirmDialog
          open={dialog === "delete"}
          onOpenChange={(open) => {
            if (!open && !deleting) setDialog(null);
          }}
          title={t.playlists.deleteDialog.title(ready.name)}
          description={t.playlists.deleteDialog.description}
          cancelLabel={t.common.cancel}
          actionLabel={
            <>
              {deleting && <Spinner aria-hidden="true" />}
              {deleting
                ? t.playlists.deleteDialog.submitting
                : t.playlists.deleteDialog.submit}
            </>
          }
          onConfirm={confirmDelete}
          pending={deleting}
        >
          {deleteFailure !== null && (
            <Alert variant="destructive">
              <CircleAlert aria-hidden="true" />
              <AlertTitle>{deleteFailure}</AlertTitle>
            </Alert>
          )}
        </ConfirmDialog>
      )}
    </ListPage>
  );
}
