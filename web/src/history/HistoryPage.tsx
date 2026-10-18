import {
  AlertTriangle,
  Ellipsis,
  History,
  ImageOff,
  Play,
  RotateCcw,
  Search,
  SearchX,
  Trash2,
  X,
} from "lucide-react";
import {
  type FocusEvent,
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Link, useLocation, useNavigate } from "react-router";

import { RequestFailed } from "../api/client";
import {
  clearWatchHistory,
  deleteWatchHistoryEntry,
  type WatchHistoryEntry,
  type WatchHistoryFilter,
} from "../api/history";
import { useLoadMoreWhenNear } from "../hooks/useLoadMoreWhenNear";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatDuration, unplayableText } from "../lib/format";
import { VideoThumbnail } from "../player/RelatedVideos";
import { DialogError } from "../tags/DialogError";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { JumpList } from "../ui/patterns/jump-list";
import { ListPage } from "../ui/patterns/list-page";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import {
  GroupedList,
  GroupedListGroup,
  GroupedListItem,
} from "../ui/patterns/grouped-list";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageHeader } from "../ui/patterns/page-header";
import { Toolbar } from "../ui/patterns/toolbar";
import { Button } from "../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Progress } from "../ui/shadcn/progress";
import { Spinner } from "../ui/shadcn/spinner";
import { ToggleGroup, ToggleGroupItem } from "../ui/shadcn/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import { useToast } from "../ui/Toast";
import { VideoThumbnail as ThumbnailFrame } from "../ui/VideoThumbnail";
import type { HistoryMode } from "../videoList/listCriteria";
import SearchBox from "../videoList/SearchBox";
import { useScrollTopOnChange } from "../videoList/useScrollTopOnChange";
import {
  DEFAULT_HISTORY_CRITERIA,
  type HistoryCriteria,
  hasHistoryConditions,
  historyCriteriaKey,
  parseHistoryCriteria,
  serializeHistoryCriteria,
} from "./historyCriteria";
import { dayLabel, groupByDay, msUntilNextMidnight } from "./historyDays";
import { entryAction, entryPosition, folderLine } from "./historyEntry";
import { firstTarget, jumpTargets } from "./historyJump";
import { useHistoryDates } from "./useHistoryDates";
import { type HistoryState, useWatchHistory } from "./useWatchHistory";

const watchOptions: readonly WatchHistoryFilter[] = ["all", "inProgress", "watched"];

/** Update local-day headings at midnight and when the tab becomes visible, without refetching. */
function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    // Fire just after midnight to avoid rendering the previous day due to clock drift.
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

function useHistoryCriteria(): {
  criteria: HistoryCriteria;
  apply: (next: HistoryCriteria, mode: HistoryMode) => void;
} {
  const location = useLocation();
  const navigate = useNavigate();
  const criteria = useMemo(
    () => parseHistoryCriteria(new URLSearchParams(location.search)),
    [location.search],
  );
  const pathname = location.pathname;
  const apply = useCallback(
    (next: HistoryCriteria, mode: HistoryMode) => {
      const search = serializeHistoryCriteria(next).toString();
      navigate(
        { pathname, search: search === "" ? "" : `?${search}` },
        { replace: mode === "replace" },
      );
    },
    [navigate, pathname],
  );
  return { criteria, apply };
}

function readyItems(state: HistoryState): WatchHistoryEntry[] {
  return state.kind === "ready" ? state.items : [];
}

/** A deletion 404 means stale data: retain rows while reloading the first page. */
export default function HistoryPage() {
  const toast = useToast();
  // Preserve the full history query in playback's return URL.
  const here = useLocation();
  const from = `${here.pathname}${here.search}`;
  const { criteria, apply } = useHistoryCriteria();
  const conditions = hasHistoryConditions(criteria);
  const history = useWatchHistory(criteria, (error) => toast(errorText(error)));
  const dates = useHistoryDates(criteria.watch, criteria.query);
  const { state, loadMore } = history;
  const now = useNow();
  const heading = useRef<HTMLHeadingElement>(null);
  const searchField = useRef<HTMLInputElement | null>(null);
  const removeButtons = useRef(new Map<number, HTMLButtonElement>());
  const lastRow = useRef<HTMLElement | null>(null);
  const [removing, setRemoving] = useState<ReadonlySet<number>>(new Set());
  const [clearOpen, setClearOpen] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [clearError, setClearError] = useState<UiText | null>(null);
  // Keep narrow-screen search open while the query is nonempty.
  const [searchOpen, setSearchOpen] = useState(false);
  const searchShown = searchOpen || criteria.query !== "";
  // Cancel the deferred blur check when the search button reopens the field.
  const searchBlurTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useScrollTopOnChange(historyCriteriaKey(criteria));

  useEffect(() => {
    const previous = document.title;
    document.title = t.history.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  // On narrow screens, open search before focusing; SearchBox cannot focus a collapsed input.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (target?.closest("input, textarea, [contenteditable=true]")) return;
      setSearchOpen(true);
      setTimeout(() => searchField.current?.focus(), 0);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const items = readyItems(state);
  const days = useMemo(() => groupByDay(items), [items]);
  const lastId = items.at(-1)?.id;

  const canLoadMore =
    state.kind === "ready" &&
    state.nextCursor !== undefined &&
    state.more.kind === "idle";
  useLoadMoreWhenNear(lastRow, canLoadMore, loadMore, items);

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
        // After removal, focus the next row, previous row, or heading.
        setTimeout(() => {
          const target =
            nextId === null ? heading.current : removeButtons.current.get(nextId);
          target?.focus();
        }, 0);
      },
      (error: unknown) => {
        settle();
        if (error instanceof RequestFailed && error.status === 404) {
          // Another tab deleted this record; silently reload from the first page.
          history.reload();
          return;
        }
        toast(errorText(error));
      },
    );
  }

  function openClear() {
    setClearError(null);
    setClearOpen(true);
  }

  function clear() {
    setClearing(true);
    setClearError(null);
    clearWatchHistory().then(
      () => {
        setClearing(false);
        setClearOpen(false);
        history.clear();
        dates.clear();
        // Clear filters so the empty library is not mistaken for no matches.
        if (conditions) apply(DEFAULT_HISTORY_CRITERIA, "replace");
        setTimeout(() => heading.current?.focus(), 0);
      },
      (error: unknown) => {
        setClearing(false);
        setClearError(t.history.clearDialog.failed(errorText(error)));
      },
    );
  }

  function openSearch() {
    // A later open action must override the pending blur collapse.
    clearTimeout(searchBlurTimer.current);
    setSearchOpen(true);
    setTimeout(() => searchField.current?.focus(), 0);
  }

  // Escape clears state and blurs before the DOM updates; check emptiness and focus after rendering.
  function onSearchBlur(event: FocusEvent<HTMLDivElement>) {
    const container = event.currentTarget;
    if (container.contains(event.relatedTarget)) return;
    clearTimeout(searchBlurTimer.current);
    searchBlurTimer.current = setTimeout(() => {
      if (container.contains(document.activeElement)) return;
      if (searchField.current?.value === "") setSearchOpen(false);
    }, 0);
  }

  const hasRows = items.length > 0;
  const refilling = state.kind === "ready" && !hasRows && state.nextCursor !== undefined;
  const empty = state.kind === "ready" && !hasRows && state.nextCursor === undefined;

  // Reload dates only when watch state or query changes.
  const targets = useMemo(
    () => jumpTargets(dates.state.kind === "ready" ? dates.state.days : [], now),
    [dates.state, now],
  );
  const targetValues = [...targets.days, ...targets.months].map((item) => item.value);
  const pressed =
    criteria.date !== undefined && targetValues.includes(criteria.date)
      ? criteria.date
      : null;
  function jump(value: string | null) {
    // Omit date for the first target to avoid two URLs for the same list.
    const date = value === null || value === firstTarget(targets) ? undefined : value;
    if (date === criteria.date) {
      // Selecting the first target without date leaves the URL unchanged but must still scroll to the top.
      window.scrollTo({ top: 0, behavior: "auto" });
      return;
    }
    const { date: _previous, ...rest } = criteria;
    apply(date === undefined ? rest : { ...rest, date }, "push");
  }

  return (
    <ListPage
      toolbarRow="header"
      header={
        <PageHeader
          titleRef={heading}
          title={t.history.title}
          actions={
            <>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="lg:hidden"
                    aria-label={t.history.search.label}
                    aria-expanded={searchShown}
                    onClick={openSearch}
                  >
                    <Search aria-hidden="true" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t.history.search.label}</TooltipContent>
              </Tooltip>
              {hasRows && (
                <DropdownMenu>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="lg:hidden"
                          aria-label={t.common.more}
                        >
                          <Ellipsis aria-hidden="true" />
                        </Button>
                      </DropdownMenuTrigger>
                    </TooltipTrigger>
                    <TooltipContent>{t.common.more}</TooltipContent>
                  </Tooltip>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem variant="destructive" onSelect={openClear}>
                      <Trash2 aria-hidden="true" />
                      {t.history.clear}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </>
          }
        />
      }
      toolbar={
        <Toolbar
          searchPlacement="end"
          searchHiddenBelowLg={!searchShown}
          search={
            <div onBlur={onSearchBlur}>
              <SearchBox
                query={criteria.query}
                onCommit={(query, mode) => apply({ ...criteria, query }, mode)}
                inputRef={searchField}
                label={t.history.search.label}
                placeholder={t.history.search.label}
                syntaxHelp={false}
              />
            </div>
          }
        >
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            aria-label={t.list.filter.watch}
            value={criteria.watch}
            onValueChange={(value) => {
              // One option must always remain selected.
              if (value === "" || value === criteria.watch) return;
              apply({ ...criteria, watch: value as WatchHistoryFilter }, "push");
            }}
          >
            {watchOptions.map((option) => (
              <ToggleGroupItem key={option} value={option} className="flex-none px-3">
                {t.list.filter.watchOptions[option]}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </Toolbar>
      }
      aside={
        <JumpList
          title={t.history.jump.title}
          groups={
            state.kind === "failed" || (empty && !conditions)
              ? []
              : [targets.days, targets.months]
          }
          value={pressed}
          onValueChange={jump}
          loading={dates.state.kind === "loading" && state.kind !== "failed" && !empty}
          emptyText={t.history.jump.none}
          failed={
            dates.state.kind === "failed" && state.kind !== "failed" && !empty
              ? {
                  message: t.history.jump.loadFailed,
                  retryLabel: t.common.retry,
                  onRetry: dates.retry,
                }
              : undefined
          }
          action={
            hasRows ? (
              <Button
                variant="ghost-destructive"
                size="sm"
                className="justify-start"
                onClick={openClear}
              >
                <Trash2 aria-hidden="true" />
                {t.history.clear}
              </Button>
            ) : undefined
          }
        />
      }
    >
      {state.kind === "loading" && (
        <LoadingState label={t.list.loading} layout="grouped" />
      )}

      {state.kind === "failed" && (
        <ErrorState
          title={<h2>{t.history.loadFailed}</h2>}
          retryLabel={t.common.retry}
          onRetry={history.retry}
        />
      )}

      {empty && !conditions && (
        <EmptyState
          icon={<History aria-hidden="true" />}
          title={<h2>{t.history.empty.title}</h2>}
          description={t.history.empty.description}
        />
      )}

      {empty && conditions && (
        <EmptyState
          icon={<SearchX aria-hidden="true" />}
          title={<h2>{t.history.noMatch.title}</h2>}
          description={t.list.noMatchesHint}
          action={
            <Button size="sm" onClick={() => apply(DEFAULT_HISTORY_CRITERIA, "push")}>
              {t.list.filter.clear}
            </Button>
          }
        />
      )}

      {hasRows && (
        <GroupedList label={t.history.list}>
          {days.map((group) => {
            const label = dayLabel(group.day, now);
            const dayText = t.history.day.full(label.name, label.date);
            return (
              <GroupedListGroup key={group.key} heading={label.name} detail={label.date}>
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

const mainLayout =
  "col-span-2 col-start-1 row-span-2 row-start-1 grid grid-cols-subgrid grid-rows-subgrid items-start lg:row-span-1 lg:items-center";

/** Missing videos have neither links nor playback actions; dates belong to group headings, not rows. */
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
  /** Return URL for playback, including history filters. */
  from: string;
  pending: boolean;
  rowRef?: Ref<HTMLLIElement>;
  removeRef: Ref<HTMLButtonElement>;
  onRemove: () => void;
}) {
  const video = entry.video;
  const title =
    video !== undefined
      ? video.title
      : entry.title === ""
        ? t.history.unknownTitle
        : entry.title;
  const remove = (
    <div className="col-start-3 row-start-1 flex justify-end lg:col-start-4">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            ref={removeRef}
            variant="ghost"
            size="icon-sm"
            className="text-muted-foreground"
            disabled={pending}
            aria-label={t.history.removeFor(title, dayText)}
            onClick={onRemove}
          >
            {pending ? <Spinner aria-hidden="true" /> : <X aria-hidden="true" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t.history.remove}</TooltipContent>
      </Tooltip>
    </div>
  );

  if (video === undefined) {
    return (
      <HistoryRowLayout rowRef={rowRef}>
        <div className={mainLayout}>
          <EntryMedia>
            <ThumbnailFrame className="rounded-md">
              <div className="flex size-full flex-col items-center justify-center gap-1 text-muted-foreground">
                <ImageOff className="size-5" strokeWidth={1.5} aria-hidden="true" />
                <span className="text-2xs">{t.list.card.noImage}</span>
              </div>
            </ThumbnailFrame>
          </EntryMedia>
          <EntryText>
            <EntryTitle title={title} muted />
            <EntryWarning text={t.history.notInLibrary} />
          </EntryText>
        </div>
        {remove}
      </HistoryRowLayout>
    );
  }

  const to = `/videos/${String(video.id)}`;
  const position = entryPosition(video);
  const action = entryAction(video);
  const folder = folderLine(video);
  const warning = unplayableText(video);
  return (
    <HistoryRowLayout rowRef={rowRef}>
      <Link
        to={to}
        state={{ from }}
        aria-label={t.history.entryLink(title, formatDuration(video.durationMs), dayText)}
        className={cn(mainLayout, "rounded-md transition-colors hover:bg-accent")}
      >
        <EntryMedia>
          <VideoThumbnail video={video} className="rounded-md" />
        </EntryMedia>
        <EntryText>
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
        </EntryText>
      </Link>
      {action !== null && (
        <div className="relative col-start-2 row-start-2 flex lg:col-start-3 lg:row-start-1">
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
        </div>
      )}
      {remove}
    </HistoryRowLayout>
  );
}

function HistoryRowLayout({
  rowRef,
  children,
}: {
  rowRef?: Ref<HTMLLIElement>;
  children: ReactNode;
}) {
  return (
    <GroupedListItem ref={rowRef}>
      <div className="grid min-w-0 flex-1 grid-cols-history-row items-start gap-x-3 gap-y-1 lg:grid-cols-history-row-wide lg:items-center">
        {children}
      </div>
    </GroupedListItem>
  );
}

function EntryMedia({ children }: { children: ReactNode }) {
  return <span className="row-span-2 min-w-0 lg:row-span-1">{children}</span>;
}

function EntryText({ children }: { children: ReactNode }) {
  return <span className="flex min-w-0 flex-col gap-1">{children}</span>;
}

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

function EntryWarning({ text }: { text: UiText }) {
  return (
    <span className="flex items-center gap-1 text-xs text-warning">
      <AlertTriangle aria-hidden="true" className="size-3" />
      {text}
    </span>
  );
}
