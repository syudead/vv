import { CircleAlert, ListChecks, TriangleAlert } from "lucide-react";
import { useCallback, useMemo, useRef, useState } from "react";
import { useParams } from "react-router";

import {
  type AssistantReviewRun,
  adoptAssistantReviewCandidates,
  rejectAssistantReviewCandidates,
} from "../api/assistant";
import { useLoadMoreWhenNear } from "../hooks/useLoadMoreWhenNear";
import { errorText, t, type UiText } from "../i18n";
import { AdminTablePage } from "../ui/patterns/admin-table-page";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadingState } from "../ui/patterns/loading-state";
import { LoadMoreRow } from "../ui/patterns/load-more-row";
import { PageHeader } from "../ui/patterns/page-header";
import { SelectionBar } from "../ui/patterns/selection-bar";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Progress } from "../ui/shadcn/progress";
import { useToast } from "../ui/Toast";
import { useDocumentTitle } from "../videoList/pageHooks";
import { useAssistant } from "./AssistantProvider";
import { proposalsByCandidate } from "./review";
import ReviewTable from "./ReviewTable";
import { type ReviewPage as Page, useReviewPage } from "./useReviewPage";

/** The most ids one adopt or reject request takes. */
const maxSelection = 1000;

function useOpenPanel(): () => void {
  const store = useAssistant();
  return useCallback(() => {
    if (store !== null && !store.open) store.toggle(true);
  }, [store]);
}

/**
 * The owner's words that started the run, from the thread that asked; the run does not carry them,
 * so after a reload the header has no description.
 */
function useRunRequest(id: number): string | undefined {
  const store = useAssistant();
  for (const exchange of store?.exchanges ?? []) {
    const reply = exchange.reply;
    if (reply?.kind === "outcome" && reply.outcome.run?.id === id) return exchange.text;
  }
  return undefined;
}

function AssistantLink() {
  const openPanel = useOpenPanel();
  return (
    <Button variant="link" size="sm" className="h-auto p-0" onClick={openPanel}>
      {t.assistant.review.back}
    </Button>
  );
}

function ProgressLine({ page, run }: { page: Page; run: AssistantReviewRun }) {
  const words = t.assistant.review;
  if (run.status === "running") {
    return (
      <div className="flex flex-col gap-1">
        <p className="text-sm text-muted-foreground tabular-nums">
          {words.judged(run.judged, run.total, run.listed)}
        </p>
        <Progress aria-label={words.progress} value={run.judged} max={run.total} />
      </div>
    );
  }
  const { adopted, rejected } = page.counts;
  return (
    <p className="text-sm text-muted-foreground tabular-nums">
      {words.summary(run.listed, run.total, adopted, rejected)}
    </p>
  );
}

function RunNotice({ run }: { run: AssistantReviewRun }) {
  const openPanel = useOpenPanel();
  const words = t.assistant.review;
  if (run.status === "failed") {
    return (
      <Alert variant="destructive">
        <CircleAlert aria-hidden="true" />
        <AlertTitle className="font-normal">{words.stopped}</AlertTitle>
        <div className="col-start-2">
          <Button variant="link" size="sm" className="h-auto p-0" onClick={openPanel}>
            {words.askAgain}
          </Button>
        </div>
      </Alert>
    );
  }
  if (run.status === "interrupted") {
    return (
      <Alert variant="warning">
        <TriangleAlert aria-hidden="true" />
        <AlertTitle className="font-normal">{words.interruptedNotice}</AlertTitle>
      </Alert>
    );
  }
  return null;
}

function NotFound() {
  return (
    <ErrorState title={t.assistant.review.notFound} description={<AssistantLink />} />
  );
}

/** Adoption and rejection of the selected rows, with the merge dialog for tags and people. */
function useActions(id: number, page: Page, clear: () => void) {
  const toast = useToast();
  const [confirming, setConfirming] = useState<readonly number[] | null>(null);
  const [pending, setPending] = useState(false);
  const [dialogError, setDialogError] = useState<UiText | null>(null);
  const [rowError, setRowError] = useState<UiText | null>(null);
  const words = t.assistant.review;

  const settle = useCallback(
    (skipped: readonly number[]) => {
      clear();
      if (skipped.length > 0) toast(words.skipped(skipped.length));
      page.refresh();
    },
    [clear, page, toast, words],
  );

  const adopt = useCallback(
    async (ids: readonly number[], merge: boolean) => {
      setPending(true);
      setRowError(null);
      setDialogError(null);
      try {
        const answer = await adoptAssistantReviewCandidates(id, ids);
        const adopted = proposalsByCandidate(ids, answer.skippedIds, answer.proposals);
        page.markAdopted(adopted);
        setConfirming(null);
        if (adopted.size > 0) {
          toast(merge ? words.merging(adopted.size) : words.creatingGroups(adopted.size));
        }
        settle(answer.skippedIds);
      } catch (error) {
        const text = words.adoptFailed(errorText(error));
        if (merge) setDialogError(text);
        else setRowError(text);
      } finally {
        setPending(false);
      }
    },
    [id, page, settle, toast, words],
  );

  const reject = useCallback(
    async (ids: readonly number[]) => {
      setPending(true);
      setRowError(null);
      try {
        const answer = await rejectAssistantReviewCandidates(id, ids);
        page.markRejected(answer.rejectedIds);
        if (answer.rejectedIds.length > 0) {
          toast(words.rejectedToast(answer.rejectedIds.length));
        }
        settle(answer.skippedIds);
      } catch (error) {
        setRowError(words.rejectFailed(errorText(error)));
      } finally {
        setPending(false);
      }
    },
    [id, page, settle, toast, words],
  );

  return {
    confirming,
    pending,
    dialogError,
    rowError,
    ask: (ids: readonly number[]) => {
      setDialogError(null);
      setConfirming(ids);
    },
    cancel: () => {
      if (!pending) setConfirming(null);
    },
    adopt,
    reject,
  };
}

type Actions = ReturnType<typeof useActions>;

function MergeDialog({ run, actions }: { run: AssistantReviewRun; actions: Actions }) {
  const words = t.assistant.review;
  const ids = actions.confirming;
  const count = ids?.length ?? 0;
  const people = run.kind === "people";
  return (
    <ConfirmDialog
      open={ids !== null}
      onOpenChange={(open) => {
        if (!open) actions.cancel();
      }}
      title={people ? words.mergePeopleTitle(count) : words.mergeTagsTitle(count)}
      description={people ? words.mergePeopleDescription : words.mergeTagsDescription}
      actionLabel={words.merge}
      cancelLabel={t.common.cancel}
      pending={actions.pending}
      onConfirm={() => {
        if (ids !== null) void actions.adopt(ids, true);
      }}
    >
      {actions.dialogError !== null && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertTitle className="font-normal">{actions.dialogError}</AlertTitle>
        </Alert>
      )}
    </ConfirmDialog>
  );
}

function ReviewSelectionBar({
  run,
  ids,
  actions,
  onClear,
}: {
  run: AssistantReviewRun;
  ids: readonly number[];
  actions: Actions;
  onClear: () => void;
}) {
  const words = t.assistant.review;
  const adopt = () => {
    if (actions.pending) return;
    if (run.kind === "works") void actions.adopt(ids, false);
    else actions.ask(ids);
  };
  return (
    <SelectionBar
      placement="header"
      count={words.selected(ids.length)}
      clearLabel={words.clearSelection}
      onClear={onClear}
      clearDisabled={actions.pending}
    >
      <Button variant="ghost" size="sm" aria-disabled={actions.pending} onClick={adopt}>
        {words.adoptSelected}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        aria-disabled={actions.pending}
        onClick={() => {
          if (!actions.pending) void actions.reject(ids);
        }}
      >
        {words.rejectSelected}
      </Button>
    </SelectionBar>
  );
}

function useSelection(page: Page) {
  const [selected, setSelected] = useState<ReadonlySet<number>>(new Set());
  const selectable = useMemo(
    () =>
      page.rows
        .filter((candidate) => candidate.state === "listed")
        .slice(0, maxSelection)
        .map((candidate) => candidate.id),
    [page.rows],
  );
  // A row that left the table or stopped being listed leaves the selection.
  const listed = useMemo(
    () =>
      new Set(
        page.rows
          .filter((row) => row.state === "listed")
          .map((candidate) => candidate.id),
      ),
    [page.rows],
  );
  const ids = useMemo(
    () => [...selected].filter((id) => listed.has(id)),
    [selected, listed],
  );
  const select = useCallback((id: number, on: boolean) => {
    setSelected((current) => {
      const next = new Set(current);
      if (on && next.size < maxSelection) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);
  const selectAll = useCallback(
    (on: boolean) => setSelected(on ? new Set(selectable) : new Set()),
    [selectable],
  );
  const clear = useCallback(() => setSelected(new Set()), []);
  const shown = useMemo(() => new Set(ids), [ids]);
  return { ids, shown, selectable, select, selectAll, clear };
}

function Tail({ page, running }: { page: Page; running: boolean }) {
  const load = page.load;
  if (load.kind !== "ready") return null;
  const words = t.assistant.review;
  if (load.more === "failed") {
    return (
      <LoadMoreRow
        status="failed"
        title={words.loadMoreFailed}
        retryLabel={t.common.retry}
        onRetry={page.loadMore}
      />
    );
  }
  if (load.more === "loading" || running) {
    return <LoadMoreRow status="loading" label={words.loadingMore} />;
  }
  return null;
}

function Body({
  page,
  run,
  selection,
}: {
  page: Page;
  run: AssistantReviewRun;
  selection: ReturnType<typeof useSelection>;
}) {
  const words = t.assistant.review;
  const running = run.status === "running" && run.judged < run.total;
  if (page.rows.length === 0) {
    if (run.status === "running") {
      return <LoadingState label={words.loadingCandidates} layout="table" count={3} />;
    }
    return (
      <EmptyState
        icon={<ListChecks aria-hidden="true" />}
        title={words.emptyTitle}
        description={words.emptyDescription}
      />
    );
  }
  return (
    <>
      <ReviewTable
        run={run}
        rows={page.rows}
        proposals={page.proposals}
        selected={selection.shown}
        selectable={selection.selectable}
        onSelect={selection.select}
        onSelectAll={selection.selectAll}
      />
      <Tail page={page} running={running} />
    </>
  );
}

function ReadyPage({
  id,
  page,
  run,
}: {
  id: number;
  page: Page;
  run: AssistantReviewRun;
}) {
  const selection = useSelection(page);
  const actions = useActions(id, page, selection.clear);
  const end = useRef<HTMLDivElement>(null);
  const load = page.load;
  const canLoadMore =
    load.kind === "ready" && load.nextCursor !== undefined && load.more === "idle";
  useLoadMoreWhenNear(end, canLoadMore, page.loadMore, page.rows.length);
  const words = t.assistant.review;
  const request = useRunRequest(run.id);
  return (
    <AdminTablePage
      header={
        selection.ids.length > 0 ? (
          <ReviewSelectionBar
            run={run}
            ids={selection.ids}
            actions={actions}
            onClear={selection.clear}
          />
        ) : (
          <PageHeader
            title={words.titles[run.kind]}
            leading={<AssistantLink />}
            description={request === undefined ? undefined : words.request(request)}
          />
        )
      }
      band={<ProgressLine page={page} run={run} />}
    >
      <RunNotice run={run} />
      {actions.rowError !== null && <ErrorState title={actions.rowError} />}
      <Body page={page} run={run} selection={selection} />
      <div ref={end} />
      <MergeDialog run={run} actions={actions} />
    </AdminTablePage>
  );
}

function LoadingPage() {
  return (
    <AdminTablePage header={<PageHeader title={t.assistant.title} />} band={null}>
      <LoadingState label={t.assistant.review.loadingCandidates} layout="table" />
    </AdminTablePage>
  );
}

/** The review list of one run: its progress, the candidates most probable first, and bulk adoption. */
export default function ReviewPage() {
  const params = useParams();
  const id = Number(params.id);
  // The route keeps the page across run ids; a fresh instance per run drops the previous run's
  // rows, adoptions and selection, and ignores its late reads.
  return <RunReviewPage key={id} id={id} />;
}

function RunReviewPage({ id }: { id: number }) {
  const page = useReviewPage(id);
  const load = page.load;
  const title =
    load.kind === "ready" ? t.assistant.review.titles[load.run.kind] : t.assistant.title;
  useDocumentTitle(String(title));
  if (load.kind === "loading") return <LoadingPage />;
  if (load.kind === "ready") return <ReadyPage id={id} page={page} run={load.run} />;
  return (
    <AdminTablePage header={<PageHeader title={t.assistant.title} />} band={null}>
      {load.kind === "notFound" ? (
        <NotFound />
      ) : (
        <ErrorState
          title={t.assistant.review.loadFailed(errorText(load.error))}
          retryLabel={t.common.retry}
          onRetry={page.retry}
        />
      )}
    </AdminTablePage>
  );
}
