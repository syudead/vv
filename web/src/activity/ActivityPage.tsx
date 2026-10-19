import { Activity, ListFilter } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router";

import type { ActivityCounts } from "../api/activity";
import { useActivityTotals } from "../api/useActivityTotals";
import { useLoadMoreWhenNear } from "../hooks/useLoadMoreWhenNear";
import { t } from "../i18n";
import { CardList } from "../ui/patterns/card-list";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { LoadingState } from "../ui/patterns/loading-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import type { HistoryMode } from "../videoList/listCriteria";
import { useScrollTopOnChange } from "../videoList/useScrollTopOnChange";
import { ActivityCard } from "./ActivityCard";
import {
  type ActivityCriteria,
  activityCriteriaKey,
  DEFAULT_ACTIVITY_CRITERIA,
  hasActivityConditions,
  parseActivityCriteria,
  serializeActivityCriteria,
} from "./activityCriteria";
import { openActions } from "./activityEntry";
import { ActivityPipelineCard } from "./ActivityPipelineCard";
import { ActivityToolbar } from "./ActivityToolbar";
import { useActivityLog } from "./useActivityLog";
import { useActivityOperations } from "./useActivityOperations";

/** Refresh relative timestamps without reloading the list. */
const clockMs = 30_000;

function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), clockMs);
    return () => clearInterval(timer);
  }, []);
  return now;
}

function useActivityCriteria(): {
  criteria: ActivityCriteria;
  apply: (next: ActivityCriteria, mode?: HistoryMode) => void;
} {
  const location = useLocation();
  const navigate = useNavigate();
  const criteria = useMemo(
    () => parseActivityCriteria(new URLSearchParams(location.search)),
    [location.search],
  );
  const pathname = location.pathname;
  const apply = useCallback(
    (next: ActivityCriteria, mode: HistoryMode = "push") => {
      const search = serializeActivityCriteria(next).toString();
      void navigate(
        { pathname, search: search === "" ? "" : `?${search}` },
        { replace: mode === "replace" },
      );
    },
    [navigate, pathname],
  );
  return { criteria, apply };
}

/**
 * A pipeline card follows its state until the owner presses its button; that choice then holds
 * while the page stays open, through refreshes, filters and status changes.
 */
function usePipelineExpansion(): {
  chosen: ReadonlyMap<number, boolean>;
  choose: (id: number, open: boolean) => void;
} {
  const [chosen, setChosen] = useState<ReadonlyMap<number, boolean>>(() => new Map());
  const choose = useCallback((id: number, open: boolean) => {
    setChosen((previous) => new Map(previous).set(id, open));
  }, []);
  return { chosen, choose };
}

/** Global counts ignore filters; running includes queued records. */
function ActivityFooter({ counts }: { counts: ActivityCounts | null }) {
  if (counts === null) return null;
  return (
    <div
      role="status"
      aria-label={t.activity.footer.label}
      className="mt-auto flex min-h-activity-card items-center gap-4 border-t border-border text-sm text-muted-foreground"
    >
      <span>
        <span className="font-semibold text-primary">
          {t.activity.footer.running(counts.running)}
        </span>
      </span>
      <span>
        <span className="font-semibold text-destructive">
          {t.activity.footer.attention(counts.attention)}
        </span>
      </span>
      <span className="ml-auto hidden lg:inline">
        {t.activity.footer.done(counts.done)}
      </span>
    </div>
  );
}

export default function ActivityPage() {
  const here = useLocation();
  const from = `${here.pathname}${here.search}`;
  const { criteria, apply } = useActivityCriteria();
  const { state, loadMore, retry, refresh } = useActivityLog(criteria);
  const operations = useActivityOperations(refresh);
  const expansion = usePipelineExpansion();
  const totals = useActivityTotals();
  const now = useNow();
  const end = useRef<HTMLDivElement>(null);
  // Reset scroll only on filter changes, preserving initial and playback-return positions.
  useScrollTopOnChange(activityCriteriaKey(criteria));

  useEffect(() => {
    const previous = document.title;
    document.title = t.activity.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  const items = state.kind === "ready" ? state.items : [];
  const canLoadMore =
    state.kind === "ready" &&
    state.nextCursor !== undefined &&
    state.more.kind === "idle";

  // Prefetch within one viewport of the list end.
  useLoadMoreWhenNear(end, canLoadMore, loadMore, items.length);

  const filtered = hasActivityConditions(criteria);
  const clear = () => apply(DEFAULT_ACTIVITY_CRITERIA);

  return (
    <ListPage
      header={<PageHeader title={t.activity.title} />}
      toolbar={<ActivityToolbar criteria={criteria} state={state} onChange={apply} />}
    >
      {state.kind === "loading" && (
        <LoadingState label={t.activity.loading} layout="cards" />
      )}
      {state.kind === "failed" && (
        <ErrorState
          title={t.activity.loadFailed}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      )}
      {state.kind === "ready" && items.length === 0 && !filtered && (
        <EmptyState
          icon={<Activity aria-hidden="true" />}
          title={t.activity.empty.title}
          description={t.activity.empty.description}
        />
      )}
      {state.kind === "ready" && items.length === 0 && filtered && (
        <EmptyState
          icon={<ListFilter aria-hidden="true" />}
          title={t.activity.noMatch.title}
          description={t.activity.noMatch.description}
          action={<Button onClick={clear}>{t.activity.filter.clear}</Button>}
        />
      )}
      {items.length > 0 && (
        <CardList label={t.activity.list}>
          {items.map((entry) =>
            entry.kind === "pipeline" ? (
              <ActivityPipelineCard
                key={entry.id}
                item={entry}
                now={now}
                from={from}
                actions={openActions(entry)}
                chosen={expansion.chosen.get(entry.id)}
                onExpandedChange={(open) => expansion.choose(entry.id, open)}
                onRetry={operations.retry}
                onUndo={operations.undo}
              />
            ) : (
              <ActivityCard
                key={entry.id}
                entry={entry}
                now={now}
                from={from}
                actions={openActions(entry)}
                onRetry={operations.retry}
                onUndo={operations.undo}
              />
            ),
          )}
        </CardList>
      )}
      {state.kind === "ready" && state.more.kind === "loading" && (
        <LoadMoreRow status="loading" label={t.activity.loadingMore} />
      )}
      {state.kind === "ready" && state.more.kind === "failed" && (
        <LoadMoreRow
          status="failed"
          title={t.activity.loadMoreFailed}
          retryLabel={t.common.retry}
          onRetry={loadMore}
        />
      )}
      <div ref={end} />
      <ActivityFooter counts={totals} />
    </ListPage>
  );
}
