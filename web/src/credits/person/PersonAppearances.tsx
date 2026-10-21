import { UserX } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router";

import { isAborted } from "../../api/client";
import { isCreditGone, type Person } from "../../api/credits";
import {
  type FaceAppearance,
  type FaceAppearanceSort,
  faceCropPath,
  isAppearanceGone,
  listPersonAppearances,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { cn } from "../../lib/cn";
import { CardGrid } from "../../ui/patterns/card-grid";
import { SelectionBar } from "../../ui/patterns/selection-bar";
import { Button } from "../../ui/shadcn/button";
import { Checkbox } from "../../ui/shadcn/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../ui/shadcn/select";
import { Spinner } from "../../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import { PersonFace, PersonFaceMark } from "../../ui/PersonFace";
import { creditDisplayName } from "../displayName";

const text = t.credits.personPage.faces;

const pageSize = 30;

/** The most faces one split takes, as `FaceAppearanceSplitRequest` allows. */
const maxSplitFaces = 200;

type Span = FaceAppearance["spans"][number];

type AppearancesState =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "ready"; items: FaceAppearance[]; total: number; nextCursor?: string };

/** The person's appearances, a page at a time; a missing person calls onGone. */
export function usePersonAppearances(personId: number, onGone: () => void) {
  const [sort, setSort] = useState<FaceAppearanceSort>("leastSimilar");
  const [state, setState] = useState<AppearancesState>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const more = useRef<AbortController | null>(null);
  // A split or its undo rescores the person's other faces, so a cursor taken before it no longer marks a boundary.
  const rescored = useRef(false);

  const fail = useCallback(
    (failure: unknown) => {
      if (isCreditGone(failure)) onGone();
      else setState({ kind: "failed", reason: errorText(failure) });
    },
    [onGone],
  );

  useEffect(() => {
    more.current?.abort();
    more.current = null;
    setLoadingMore(false);
    rescored.current = false;
    const controller = new AbortController();
    setState({ kind: "loading" });
    listPersonAppearances(personId, { limit: pageSize, sort }, controller.signal).then(
      (page) => {
        if (controller.signal.aborted) return;
        setState({
          kind: "ready",
          items: page.items,
          total: page.total,
          nextCursor: page.nextCursor,
        });
      },
      (failure: unknown) => {
        if (!controller.signal.aborted && !isAborted(failure)) fail(failure);
      },
    );
    return () => controller.abort();
  }, [personId, sort, attempt, fail]);

  useEffect(() => () => more.current?.abort(), []);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);

  const cursor = state.kind === "ready" ? state.nextCursor : undefined;
  const loadMore = useCallback(() => {
    if (cursor === undefined || more.current !== null) return;
    const controller = new AbortController();
    more.current = controller;
    setLoadingMore(true);
    // After a rescoring the rest is read whole and only faces not yet shown are added.
    const whole = rescored.current;
    listPersonAppearances(
      personId,
      whole ? { sort } : { limit: pageSize, cursor, sort },
      controller.signal,
    )
      .then(
        (page) => {
          if (controller.signal.aborted) return;
          // Only a whole read that arrived settles the rescoring; a failed one is read whole again.
          if (whole) rescored.current = false;
          setState((current) => {
            if (current.kind !== "ready") return current;
            const shown = new Set(current.items.map((item) => item.appearanceId));
            return {
              kind: "ready",
              items: [
                ...current.items,
                ...page.items.filter((item) => !shown.has(item.appearanceId)),
              ],
              total: page.total,
              nextCursor: page.nextCursor,
            };
          });
        },
        (failure: unknown) => {
          if (!controller.signal.aborted && !isAborted(failure)) fail(failure);
        },
      )
      .finally(() => {
        if (more.current !== controller) return;
        more.current = null;
        setLoadingMore(false);
      });
  }, [personId, cursor, sort, fail]);

  const remove = useCallback((appearanceId: number) => {
    setState((current) =>
      current.kind === "ready"
        ? {
            ...current,
            items: current.items.filter((item) => item.appearanceId !== appearanceId),
          }
        : current,
    );
  }, []);

  const markRescored = useCallback(() => {
    rescored.current = true;
  }, []);

  return { state, sort, setSort, reload, remove, loadMore, loadingMore, markRescored };
}

export interface AppearanceActions {
  /** Splits the faces into one new person and returns that person; a rejection is shown under the grid. */
  split: (appearanceIds: readonly number[]) => Promise<Person>;
  /** Merges the split person back, taking the faces and their credits back. */
  undoSplit: (created: Person) => Promise<void>;
  onGone: () => void;
}

/** The video page at the span's start; the address keeps `t` in seconds across a reload. */
export function spanPath(videoId: number, span: Span): string {
  return `/videos/${String(videoId)}?t=${String(Math.floor(span.start))}`;
}

/** Which faces are split into whom, with the faces in flight; split faces stay in the grid with Undo. */
function useSplits(
  actions: AppearanceActions,
  remove: (appearanceId: number) => void,
  rescored: () => void,
) {
  const [splits, setSplits] = useState<ReadonlyMap<number, Person>>(new Map());
  const [pending, setPending] = useState<ReadonlySet<number>>(new Set());
  const [error, setError] = useState<UiText | null>(null);
  // The faces in flight as of now, so a second press before the next render sends nothing.
  const inFlight = useRef(new Set<number>());

  async function track(ids: readonly number[], work: () => Promise<void>) {
    if (ids.length === 0 || ids.some((id) => inFlight.current.has(id))) return;
    for (const id of ids) inFlight.current.add(id);
    setPending((current) => new Set([...current, ...ids]));
    setError(null);
    try {
      await work();
    } finally {
      for (const id of ids) inFlight.current.delete(id);
      setPending((current) => new Set([...current].filter((id) => !ids.includes(id))));
    }
  }

  async function split(ids: readonly number[]) {
    await track(ids, async () => {
      try {
        const created = await actions.split(ids);
        rescored();
        setSplits(
          (current) => new Map([...current, ...ids.map((id) => [id, created] as const)]),
        );
      } catch (failure) {
        if (ids.length === 1 && isAppearanceGone(failure)) remove(ids[0] as number);
        else if (isCreditGone(failure)) actions.onGone();
        else setError(text.splitFailed(errorText(failure)));
      }
    });
  }

  async function undo(created: Person) {
    const ids = [...splits]
      .filter(([, person]) => person.id === created.id)
      .map(([id]) => id);
    await track(ids, async () => {
      try {
        await actions.undoSplit(created);
        rescored();
        setSplits((current) => new Map([...current].filter(([id]) => !ids.includes(id))));
      } catch (failure) {
        setError(text.undoSplitFailed(errorText(failure)));
      }
    });
  }
  return { splits, pending, error, split, undo };
}

/**
 * `Faces in videos`: a grid of crops, least like the person first by default, so a face that is not
 * the person comes up early. A split face stays in place, dimmed with Undo, so the grid never moves
 * under the owner; several faces can be selected and split into one new person.
 */
export function PersonAppearances({
  appearances,
  actions,
}: {
  appearances: ReturnType<typeof usePersonAppearances>;
  actions: AppearanceActions;
}) {
  const { sort, setSort, reload, loadMore, loadingMore } = appearances;
  const splits = useSplits(actions, appearances.remove, appearances.markRescored);
  const state = useKeptSplitFaces(appearances.state, splits.splits);
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<ReadonlySet<number>>(new Set());

  function toggle(id: number) {
    setSelected((current) => {
      const next = new Set(current);
      if (!next.delete(id) && next.size < maxSplitFaces) next.add(id);
      return next;
    });
  }

  async function splitSelected() {
    const ids = [...selected];
    await splits.split(ids);
    setSelected(new Set());
  }

  const toolbar = (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        value={sort}
        onValueChange={(next) => {
          // The new order starts from its first page, so faces checked further down would be split unseen.
          setSelected(new Set());
          setSort(next as FaceAppearanceSort);
        }}
      >
        <SelectTrigger size="sm" aria-label={text.sortLabel}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="leastSimilar">{text.sortLeastSimilar}</SelectItem>
          <SelectItem value="newest">{text.sortNewest}</SelectItem>
        </SelectContent>
      </Select>
      <Button
        variant="outline"
        size="sm"
        aria-pressed={selecting}
        onClick={() => {
          setSelecting((value) => !value);
          setSelected(new Set());
        }}
      >
        {text.select}
      </Button>
    </div>
  );
  return (
    <>
      {toolbar}
      <AppearancesBody
        state={state}
        reload={reload}
        tile={(item) => (
          <AppearanceTile
            key={item.appearanceId}
            appearance={item}
            created={splits.splits.get(item.appearanceId)}
            pending={splits.pending.has(item.appearanceId)}
            selection={
              selecting
                ? {
                    checked: selected.has(item.appearanceId),
                    full: selected.size >= maxSplitFaces,
                    toggle: () => toggle(item.appearanceId),
                  }
                : undefined
            }
            split={() => void splits.split([item.appearanceId])}
            undo={(created) => void splits.undo(created)}
          />
        )}
      />
      {splits.error !== null && (
        <p className="text-sm text-destructive">{splits.error}</p>
      )}
      {state.kind === "ready" && state.nextCursor !== undefined && (
        <div className="flex items-center gap-2">
          <Button
            variant="link"
            size="sm"
            aria-busy={loadingMore || undefined}
            onClick={loadMore}
          >
            {loadingMore && <Spinner aria-hidden="true" />}
            {t.credits.personPage.showMore}
          </Button>
        </div>
      )}
      {selected.size > 0 && (
        <SelectionBar
          count={
            selected.size >= maxSplitFaces
              ? text.selectedFull(selected.size)
              : text.selected(selected.size)
          }
          clearLabel={text.clearSelection}
          onClear={() => setSelected(new Set())}
          clearDisabled={splits.pending.size > 0}
        >
          <Button
            variant="ghost"
            size="sm"
            aria-disabled={splits.pending.size > 0 || undefined}
            onClick={() => void splitSelected()}
          >
            <UserX aria-hidden="true" />
            {text.splitSelected}
          </Button>
        </SelectionBar>
      )}
    </>
  );
}

/**
 * The list with the faces split on the page kept first when a new order or page no longer holds them, so their
 * Undo stays within reach until a Refresh.
 */
function useKeptSplitFaces(
  state: AppearancesState,
  splits: ReadonlyMap<number, unknown>,
): AppearancesState {
  const known = useRef(new Map<number, FaceAppearance>());
  if (state.kind !== "ready") return state;
  for (const item of state.items) known.current.set(item.appearanceId, item);
  const listed = new Set(state.items.map((item) => item.appearanceId));
  const kept = [...splits.keys()]
    .filter((id) => !listed.has(id))
    .map((id) => known.current.get(id))
    .filter((item) => item !== undefined);
  return kept.length === 0 ? state : { ...state, items: [...kept, ...state.items] };
}

function AppearancesBody({
  state,
  reload,
  tile,
}: {
  state: AppearancesState;
  reload: () => void;
  tile: (item: FaceAppearance) => ReactNode;
}) {
  if (state.kind === "loading") {
    return <p className="text-muted-foreground">{text.loading}</p>;
  }
  if (state.kind === "failed") {
    return (
      <p className="flex flex-wrap items-center gap-2 text-muted-foreground">
        {text.loadFailed(state.reason)}
        <Button variant="link" size="sm" onClick={reload}>
          {t.common.retry}
        </Button>
      </p>
    );
  }
  if (state.items.length === 0) {
    return <p className="text-muted-foreground">{text.none}</p>;
  }
  return (
    <div>
      <CardGrid size="face">{state.items.map(tile)}</CardGrid>
    </div>
  );
}

function AppearanceTile({
  appearance,
  created,
  pending,
  selection,
  split,
  undo,
}: {
  appearance: FaceAppearance;
  created: Person | undefined;
  pending: boolean;
  /** full is true once the selection holds as many faces as one split takes. */
  selection: { checked: boolean; full: boolean; toggle: () => void } | undefined;
  split: () => void;
  undo: (created: Person) => void;
}) {
  const location = useLocation();
  const from = `${location.pathname}${location.search}`;
  const { videoId, title, spans, similarity } = appearance;
  const first = spans[0];

  const crop = (
    <PersonFace
      shape="scene"
      name={title}
      src={appearance.hasCrop ? faceCropPath(appearance.appearanceId) : undefined}
    >
      {similarity !== undefined && (
        <PersonFaceMark corner="bottom-end">
          <span
            aria-label={text.similarityFor(title, similarity)}
            className={cn(
              "rounded-sm bg-overlay px-1 text-xs font-medium tabular-nums",
              appearance.unlike && "text-warning",
            )}
          >
            {text.similarity(similarity)}
          </span>
        </PersonFaceMark>
      )}
    </PersonFace>
  );
  return (
    <div className="relative flex min-w-0 flex-col gap-1">
      <div className={cn("flex flex-col gap-1", created !== undefined && "opacity-40")}>
        {videoId === undefined ? (
          <>
            {crop}
            <span className="truncate text-xs text-muted-foreground">
              {text.notInLibrary}
            </span>
          </>
        ) : (
          <Link
            to={
              first === undefined
                ? `/videos/${String(videoId)}`
                : spanPath(videoId, first)
            }
            state={{ from }}
            title={title}
            className="group flex flex-col gap-1 rounded-md"
          >
            {crop}
            <span className="truncate text-xs font-medium group-hover:underline">
              {title}
            </span>
          </Link>
        )}
        <SpanLinks videoId={videoId} title={title} spans={spans} from={from} />
      </div>
      {created === undefined ? (
        <PersonFaceMark corner="top-end">
          {selection === undefined ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="bg-overlay"
                  aria-label={text.notThisPersonFor(title)}
                  aria-disabled={pending || undefined}
                  aria-busy={pending || undefined}
                  onClick={split}
                >
                  {pending ? (
                    <Spinner aria-hidden="true" />
                  ) : (
                    <UserX aria-hidden="true" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{text.notThisPerson}</TooltipContent>
            </Tooltip>
          ) : (
            <Checkbox
              aria-label={text.selectFace(title)}
              checked={selection.checked}
              disabled={selection.full && !selection.checked}
              onCheckedChange={selection.toggle}
              className="size-6 bg-overlay"
            />
          )}
        </PersonFaceMark>
      ) : (
        <p className="flex flex-wrap items-center gap-x-1 text-xs text-muted-foreground">
          {text.splitResult(creditDisplayName(created))}
          <Button
            variant="link"
            size="sm"
            className="h-auto p-0 text-xs"
            aria-label={text.undoSplitFor(title)}
            aria-busy={pending || undefined}
            onClick={() => undo(created)}
          >
            {t.credits.personPage.candidates.undo}
          </Button>
        </p>
      )}
    </div>
  );
}

function SpanLinks({
  videoId,
  title,
  spans,
  from,
}: {
  videoId: number | undefined;
  title: string;
  spans: readonly Span[];
  from: string;
}) {
  if (spans.length === 0) return null;
  return (
    <p className="flex flex-wrap gap-x-1.5 text-xs text-muted-foreground">
      {spans.map((span) => {
        const start = formatDuration(span.start * 1000);
        return videoId === undefined ? (
          <span key={span.start}>{start}</span>
        ) : (
          <Link
            key={span.start}
            to={spanPath(videoId, span)}
            state={{ from, autoplay: true }}
            aria-label={text.spanLink(title, start)}
            className="hover:underline"
          >
            {start}
          </Link>
        );
      })}
    </p>
  );
}
