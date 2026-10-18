import { useLayoutEffect, useRef } from "react";

/**
 * Reset scroll only after a settled condition changes, preserving initial and playback-return
 * restoration. Undefined means unresolved; the first defined condition counts as initial.
 */
export function useScrollTopOnChange(signature: string | undefined) {
  const known = useRef(signature);
  useLayoutEffect(() => {
    if (signature === undefined || known.current === signature) return;
    const first = known.current === undefined;
    known.current = signature;
    if (!first) window.scrollTo({ top: 0, behavior: "auto" });
  }, [signature]);
}
