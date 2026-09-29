import { describe, expect, it } from "vitest";

import {
  initialStallState,
  stallStep,
  waitDeadlineMs,
  type StallEvent,
  type StallState,
} from "./stallMonitor";

/** run は [時刻（秒）, 出来事] の並びを順に渡した状態を返す。 */
function run(steps: [number, StallEvent][], state: StallState = initialStallState) {
  return steps.reduce(
    (current, [s, event]) => stallStep(current, event, s * 1000),
    state,
  );
}

/** 再生を始めて落ち着いた（最初の playing が来た）状態。 */
const settled = run([
  [0, "loadstart"],
  [0, "play"],
  [0, "waiting"],
  [1, "playing"],
]);

/** at 秒に始まり 1 秒で戻るデータ待ち。 */
const wait = (at: number): [number, StallEvent][] => [
  [at, "waiting"],
  [at + 1, "playing"],
];

describe("stallMonitor", () => {
  it("60 秒の中で 3 回のデータ待ちで判断し、2 回では判断しない", () => {
    const two = run([...wait(10), ...wait(30)], settled);
    expect(two.stalled).toBe(false);
    expect(run(wait(60), two).stalled).toBe(true);
  });

  it("61 秒前に始まった待ちは数えない", () => {
    const state = run([...wait(10), ...wait(50), ...wait(71)], settled);
    expect(state.waitStartsMs).toEqual([50_000, 71_000]);
    expect(state.stalled).toBe(false);
  });

  it("1 回の待ちが 10 秒を超えたら判断する", () => {
    const waiting = run([[20, "waiting"]], settled);
    expect(waitDeadlineMs(waiting)).toBe(30_000);
    expect(stallStep(waiting, "tick", 29_999).stalled).toBe(false);
    expect(stallStep(waiting, "tick", 30_000).stalled).toBe(true);
    // タイマーが遅れても、戻ったときの長さで判断する。
    expect(stallStep(waiting, "playing", 31_000).stalled).toBe(true);
    // 10 秒に届かずに戻れば判断しない。
    const back = stallStep(waiting, "playing", 29_000);
    expect(back.stalled).toBe(false);
    expect(waitDeadlineMs(back)).toBeNull();
  });

  it.each<StallEvent>(["seeking", "loadstart", "play"])(
    "%s の直後の最初の待ちは数えない",
    (event) => {
      const state = run(
        [
          [20, event],
          [20, "waiting"],
          [40, "tick"],
        ],
        settled,
      );
      expect(state.waitStartsMs).toEqual([]);
      expect(state.stalled).toBe(false);
      expect(waitDeadlineMs(state)).toBeNull();
      // 次の playing のあとの待ちは数える。
      expect(
        run(
          [
            [41, "playing"],
            [50, "waiting"],
          ],
          state,
        ).waitStartsMs,
      ).toEqual([50_000]);
    },
  );

  it("始めたばかりのプレイヤーの待ちは数えない", () => {
    expect(
      run([
        [0, "waiting"],
        [20, "tick"],
      ]).stalled,
    ).toBe(false);
  });

  it("待ちの途中のシークで、その待ちの長さは測らない", () => {
    const state = run(
      [
        [20, "waiting"],
        [25, "seeking"],
        [40, "tick"],
      ],
      settled,
    );
    expect(state.stalled).toBe(false);
    expect(state.waitStartsMs).toEqual([20_000]);
  });

  it.each<StallEvent>(["pause", "recovering"])(
    "%s の間の待ちは数えず、それまでの回数を数え直す",
    (event) => {
      const before = run([...wait(10), ...wait(20)], settled);
      const stopped = run(
        [
          [25, event],
          [26, "waiting"],
          [40, "tick"],
        ],
        before,
      );
      expect(stopped.waitStartsMs).toEqual([]);
      expect(stopped.stalled).toBe(false);
      // 戻ってからの 1 回では、前の 2 回と合わせて判断しない。
      const resumed = run([[41, "play"], [42, "playing"], ...wait(45)], stopped);
      expect(resumed.stalled).toBe(false);
    },
  );

  it("判断は再生の終わり・失敗の clear まで保つ", () => {
    const stalled = run([...wait(10), ...wait(20), ...wait(30)], settled);
    expect(stalled.stalled).toBe(true);
    expect(
      run(
        [
          [40, "pause"],
          [41, "seeking"],
        ],
        stalled,
      ).stalled,
    ).toBe(true);
    expect(stallStep(stalled, "clear", 50_000)).toEqual(initialStallState);
  });
});
