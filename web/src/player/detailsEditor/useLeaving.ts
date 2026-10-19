import { useCallback, useEffect, useRef, useState } from "react";
import { type Location, useLocation, useNavigate } from "react-router";

// Shared by the full-page editors (videos and people): Back, Cancel and the aside link leave the same way.

function isSentinel(state: unknown): boolean {
  return (state as { sentinel?: unknown } | null)?.sentinel === true;
}

/** Entry links put `fromPage: true` in the history state so leaving returns along the history. */
function openedFromPage(state: unknown): boolean {
  return (state as { fromPage?: unknown } | null)?.fromPage === true;
}

/**
 * Add a history sentinel to intercept Back and confirm dirty drafts. Restore the page's history or
 * replace direct editor URLs with `pagePath` on exit; `handBack` runs first with the saved value.
 */
export function useLeaving<T>({
  pagePath,
  changed,
  saving,
  handBack,
}: {
  pagePath: string;
  changed: boolean;
  saving: boolean;
  handBack: (saved?: T) => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const locationRef = useRef<Location>(location);
  locationRef.current = location;
  const changedRef = useRef(changed);
  changedRef.current = changed;
  const savingRef = useRef(saving);
  savingRef.current = saving;
  const handBackRef = useRef(handBack);
  handBackRef.current = handBack;
  const leaving = useRef(false);
  // Pop the sentinel before replacing a directly opened editor; otherwise Back would reopen the leftover editor entry.
  const replaceOnArrival = useRef(false);
  const sentinelSeen = useRef(false);
  const handledKey = useRef<string | null>(null);
  const [confirm, setConfirm] = useState<"cancel" | "back" | null>(null);

  const pushSentinel = useCallback(() => {
    const current = locationRef.current;
    const state = (current.state ?? {}) as Record<string, unknown>;
    void navigate(
      { pathname: current.pathname, search: current.search },
      { state: { ...state, sentinel: true } },
    );
  }, [navigate]);

  const leave = useCallback(
    (saved?: T) => {
      leaving.current = true;
      setConfirm(null);
      handBackRef.current(saved);
      const onSentinel = isSentinel(locationRef.current.state);
      if (openedFromPage(locationRef.current.state)) {
        void navigate(onSentinel ? -2 : -1);
      } else if (onSentinel) {
        replaceOnArrival.current = true;
        void navigate(-1);
      } else {
        void navigate(pagePath, { replace: true });
      }
    },
    [navigate, pagePath],
  );

  useEffect(() => {
    // Process each history entry once despite StrictMode effect replay.
    if (handledKey.current === location.key) return;
    handledKey.current = location.key;
    if (isSentinel(location.state)) {
      sentinelSeen.current = true;
      return;
    }
    if (!sentinelSeen.current) {
      pushSentinel();
      return;
    }
    if (replaceOnArrival.current) {
      replaceOnArrival.current = false;
      void navigate(pagePath, { replace: true });
      return;
    }
    if (leaving.current) return;
    if (savingRef.current) {
      pushSentinel();
      return;
    }
    if (changedRef.current) {
      setConfirm("back");
      return;
    }
    leave();
  }, [leave, location.key, location.state, navigate, pagePath, pushSentinel]);

  useEffect(() => {
    if (!changed) return;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      // Older browsers inspect returnValue.
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [changed]);

  const requestLeave = useCallback(() => {
    if (savingRef.current) return;
    if (changedRef.current) setConfirm("cancel");
    else leave();
  }, [leave]);

  const keepEditing = useCallback(() => {
    // Closing the discard dialog after leaving must not recreate the sentinel.
    if (leaving.current) return;
    if (confirm === "back") pushSentinel();
    setConfirm(null);
  }, [confirm, pushSentinel]);

  return {
    confirmOpen: confirm !== null,
    leave,
    requestLeave,
    keepEditing,
  };
}
