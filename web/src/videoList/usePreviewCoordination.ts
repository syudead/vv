import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";

export interface PreviewState {
  readonly activeId: number | null;
  /** Advances on every reset so cards drop pending or playing previews. */
  readonly epoch: number;
}

export type PreviewListener = (current: PreviewState, previous: PreviewState) => void;

/**
 * One hover preview shared by a list's cards. Cards subscribe instead of receiving the state as
 * props, so a preview start or reset re-renders only the cards whose own state changes.
 */
export interface PreviewCoordinator {
  readonly state: () => PreviewState;
  readonly start: (id: number) => void;
  readonly reset: () => void;
  readonly subscribe: (listener: PreviewListener) => () => void;
}

export function createPreviewCoordinator(): PreviewCoordinator {
  let state: PreviewState = { activeId: null, epoch: 0 };
  const listeners = new Set<PreviewListener>();
  const change = (next: PreviewState) => {
    const previous = state;
    state = next;
    for (const listener of [...listeners]) listener(next, previous);
  };
  return {
    state: () => state,
    start: (id) => {
      if (state.activeId !== id) change({ ...state, activeId: id });
    },
    reset: () => change({ activeId: null, epoch: state.epoch + 1 }),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

/** Coordinate one hover preview across cards and reset it on viewport resizing. */
export function usePreviewCoordination() {
  const [coordinator] = useState(createPreviewCoordinator);
  const resetPreview = coordinator.reset;

  useEffect(() => {
    window.addEventListener("resize", resetPreview);
    return () => window.removeEventListener("resize", resetPreview);
  }, [resetPreview]);

  const cardProps = useMemo(() => ({ previewCoordinator: coordinator }), [coordinator]);

  return { resetPreview, cardProps };
}

export type PreviewCardProps = ReturnType<typeof usePreviewCoordination>["cardProps"];

const unsubscribed = () => () => undefined;

/** Whether `id` may show its preview; every card may when no coordinator is supplied. */
export function usePreviewActive(
  coordinator: PreviewCoordinator | undefined,
  id: number,
): boolean {
  return useSyncExternalStore(
    coordinator?.subscribe ?? unsubscribed,
    () => coordinator === undefined || coordinator.state().activeId === id,
  );
}

/** Call `onChange` on each coordinator change without re-rendering the caller. */
export function usePreviewChanges(
  coordinator: PreviewCoordinator | undefined,
  onChange: PreviewListener,
) {
  const latest = useRef(onChange);
  useLayoutEffect(() => {
    latest.current = onChange;
  });
  useEffect(
    () =>
      coordinator?.subscribe((current, previous) => latest.current(current, previous)),
    [coordinator],
  );
}
