import type { MouseEvent } from "react";

/** Whether a link click navigates this tab; modified clicks open another tab or window. */
export function isPlainClick(event: MouseEvent): boolean {
  return (
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey
  );
}
