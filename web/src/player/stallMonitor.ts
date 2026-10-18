/** Count three waits within 60 seconds or one lasting 10 seconds, excluding waits before playing after seeks/source changes. Pause or reconnect resets counting; callers supply time and deadline ticks. */

export const stallWindowMs = 60_000;
export const stallCountThreshold = 3;
export const stallDurationMs = 10_000;

export interface StallState {
  /** Count only after playing and before seek, source change, pause, or reconnect. */
  counting: boolean;
  waitingSinceMs: number | null;
  /** Counted wait starts within the window, oldest first. */
  waitStartsMs: number[];
  /** Latched until end or failure clears the warning. */
  stalled: boolean;
}

export type StallEvent =
  | "waiting"
  | "playing"
  | "seeking"
  | "loadstart"
  | "play"
  | "pause"
  | "recovering"
  | "tick"
  | "clear";

export const initialStallState: StallState = {
  counting: false,
  waitingSinceMs: null,
  waitStartsMs: [],
  stalled: false,
};

export function stallStep(
  state: StallState,
  event: StallEvent,
  nowMs: number,
): StallState {
  switch (event) {
    case "waiting": {
      if (!state.counting || state.waitingSinceMs !== null) return state;
      const waitStartsMs = [...inWindow(state.waitStartsMs, nowMs), nowMs];
      return {
        ...state,
        waitingSinceMs: nowMs,
        waitStartsMs,
        stalled: state.stalled || waitStartsMs.length >= stallCountThreshold,
      };
    }
    case "playing":
      return {
        ...judgeDuration(state, nowMs),
        counting: true,
        waitingSinceMs: null,
      };
    case "seeking":
    case "loadstart":
    case "play":
      // Exclude waits before playback settles; retain earlier wait counts without measuring their duration.
      return { ...judgeDuration(state, nowMs), counting: false, waitingSinceMs: null };
    case "pause":
    case "recovering":
      return { ...state, counting: false, waitingSinceMs: null, waitStartsMs: [] };
    case "tick":
      return judgeDuration(state, nowMs);
    case "clear":
      return initialStallState;
  }
}

export function waitDeadlineMs(state: StallState): number | null {
  if (state.stalled || state.waitingSinceMs === null) return null;
  return state.waitingSinceMs + stallDurationMs;
}

function judgeDuration(state: StallState, nowMs: number): StallState {
  if (state.stalled || state.waitingSinceMs === null) return state;
  if (nowMs - state.waitingSinceMs < stallDurationMs) return state;
  return { ...state, stalled: true };
}

function inWindow(startsMs: number[], nowMs: number): number[] {
  return startsMs.filter((start) => nowMs - start < stallWindowMs);
}
