import { useCallback, useEffect, useRef, useState } from "react";

import { RequestFailed } from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { getTag, type Tag } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";

/** The one `Alert` under the tag page's header. */
export type TagPageNotice =
  | { kind: "error" | "stale"; message: UiText }
  | { kind: "detached"; name: string; id: number };

export interface TagWrite<T> {
  /** Which control pressed it; that control shows the spinner. */
  key: string;
  write: () => Promise<T>;
  done: (result: T) => void;
  /** The synonym the write names, for the reason of a `tag_synonym_not_found` answer. */
  name?: string;
  /** Handles a failure itself, such as a name error under its field; true when it did. */
  failed?: (failure: unknown) => boolean;
}

const staleCodes: ReadonlySet<string> = new Set([
  "tag_not_found",
  "tag_synonym_not_found",
  "tag_not_tentative",
]);

function staleReason(failure: RequestFailed, name: string | undefined): UiText {
  if (failure.code === "tag_synonym_not_found" && name !== undefined) {
    return t.tags.page.staleName(name);
  }
  if (failure.code === "tag_not_found") return t.tags.page.staleTag;
  return t.tags.page.staleState;
}

/**
 * One tag for its page: read with `getTag`, read again in place on a `tags` event once no write of
 * the page is in flight, and gone on `404 tag_not_found`. Writes run one at a time through `run`.
 */
export function useTagRecord(id: number) {
  const [tag, setTag] = useState<Tag | undefined>(undefined);
  const [gone, setGone] = useState(false);
  const [loadError, setLoadError] = useState<UiText | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [notice, setNotice] = useState<TagPageNotice | null>(null);
  const busyRef = useRef(false);
  const waitingRef = useRef(false);
  const readRef = useRef<AbortController | null>(null);

  const load = useCallback(() => {
    readRef.current?.abort();
    const controller = new AbortController();
    readRef.current = controller;
    getTag(id, controller.signal).then(
      (fresh) => {
        if (controller.signal.aborted) return;
        setTag(fresh);
        setLoadError(null);
      },
      (failure: unknown) => {
        if (controller.signal.aborted) return;
        if (failure instanceof RequestFailed && failure.code === "tag_not_found") {
          setGone(true);
          return;
        }
        setLoadError(errorText(failure));
      },
    );
  }, [id]);

  useEffect(() => {
    setTag(undefined);
    // An id that no tag can have reads as gone without a request.
    setGone(id <= 0);
    setNotice(null);
    if (id > 0) load();
    return () => readRef.current?.abort();
  }, [id, load]);

  useEffect(
    () =>
      subscribeServerEvents({
        tags: () => {
          if (id <= 0) return;
          if (busyRef.current) waitingRef.current = true;
          else load();
        },
      }),
    [id, load],
  );

  const run = useCallback(
    async <T>({ key, write, done, name, failed }: TagWrite<T>): Promise<boolean> => {
      if (busyRef.current) return false;
      busyRef.current = true;
      setBusy(key);
      setNotice(null);
      try {
        done(await write());
        return true;
      } catch (failure) {
        if (failure instanceof RequestFailed && staleCodes.has(failure.code)) {
          setNotice({ kind: "stale", message: staleReason(failure, name) });
          waitingRef.current = true;
        } else if (failed?.(failure) !== true) {
          setNotice({ kind: "error", message: errorText(failure) });
        }
        return false;
      } finally {
        busyRef.current = false;
        setBusy(null);
        if (waitingRef.current) {
          waitingRef.current = false;
          load();
        }
      }
    },
    [load],
  );

  return { tag, setTag, gone, loadError, load, busy, notice, setNotice, run };
}

export type TagRecord = ReturnType<typeof useTagRecord>;
