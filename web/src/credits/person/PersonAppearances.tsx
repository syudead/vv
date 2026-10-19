import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router";

import { isAborted } from "../../api/client";
import { isCreditGone } from "../../api/credits";
import {
  type FaceAppearance,
  faceCropPath,
  isAppearanceGone,
  listPersonAppearances,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { formatDuration } from "../../lib/format";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { PersonFace } from "../../ui/PersonFace";

const text = t.credits.personPage.faces;

const pageSize = 20;

type Span = FaceAppearance["spans"][number];

type AppearancesState =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "ready"; items: FaceAppearance[]; nextCursor?: string };

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
        setState({ kind: "ready", items: page.items, nextCursor: page.nextCursor });
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

  // Splitting every shown row leaves no row to show while later pages remain, so read the next one.
  const emptied =
    state.kind === "ready" && state.items.length === 0 && cursor !== undefined;
  useEffect(() => {
    if (emptied) loadMore();
  }, [emptied, loadMore]);

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
  /** Splits the face into a new person and updates the page; a rejection is shown under the row. */
  split: (appearance: FaceAppearance) => Promise<void>;
  onGone: () => void;
}

/** The video page at the span's start; the address keeps `t` in seconds across a reload. */
export function spanPath(videoId: number, span: Span): string {
  return `/videos/${String(videoId)}?t=${String(Math.floor(span.start))}`;
}

function spanText(span: Span): string {
  return `${formatDuration(span.start * 1000)}–${formatDuration(span.end * 1000)}`;
}

/** `Faces in videos`: a crop beside its video's title and spans, newest video first. */
export function PersonAppearances({
  appearances,
  actions,
}: {
  appearances: ReturnType<typeof usePersonAppearances>;
  actions: AppearanceActions;
}) {
  const { state, reload, loadMore, loadingMore } = appearances;
  if (
    state.kind === "loading" ||
    (state.kind === "ready" && state.items.length === 0 && state.nextCursor === undefined)
  ) {
    return null;
  }
  return (
    <section className="flex flex-col gap-2 text-sm">
      <h2 className="text-sm font-semibold">{text.heading}</h2>
      {state.kind === "failed" ? (
        <p className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          {text.loadFailed(state.reason)}
          <Button variant="link" size="sm" onClick={reload}>
            {t.common.retry}
          </Button>
        </p>
      ) : (
        <>
          <ul className="flex flex-col gap-2">
            {state.items.map((item) => (
              <AppearanceRow
                key={item.appearanceId}
                appearance={item}
                actions={actions}
                remove={appearances.remove}
              />
            ))}
          </ul>
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
      )}
    </section>
  );
}

function AppearanceRow({
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
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);

  async function split() {
    if (pending) return;
    setPending(true);
    setError(null);
    try {
      await actions.split(appearance);
    } catch (failure) {
      setPending(false);
      if (isAppearanceGone(failure)) remove(appearance.appearanceId);
      else if (isCreditGone(failure)) actions.onGone();
      else setError(text.splitFailed(errorText(failure)));
    }
  }

  return (
    <li className="flex flex-wrap items-start gap-2">
      <PersonFace
        shape="scene"
        name={title}
        src={appearance.hasCrop ? faceCropPath(appearance.appearanceId) : undefined}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        {videoId === undefined ? (
          <span className="truncate text-muted-foreground">{text.notInLibrary}</span>
        ) : (
          <Link
            to={
              first === undefined
                ? `/videos/${String(videoId)}`
                : spanPath(videoId, first)
            }
            state={{ from }}
            className="truncate hover:underline"
          >
            {title}
          </Link>
        )}
        {spans.length > 0 && (
          <p className="text-xs text-muted-foreground">
            {spans.map((span, index) => (
              <Fragment key={span.start}>
                {index > 0 && " · "}
                {videoId === undefined ? (
                  spanText(span)
                ) : (
                  <Link
                    to={spanPath(videoId, span)}
                    state={{ from, autoplay: true }}
                    aria-label={text.spanLink(title, formatDuration(span.start * 1000))}
                    className="hover:underline"
                  >
                    {spanText(span)}
                  </Link>
                )}
              </Fragment>
            ))}
          </p>
        )}
        {error !== null && <p className="text-xs text-destructive">{error}</p>}
      </div>
      <Button
        variant="ghost"
        size="sm"
        aria-label={text.notThisPersonFor(title)}
        aria-disabled={pending || undefined}
        aria-busy={pending || undefined}
        onClick={() => void split()}
      >
        {pending && <Spinner aria-hidden="true" />}
        {text.notThisPerson}
      </Button>
    </li>
  );
}
