import { useCallback, useState, useSyncExternalStore } from "react";

let shared: { create: typeof window.matchMedia; query: MediaQueryList } | null = null;

/** One query for every card, made again only if matchMedia itself is replaced. */
function withoutHover(): MediaQueryList | null {
  if (typeof window.matchMedia !== "function") return null;
  if (shared?.create !== window.matchMedia) {
    shared = { create: window.matchMedia, query: window.matchMedia("(hover: none)") };
  }
  return shared.query;
}

function followHover(listener: () => void) {
  const query = withoutHover();
  query?.addEventListener?.("change", listener);
  return () => query?.removeEventListener?.("change", listener);
}

/** Devices without hover show card actions at rest, so they never wait for an approach. */
function shownAtRest(): boolean {
  return withoutHover()?.matches ?? true;
}

/**
 * Card actions stay invisible until a pointer or focus reaches the card, so they are created then
 * and kept; they are most of a card's components, and creating them for every loaded card made
 * each appended page stall. A favorite heart, which shows at rest, renders regardless.
 */
export function useCardActions() {
  // A convertible can lose hover after the page opens, when actions must show at rest.
  const atRest = useSyncExternalStore(followHover, shownAtRest);
  const [approached, setApproached] = useState(false);
  const approach = useCallback(() => setApproached(true), []);
  return { ready: atRest || approached, approach };
}
