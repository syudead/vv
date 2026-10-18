import { useCallback, useEffect, useEffectEvent, useRef, useState } from "react";

import { errorText, type UiText } from "../i18n";
import {
  type FolderListing,
  type FolderRef,
  getFolder,
  isAborted,
  listRootFolders,
  RequestFailed,
  type RootFolderListing,
} from "./client";

export interface FolderListingState<T> {
  data: T | null;
  loading: boolean;
  notFound: boolean;
  error: UiText | null;
  reload: () => void;
  /** Keep current content during refresh and suppress transient errors; missing folders become notFound. */
  refresh: () => void;
  update: (change: (data: T) => T) => void;
}

function useFolderData<T>(
  load: (signal: AbortSignal) => Promise<T>,
  key: string,
  seed?: T,
): FolderListingState<T> {
  const [data, setData] = useState<T | null>(seed ?? null);
  const [loading, setLoading] = useState(seed === undefined);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  const [generation, setGeneration] = useState(seed === undefined ? 1 : 0);
  const [quietGeneration, setQuietGeneration] = useState(0);

  // load identity changes each render; only key and generation identify a new request.
  const request = useEffectEvent((signal: AbortSignal) => load(signal));

  // Defer refresh until ordinary loading finishes so older responses cannot overwrite newer data.
  // A new ordinary load supersedes and aborts an active refresh.
  const normalInFlight = useRef(false);
  const quietPending = useRef(false);
  const quietController = useRef<AbortController | null>(null);

  useEffect(() => {
    // Generation zero restores the seed without fetching.
    if (generation === 0) return;
    const controller = new AbortController();
    quietController.current?.abort();
    quietPending.current = false;
    normalInFlight.current = true;
    setLoading(true);
    setError(null);
    setNotFound(false);
    const settle = () => {
      if (controller.signal.aborted) return;
      normalInFlight.current = false;
      if (quietPending.current) {
        quietPending.current = false;
        setQuietGeneration((value) => value + 1);
      }
    };
    request(controller.signal)
      .then((value) => {
        setData(value);
        setLoading(false);
        settle();
      })
      .catch((failure: unknown) => {
        if (isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          setNotFound(true);
        } else {
          setError(errorText(failure));
        }
        setData(null);
        setLoading(false);
        settle();
      });
    return () => {
      controller.abort();
      normalInFlight.current = false;
    };
  }, [generation, key]);

  useEffect(() => {
    if (quietGeneration === 0) return;
    if (normalInFlight.current) {
      quietPending.current = true;
      return;
    }
    const controller = new AbortController();
    quietController.current = controller;
    request(controller.signal)
      .then((value) => {
        setData(value);
        setNotFound(false);
        setError(null);
      })
      .catch((failure: unknown) => {
        if (isAborted(failure)) return;
        if (failure instanceof RequestFailed && failure.status === 404) {
          setNotFound(true);
          setData(null);
        }
      });
    return () => controller.abort();
  }, [quietGeneration]);

  const reload = useCallback(() => setGeneration((value) => value + 1), []);
  const refresh = useCallback(() => setQuietGeneration((value) => value + 1), []);
  const update = useCallback(
    (change: (data: T) => T) =>
      setData((current) => (current === null ? current : change(current))),
    [],
  );
  return { data, loading, notFound, error, reload, refresh, update };
}

export function useRootFolders(): FolderListingState<RootFolderListing> {
  return useFolderData(listRootFolders, "root");
}

export function useFolderListing(
  folder: FolderRef,
  seed?: FolderListing,
): FolderListingState<FolderListing> {
  return useFolderData(
    (signal) => getFolder(folder, signal),
    `${String(folder.rootId)}\0${folder.path}`,
    seed,
  );
}

export type RootFolderName =
  | { status: "idle" | "loading" }
  | { status: "ready"; name: string }
  | { status: "failed" };

/** Fetch the root name separately: guest child responses omit the absolute root path and expose only the final path segment. */
export function useRootFolderName(rootId: number, enabled: boolean): RootFolderName {
  const [result, setResult] = useState<{ rootId: number; name: RootFolderName }>({
    rootId,
    name: { status: "idle" },
  });

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    setResult({ rootId, name: { status: "loading" } });
    getFolder({ rootId, path: "" }, controller.signal)
      .then((listing) =>
        setResult({ rootId, name: { status: "ready", name: listing.folder.name } }),
      )
      .catch((failure: unknown) => {
        if (isAborted(failure)) return;
        setResult({ rootId, name: { status: "failed" } });
      });
    return () => controller.abort();
  }, [enabled, rootId]);

  if (!enabled) return { status: "idle" };
  return result.rootId === rootId ? result.name : { status: "loading" };
}
