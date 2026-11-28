import { act } from "@testing-library/react";

/**
 * Drain every queued promise callback, such as a faked response's body and its handlers, then flush
 * the React work they scheduled. It waits for one event-loop turn, not for the clock, so a test
 * still observes the request or state that should start the chain before calling it.
 */
export function settle(): Promise<void> {
  return act(() => new Promise<void>((resolve) => setTimeout(resolve, 0)));
}
