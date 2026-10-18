import { useEffect } from "react";

/** Respect handled Escape events and detect dialogs globally: disabled controls can blur focus to body, and clearing selection could unmount a pending mutation's dialog before refresh or toast handling. */
export function useEscapeClearsSelection(active: boolean, clear: () => void): void {
  useEffect(() => {
    if (!active) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      if (document.querySelector('[aria-modal="true"]') !== null) return;
      clear();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [active, clear]);
}
