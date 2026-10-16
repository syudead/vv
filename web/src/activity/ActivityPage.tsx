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
import { ActivityToolbar } from "./ActivityToolbar";
import { useActivityLog } from "./useActivityLog";
import { useActivityOperations } from "./useActivityOperations";

/** 時刻の表示を進める間隔。一覧は読み直さない。 */
const clockMs = 30_000;

function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), clockMs);
    return () => clearInterval(timer);
  }, []);
  return now;
}

/** useActivityCriteria は URL を絞り込みとして読み、書き換える口を返す。 */
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
 * ActivityFooter はログ全体の数の帯である。絞り込みによらず、サイドバーと同じ数を出す
 * （ui-design.md「Footer band」）。`running` は待ちの記録も含む。
 */
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

/**
 * ActivityPage は所有者のアクティビティの記録を新しい順にカードで並べる画面である
 * （specs/055-activity/ui-design.md「Activity page」）。絞り込みは URL に持ち、`activity` の
 * 知らせで件数と読み込んだページを読み直す。最後のカードが見えたら次のページを読む。
 */
export default function ActivityPage() {
  const here = useLocation();
  const from = `${here.pathname}${here.search}`;
  const { criteria, apply } = useActivityCriteria();
  const { state, loadMore, retry, refresh } = useActivityLog(criteria);
  const operations = useActivityOperations(refresh);
  const totals = useActivityTotals();
  const now = useNow();
  const end = useRef<HTMLDivElement>(null);
  // 絞り込みを変えたら先頭へ戻す。最初の表示と再生からの戻りでは位置を保つ。
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

  // 一覧の末尾が画面の下から 1 画面分の内に来たら次のページを読む。
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
          {items.map((entry) => (
            <ActivityCard
              key={entry.id}
              entry={entry}
              now={now}
              from={from}
              actions={openActions(entry)}
              onRetry={operations.retry}
              onUndo={operations.undo}
            />
          ))}
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
