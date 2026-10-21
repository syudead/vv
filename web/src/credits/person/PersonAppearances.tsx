import { UserX } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router";

import { isAborted } from "../../api/client";
import { isCreditGone, type Person } from "../../api/credits";
import {
  type FaceAppearance,
  faceCropPath,
  isAppearanceGone,
  listPersonAppearances,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { CardGrid } from "../../ui/patterns/card-grid";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "../../ui/shadcn/tooltip";
import { PersonFace, PersonFaceMark } from "../../ui/PersonFace";
import { creditDisplayName } from "../displayName";

const text = t.credits.personPage.faces;

const pageSize = 30;

type Span = FaceAppearance["spans"][number];

type AppearancesState =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "ready"; items: FaceAppearance[]; total: number; nextCursor?: string };

/** The person's appearances, a page at a time; a missing person calls onGone. */
export function usePersonAppearances(personId: number, onGone: () => void) {
  const [state, setState] = useState<AppearancesState>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const more = useRef<AbortController | null>(null);

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
    const controller = new AbortController();
    listPersonAppearances(personId, { limit: pageSize }, controller.signal).then(
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
  }, [personId, attempt, fail]);

  useEffect(() => () => more.current?.abort(), []);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);

  const cursor = state.kind === "ready" ? state.nextCursor : undefined;
  const loadMore = useCallback(() => {
    if (cursor === undefined || more.current !== null) return;
    const controller = new AbortController();
    more.current = controller;
    setLoadingMore(true);
    listPersonAppearances(personId, { limit: pageSize, cursor }, controller.signal)
      .then(
        (page) => {
          if (controller.signal.aborted) return;
          setState((current) =>
            current.kind === "ready"
              ? {
                  kind: "ready",
                  items: [...current.items, ...page.items],
                  total: page.total,
                  nextCursor: page.nextCursor,
                }
              : current,
          );
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
  }, [personId, cursor, fail]);

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

  return { state, reload, remove, loadMore, loadingMore };
}

export interface AppearanceActions {
  /** Splits the face into a new person and returns that person; a rejection is shown under the tile. */
  split: (appearance: FaceAppearance) => Promise<Person>;
  /** Merges the split person back, taking the face and its credit back. */
  undoSplit: (created: Person) => Promise<void>;
  onGone: () => void;
}

/** The video page at the span's start; the address keeps `t` in seconds across a reload. */
export function spanPath(videoId: number, span: Span): string {
  return `/videos/${String(videoId)}?t=${String(Math.floor(span.start))}`;
}

/**
 * `Faces in videos`: a grid of crops, newest video first, so a face that is not the person stands
 * out. A split face stays in place, dimmed with Undo, so the grid never moves under the owner.
 */
export function PersonAppearances({
  appearances,
  actions,
}: {
  appearances: ReturnType<typeof usePersonAppearances>;
  actions: AppearanceActions;
}) {
  const { state, reload, loadMore, loadingMore } = appearances;
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
    <>
      <div>
        <CardGrid size="face">
          {state.items.map((item) => (
            <AppearanceTile
              key={item.appearanceId}
              appearance={item}
              actions={actions}
              remove={appearances.remove}
            />
          ))}
        </CardGrid>
      </div>
      {state.nextCursor !== undefined && (
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
    </>
  );
}

function useSplit(
  appearance: FaceAppearance,
  actions: AppearanceActions,
  remove: (appearanceId: number) => void,
) {
  const [created, setCreated] = useState<Person | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  async function run() {
    if (pending) return;
    setPending(true);
    setError(null);
    try {
      if (created === null) setCreated(await actions.split(appearance));
      else {
        await actions.undoSplit(created);
        setCreated(null);
      }
    } catch (failure) {
      if (created === null && isAppearanceGone(failure)) remove(appearance.appearanceId);
      else if (created === null && isCreditGone(failure)) actions.onGone();
      else
        setError(
          (created === null ? text.splitFailed : text.undoSplitFailed)(
            errorText(failure),
          ),
        );
    } finally {
      setPending(false);
    }
  }
  return { created, pending, error, run };
}

function AppearanceTile({
  appearance,
  actions,
  remove,
}: {
  appearance: FaceAppearance;
  actions: AppearanceActions;
  remove: (appearanceId: number) => void;
}) {
  const location = useLocation();
  const from = `${location.pathname}${location.search}`;
  const { videoId, title, spans } = appearance;
  const first = spans[0];
  const { created, pending, error, run } = useSplit(appearance, actions, remove);

  const crop = (
    <PersonFace
      shape="scene"
      name={title}
      src={appearance.hasCrop ? faceCropPath(appearance.appearanceId) : undefined}
    />
  );
  return (
    <div className="relative flex min-w-0 flex-col gap-1">
      <div
        className={
          created === null ? "flex flex-col gap-1" : "flex flex-col gap-1 opacity-40"
        }
      >
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
      {created === null ? (
        <PersonFaceMark corner="top-end">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className="bg-overlay"
                aria-label={text.notThisPersonFor(title)}
                aria-disabled={pending || undefined}
                aria-busy={pending || undefined}
                onClick={() => void run()}
              >
                {pending ? <Spinner aria-hidden="true" /> : <UserX aria-hidden="true" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{text.notThisPerson}</TooltipContent>
          </Tooltip>
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
            onClick={() => void run()}
          >
            {t.credits.personPage.candidates.undo}
          </Button>
        </p>
      )}
      {error !== null && <p className="text-xs text-destructive">{error}</p>}
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
