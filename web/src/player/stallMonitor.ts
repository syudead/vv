/**
 * stallMonitor は、回線の遅さで再生が途切れているかを判断する純粋な状態機械である
 * （specs/027-playback-quality/research.md R-6、親 Issue 要件 9・Edge Case 8）。
 *
 * データ待ちは、再生中に届いた `waiting` から次の `playing` までとする。60 秒の窓の中に
 * 始まったデータ待ちが 3 回以上になったか、1 回のデータ待ちが 10 秒を超えたら判断する。
 * シーク・source の設定（最初の読み込みと画質の切り替えを含む）・再生の開始のあとは、次の
 * `playing` が来るまでの待ちを数えない。止めたときと、通信の失敗で読み込み直すときは数え直す。
 *
 * 時刻は呼ぶ側が渡す。10 秒の判定は、呼ぶ側がデータ待ちの始まりにタイマーを掛けて
 * `tick` で確かめる（`waitDeadlineMs`）。
 */

/** stallWindowMs はデータ待ちの回数を数える窓の長さである。 */
export const stallWindowMs = 60_000;
/** stallCountThreshold は、窓の中で途切れていると判断するデータ待ちの回数である。 */
export const stallCountThreshold = 3;
/** stallDurationMs は、1 回で途切れていると判断するデータ待ちの長さである。 */
export const stallDurationMs = 10_000;

export interface StallState {
  /**
   * 数えている（前の `playing` のあとで、シーク・source の設定・再生の開始・停止・
   * 読み込み直しがまだ来ていない）。
   */
  counting: boolean;
  /** 数えているデータ待ちが始まった時刻。待っていなければ null。 */
  waitingSinceMs: number | null;
  /** 窓の中に始まった、数えたデータ待ちの始まりの時刻（古い順）。 */
  waitStartsMs: number[];
  /** 途切れていると判断した。再生の終わり・失敗（`clear`）まで下ろさない。 */
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

/** stallStep は、nowMs に届いた event のあとの状態を返す。 */
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
      // 落ち着くまでの待ちは数えない。始まっていた待ちは回数に残し、長さは測らない。
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

/**
 * waitDeadlineMs は、今のデータ待ちが続いたときに 1 回の長さで判断する時刻を返す。
 * 待っていない、またはすでに判断したなら null。
 */
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
