import { AlertTriangle, Ellipsis, History, ImageOff, Trash2, X } from "lucide-react";
import { type Ref, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";

import { RequestFailed } from "../api/client";
import {
  clearWatchHistory,
  deleteWatchHistoryEntry,
  type WatchHistoryEntry,
} from "../api/history";
import { errorText, formatTime, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration, unplayableText } from "../lib/format";
import { VideoThumbnail } from "../player/RelatedVideos";
import { DialogError } from "../tags/DialogError";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import {
  GroupedList,
  GroupedListGroup,
  GroupedListItem,
} from "../ui/patterns/grouped-list";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";
import { VideoThumbnail as ThumbnailFrame } from "../ui/VideoThumbnail";
import { dayLabel, groupByDay, msUntilNextMidnight } from "./historyDays";
import { type HistoryState, useWatchHistory } from "./useWatchHistory";

/** 履歴から開いた再生画面の戻り先（ui-design.md「Entry row」）。 */
const backTo = "/history";

/**
 * useNow は日の見出し（Today・Yesterday）を決める今の時刻である。見る人のローカルの 0 時と、
 * タブがまた見えたときに進める。一覧は読み直さない（ui-design.md「Day groups」）。
 */
function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    // 0 時ちょうどより少し後に進め、時計の誤差で前の日のまま描かないようにする。
    const timer = setTimeout(() => setNow(new Date()), msUntilNextMidnight(now) + 1000);
    return () => clearTimeout(timer);
  }, [now]);
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === "visible") setNow(new Date());
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, []);
  return now;
}

function readyItems(state: HistoryState): WatchHistoryEntry[] {
  return state.kind === "ready" ? state.items : [];
}

/**
 * HistoryPage は所有者の視聴履歴を、日ごとのまとまりで新しい順に並べる画面である
 * （specs/043-watch-history/ui-design.md「History screen」）。
 *
 * 行を押すと再生画面を開き、行の × でその件を確かめずに消す。全件を消すのは見出しの
 * 「More」の奥にあり、確認の窓を通す。削除の 404 は一覧が古いということなので、行を
 * 残したまま最初のページから読み直す（research.md R-6）。
 */
export default function HistoryPage() {
  const toast = useToast();
  const history = useWatchHistory((error) => toast(errorText(error)));
  const { state, loadMore } = history;
  const now = useNow();
  const heading = useRef<HTMLHeadingElement>(null);
  const removeButtons = useRef(new Map<number, HTMLButtonElement>());
  const lastRow = useRef<HTMLElement | null>(null);
  const [removing, setRemoving] = useState<ReadonlySet<number>>(new Set());
  const [clearOpen, setClearOpen] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [clearError, setClearError] = useState<UiText | null>(null);

  useEffect(() => {
    const previous = document.title;
    document.title = t.history.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  const items = readyItems(state);
  const days = useMemo(() => groupByDay(items), [items]);
  const lastId = items.at(-1)?.id;

  // 最後の行が画面の下から 1 画面分の内に来たら次のページを読む（ui-design.md「Paging」）。
  const canLoadMore =
    state.kind === "ready" &&
    state.nextCursor !== undefined &&
    state.more.kind === "idle";
  useEffect(() => {
    const target = lastRow.current;
    if (!canLoadMore || target === null) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) loadMore();
      },
      { rootMargin: "0px 0px 100% 0px" },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [canLoadMore, items, loadMore]);

  function remove(entry: WatchHistoryEntry) {
    const id = entry.id;
    if (removing.has(id)) return;
    setRemoving((current) => new Set(current).add(id));
    const settle = () =>
      setRemoving((current) => {
        const rest = new Set(current);
        rest.delete(id);
        return rest;
      });
    deleteWatchHistoryEntry(id).then(
      () => {
        settle();
        const nextId = history.remove(id);
        // 行が抜けた後に、次の行（無ければ前の行、無ければ見出し）へフォーカスを移す。
        setTimeout(() => {
          const target =
            nextId === null ? heading.current : removeButtons.current.get(nextId);
          target?.focus();
        }, 0);
      },
      (error: unknown) => {
        settle();
        if (error instanceof RequestFailed && error.status === 404) {
          // 別のタブで消えていた。知らせずに最初のページから読み直す（R-6）。
          history.reload();
          return;
        }
        toast(errorText(error));
      },
    );
  }

  function clear() {
    setClearing(true);
    setClearError(null);
    clearWatchHistory().then(
      () => {
        setClearing(false);
        setClearOpen(false);
        history.clear();
        setTimeout(() => heading.current?.focus(), 0);
      },
      (error: unknown) => {
        setClearing(false);
        setClearError(t.history.clearDialog.failed(errorText(error)));
      },
    );
  }

  const hasRows = items.length > 0;
  const refilling = state.kind === "ready" && !hasRows && state.nextCursor !== undefined;
  const empty = state.kind === "ready" && !hasRows && state.nextCursor === undefined;

  return (
    <ListPage
      header={
        <PageHeader
          titleRef={heading}
          title={t.history.title}
          actions={
            hasRows ? (
              <DropdownMenu>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-sm" aria-label={t.common.more}>
                        <Ellipsis aria-hidden="true" />
                      </Button>
                    </DropdownMenuTrigger>
                  </TooltipTrigger>
                  <TooltipContent>{t.common.more}</TooltipContent>
                </Tooltip>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setClearError(null);
                      setClearOpen(true);
                    }}
                  >
                    <Trash2 aria-hidden="true" />
                    {t.history.clear}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : undefined
          }
        />
      }
    >
      {state.kind === "loading" && (
        <div className="flex flex-col gap-2" aria-hidden="true">
          <Skeleton className="h-9 w-1/4" />
          {Array.from({ length: 6 }, (_, index) => (
            <Skeleton key={index} className="h-16 w-full" />
          ))}
        </div>
      )}

      {state.kind === "failed" && (
        <ErrorState
          title={<h2>{t.history.loadFailed}</h2>}
          retryLabel={t.common.retry}
          onRetry={history.retry}
        />
      )}

      {empty && (
        <EmptyState
          icon={<History aria-hidden="true" />}
          title={<h2>{t.history.empty.title}</h2>}
          description={t.history.empty.description}
        />
      )}

      {hasRows && (
        <GroupedList label={t.history.list}>
          {days.map((group) => {
            const dayText = dayLabel(group.day, now);
            return (
              <GroupedListGroup key={group.key} heading={dayText}>
                {group.entries.map((entry) => (
                  <HistoryRow
                    key={entry.id}
                    entry={entry}
                    dayText={dayText}
                    pending={removing.has(entry.id)}
                    rowRef={
                      entry.id === lastId
                        ? (element) => {
                            lastRow.current = element;
                          }
                        : undefined
                    }
                    removeRef={(element) => {
                      if (element === null) removeButtons.current.delete(entry.id);
                      else removeButtons.current.set(entry.id, element);
                    }}
                    onRemove={() => remove(entry)}
                  />
                ))}
              </GroupedListGroup>
            );
          })}
        </GroupedList>
      )}

      {state.kind === "ready" &&
        (hasRows || refilling) &&
        state.more.kind === "failed" && (
          <LoadMoreRow
            status="failed"
            title={t.list.loadMoreFailed(errorText(state.more.error))}
            retryLabel={t.common.retry}
            onRetry={loadMore}
          />
        )}
      {state.kind === "ready" &&
        (state.more.kind === "loading" || (refilling && state.more.kind === "idle")) && (
          <LoadMoreRow status="loading" label={t.list.loading} />
        )}

      <ConfirmDialog
        open={clearOpen}
        onOpenChange={(open) => {
          if (!clearing) setClearOpen(open);
        }}
        title={t.history.clearDialog.title}
        description={t.history.clearDialog.description}
        cancelLabel={t.common.cancel}
        actionLabel={
          <>
            {clearing && <Spinner aria-hidden="true" />}
            {clearing ? t.history.clearDialog.submitting : t.history.clearDialog.submit}
          </>
        }
        onConfirm={clear}
        pending={clearing}
      >
        {clearError !== null && <DialogError message={clearError} />}
      </ConfirmDialog>
    </ListPage>
  );
}

/**
 * HistoryRow は履歴の 1 件である。サムネイル・題名・時刻を 1 つのリンクにして再生画面を開き、
 * 行の端の × で消す。動画がライブラリに無い件はリンクにせず、「Not in the library」と
 * 書く（ui-design.md「Entry row」「Entry not in the library」）。
 */
function HistoryRow({
  entry,
  dayText,
  pending,
  rowRef,
  removeRef,
  onRemove,
}: {
  entry: WatchHistoryEntry;
  dayText: string;
  pending: boolean;
  rowRef?: (element: HTMLElement | null) => void;
  removeRef: Ref<HTMLButtonElement>;
  onRemove: () => void;
}) {
  const time = formatTime(entry.playedAt);
  const video = entry.video;
  const title =
    video !== undefined
      ? video.title
      : entry.title === ""
        ? t.history.unknownTitle
        : entry.title;
  return (
    <GroupedListItem>
      {video !== undefined ? (
        <Link
          ref={rowRef}
          to={`/videos/${String(video.id)}`}
          state={{ from: backTo }}
          aria-label={t.history.entryLink(
            title,
            formatDuration(video.durationMs),
            dayText,
            time,
          )}
          className="-m-1 flex min-w-0 flex-1 items-center gap-3 rounded-md p-1 transition-colors hover:bg-accent"
        >
          <VideoThumbnail video={video} className="w-list-thumb rounded-sm" />
          <EntryLines title={title} time={time} warning={unplayableText(video)} />
        </Link>
      ) : (
        <div ref={rowRef} className="-m-1 flex min-w-0 flex-1 items-center gap-3 p-1">
          <ThumbnailFrame className="w-list-thumb shrink-0 rounded-sm">
            <div className="flex size-full flex-col items-center justify-center gap-1 text-muted-foreground">
              <ImageOff className="size-5" strokeWidth={1.5} aria-hidden="true" />
              <span className="text-2xs">{t.list.card.noImage}</span>
            </div>
          </ThumbnailFrame>
          <EntryLines title={title} time={time} warning={t.history.notInLibrary} muted />
        </div>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            ref={removeRef}
            variant="ghost"
            size="icon-sm"
            className="shrink-0 text-muted-foreground"
            disabled={pending}
            aria-label={t.history.removeFor(title, dayText, time)}
            onClick={onRemove}
          >
            {pending ? <Spinner aria-hidden="true" /> : <X aria-hidden="true" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t.history.remove}</TooltipContent>
      </Tooltip>
    </GroupedListItem>
  );
}

/** EntryLines は行の題名と時刻と、再生できないときの注意の行である。 */
function EntryLines({
  title,
  time,
  warning,
  muted = false,
}: {
  title: string;
  time: string;
  warning: UiText | null;
  muted?: boolean;
}) {
  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span
        className={cn(
          "line-clamp-2 text-sm font-medium wrap-anywhere",
          muted ? "text-muted-foreground" : "text-foreground",
        )}
        title={title}
      >
        {title}
      </span>
      <span className="text-xs text-muted-foreground tabular-nums">{time}</span>
      {warning !== null && (
        <span className="flex items-center gap-1 text-xs text-warning">
          <AlertTriangle aria-hidden="true" className="size-3" />
          {warning}
        </span>
      )}
    </span>
  );
}
