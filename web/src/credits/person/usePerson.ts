import { useCallback, useEffect, useRef, useState } from "react";

import { isAborted } from "../../api/client";
import { getCredit, isCreditGone, type Person } from "../../api/credits";
import { errorText, type UiText } from "../../i18n";

export type PersonState =
  | { kind: "loading" }
  | { kind: "failed"; reason: UiText }
  | { kind: "gone" }
  | { kind: "ready"; person: Person };

/** The route's person id, or null when it is not a positive safe integer. */
export function parsePersonId(value: string | undefined): number | null {
  if (value === undefined || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}

/**
 * Read one person, or start from `handed` without reading; any action that meets a missing person
 * calls markGone.
 */
export function usePerson(id: number | null, handed?: Person) {
  const [state, setState] = useState<PersonState>(() => {
    if (id === null) return { kind: "gone" };
    return handed === undefined ? { kind: "loading" } : { kind: "ready", person: handed };
  });
  const [attempt, setAttempt] = useState(0);
  const gone = useRef(id === null);
  const skipFirstRead = useRef(handed !== undefined);

  useEffect(() => {
    if (id === null || (attempt === 0 && skipFirstRead.current)) return;
    const controller = new AbortController();
    setState({ kind: "loading" });
    getCredit("person", id, controller.signal).then(
      (credit) => {
        if (controller.signal.aborted || gone.current) return;
        setState({ kind: "ready", person: credit as Person });
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
  }, [id, attempt]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);
  /** Show the result of a later action; ignored once the person is gone. */
  const replace = useCallback((next: Person | ((current: Person) => Person)) => {
    if (gone.current) return;
    setState((current) => {
      if (typeof next !== "function") return { kind: "ready", person: next };
      return current.kind === "ready"
        ? { kind: "ready", person: next(current.person) }
        : current;
    });
  }, []);
  const markGone = useCallback(() => {
    gone.current = true;
    setState({ kind: "gone" });
  }, []);

  return { state, reload, replace, markGone };
}
