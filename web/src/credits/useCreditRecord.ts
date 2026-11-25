import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../api/client";
import { type Credit, type CreditKind, getCredit, isCreditGone } from "../api/credits";
import { errorText, type UiText } from "../i18n";
import { useDocumentTitle } from "../videoList/pageHooks";

export type CreditState<T extends Credit> =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "gone" }
  | { kind: "ready"; credit: T };

/** The route's id, or null when it is not a positive safe integer. */
export function parseCreditId(value: string | undefined): number | null {
  if (value === undefined || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}

/**
 * Read one person or organization, or start from `handed` without reading; any action that meets
 * the missing record calls markGone.
 */
export function useCreditRecord<T extends Credit>(
  kind: CreditKind,
  id: number | null,
  handed?: T,
) {
  const [state, setState] = useState<CreditState<T>>(() => {
    if (id === null) return { kind: "gone" };
    return handed === undefined ? { kind: "loading" } : { kind: "ready", credit: handed };
  });
  const [attempt, setAttempt] = useState(0);
  const gone = useRef(id === null);
  const skipFirstRead = useRef(handed !== undefined);

  useEffect(() => {
    if (id === null || (attempt === 0 && skipFirstRead.current)) return;
    const controller = new AbortController();
    setState({ kind: "loading" });
    getCredit(kind, id, controller.signal).then(
      (credit) => {
        if (controller.signal.aborted || gone.current) return;
        setState({ kind: "ready", credit: credit as T });
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        if (isCreditGone(failure)) {
          gone.current = true;
          setState({ kind: "gone" });
          return;
        }
        setState({ kind: "failed", reason: errorText(failure) });
      },
    );
    return () => controller.abort();
  }, [kind, id, attempt]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);
  /** Show the result of a later action; ignored once the record is gone. */
  const replace = useCallback((next: T | ((current: T) => T)) => {
    if (gone.current) return;
    setState((current) => {
      if (typeof next !== "function") return { kind: "ready", credit: next };
      return current.kind === "ready"
        ? { kind: "ready", credit: next(current.credit) }
        : current;
    });
  }, []);
  const markGone = useCallback(() => {
    gone.current = true;
    setState({ kind: "gone" });
  }, []);

  return { state, reload, replace, markGone };
}

/** The routed record, handed over or read, titling the document while it is shown. */
export function useRoutedCredit<T extends Credit>(
  kind: CreditKind,
  id: number | null,
  handed: T | undefined,
  title: (credit: T) => string,
) {
  const result = useCreditRecord<T>(kind, id, handed);
  const record = result.state.kind === "ready" ? result.state.credit : undefined;
  useDocumentTitle(record === undefined ? undefined : title(record));
  return { ...result, record };
}
