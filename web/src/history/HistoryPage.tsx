import {
  AlertTriangle,
  Ellipsis,
  History,
  ImageOff,
  Play,
  RotateCcw,
  Trash2,
  X,
} from "lucide-react";
import { type Ref, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation } from "react-router";

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
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Timeline, TimelineGroup, TimelineItem } from "../ui/patterns/timeline";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";
import { VideoThumbnail as ThumbnailFrame } from "../ui/VideoThumbnail";
import { dayLabel, groupByDay, msUntilNextMidnight } from "./historyDays";
import { entryAction, entryPosition, folderLine } from "./historyEntry";
import { type HistoryState, useWatchHistory } from "./useWatchHistory";

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
 * HistoryPage は所有者の視聴履歴を、日ごとの時間軸に新しい順に並べる画面である
 * （specs/043-watch-history/ui-design.md「History screen」）。
 *
 * 行を押すと再生画面を開き、「Resume」「Start over」はその場で再生を始めさせる。行の × で
 * その件を確かめずに消す。全件を消すのは見出しの
 * 「More」の奥にあり、確認の窓を通す。削除の 404 は一覧が古いということなので、行を
 * 残したまま最初のページから読み直す（research.md R-6）。
 */
export default function HistoryPage() {
  const toast = useToast();
  // 行から開いた再生画面の戻り先は、今の履歴の URL（ui-design.md「Entry row」）。
  const here = useLocation();
  const from = `${here.pathname}${here.search}`;
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
        <LoadingState label={t.list.loading} layout="timeline" />
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
        <Timeline label={t.history.list}>
          {days.map((group) => {
            const label = dayLabel(group.day, now);
            const dayText = t.history.day.full(label.name, label.date);
            return (
              <TimelineGroup key={group.key} label={label.name} detail={label.date}>
                {group.entries.map((entry) => (
                  <HistoryRow
                    key={entry.id}
                    entry={entry}
                    dayText={dayText}
                    from={from}
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
              </TimelineGroup>
            );
          })}
        </Timeline>
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
 * HistoryRow は時間軸の 1 件である。時刻・サムネイル・文字を 1 つのリンクにして再生画面を
 * 開き、その後ろに「Resume」か「Start over」、端に × を置く。動画がライブラリに無い件は
 * リンクにも操作にもせず、「Not in the library」と書く（ui-design.md「Entry row」
 * 「Entry not in the library」）。
 */
function HistoryRow({
  entry,
  dayText,
  from,
  pending,
  rowRef,
  removeRef,
  onRemove,
}: {
  entry: WatchHistoryEntry;
  dayText: string;
  /** 再生画面の戻り先（今の履歴の URL）。 */
  from: string;
  pending: boolean;
  rowRef?: Ref<HTMLLIElement>;
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
  const remove = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          ref={removeRef}
          variant="ghost"
          size="icon-sm"
          className="text-muted-foreground"
          disabled={pending}
          aria-label={t.history.removeFor(title, dayText, time)}
          onClick={onRemove}
        >
          {pending ? <Spinner aria-hidden="true" /> : <X aria-hidden="true" />}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t.history.remove}</TooltipContent>
    </Tooltip>
  );

  if (video === undefined) {
    return (
      <TimelineItem
        ref={rowRef}
        time={time}
        media={
          <ThumbnailFrame className="rounded-md">
            <div className="flex size-full flex-col items-center justify-center gap-1 text-muted-foreground">
              <ImageOff className="size-5" strokeWidth={1.5} aria-hidden="true" />
              <span className="text-2xs">{t.list.card.noImage}</span>
            </div>
          </ThumbnailFrame>
        }
        remove={remove}
      >
        <EntryTitle title={title} muted />
        <EntryWarning text={t.history.notInLibrary} />
      </TimelineItem>
    );
  }

  const to = `/videos/${String(video.id)}`;
  const position = entryPosition(video);
  const action = entryAction(video);
  const folder = folderLine(video);
  const warning = unplayableText(video);
  return (
    <TimelineItem
      ref={rowRef}
      time={time}
      media={<VideoThumbnail video={video} className="rounded-md" />}
      main={({ className, children }) => (
        <Link
          to={to}
          state={{ from }}
          aria-label={t.history.entryLink(
            title,
            formatDuration(video.durationMs),
            dayText,
            time,
          )}
          className={className}
        >
          {children}
        </Link>
      )}
      action={
        action === null ? undefined : (
          <Button asChild variant="outline" size="sm">
            <Link
              to={to}
              state={{ from, autoplay: true }}
              aria-label={
                action === "resume"
                  ? t.history.resumeFor(title)
                  : t.history.startOverFor(title)
              }
            >
              {action === "resume" ? (
                <Play aria-hidden="true" />
              ) : (
                <RotateCcw aria-hidden="true" />
              )}
              {action === "resume" ? t.history.resume : t.history.startOver}
            </Link>
          </Button>
        )
      }
      remove={remove}
    >
      <EntryTitle title={title} />
      {folder !== null && (
        <span className="truncate text-xs text-muted-foreground">{folder}</span>
      )}
      {position !== null && (
        <span className="flex items-center gap-2">
          <Progress
            value={position.value}
            max={position.max}
            aria-label={t.list.card.watchedRatio}
            className="max-w-xs flex-1"
          />
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {position.text}
          </span>
        </span>
      )}
      {warning !== null && <EntryWarning text={warning} />}
    </TimelineItem>
  );
}

/** EntryTitle は行の題名である。ライブラリに無い件は弱い色で書く。 */
function EntryTitle({ title, muted = false }: { title: string; muted?: boolean }) {
  return (
    <span
      className={cn(
        "line-clamp-2 text-sm font-medium wrap-anywhere",
        muted ? "text-muted-foreground" : "text-foreground",
      )}
      title={title}
    >
      {title}
    </span>
  );
}

/** EntryWarning は再生できないことを知らせる行である。 */
function EntryWarning({ text }: { text: UiText }) {
  return (
    <span className="flex items-center gap-1 text-xs text-warning">
      <AlertTriangle aria-hidden="true" className="size-3" />
      {text}
    </span>
  );
}
