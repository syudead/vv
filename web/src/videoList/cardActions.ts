import { useCallback, useState } from "react";

/** Devices without hover show card actions at rest, so they never wait for an approach. */
function shownAtRest(): boolean {
  return (
    typeof window.matchMedia !== "function" || window.matchMedia("(hover: none)").matches
  );
}

/**
 * Card actions stay invisible until a pointer or focus reaches the card, so they are created then
 * and kept; they are most of a card's components, and creating them for every loaded card made
 * each appended page stall. A favorite heart, which shows at rest, renders regardless.
 */
export function useCardActions() {
  const [ready, setReady] = useState(shownAtRest);
  const approach = useCallback(() => setReady(true), []);
  return { ready, approach };
}
