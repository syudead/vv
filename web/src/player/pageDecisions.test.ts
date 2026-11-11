import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import {
  autoplayRequested,
  backTarget,
  closePlayer,
  linkStateFrom,
  resumePosition,
  returnEntry,
  returnState,
  startFromSearch,
} from "./pageDecisions";

function withProgress(progress: Video["progress"]): Video {
  return { progress } as Video;
}

describe("backTarget", () => {
  it("遷移元の一覧 URL を返す", () => {
    expect(backTarget({ from: "/folders/1?sort=name" })).toBe("/folders/1?sort=name");
  });

  it("遷移元が無い・文字列でないときは / を返す", () => {
    expect(backTarget(null)).toBe("/");
    expect(backTarget(undefined)).toBe("/");
    expect(backTarget({})).toBe("/");
    expect(backTarget({ from: 1 })).toBe("/");
  });

  it("外部 URL は受け付けない", () => {
    expect(backTarget({ from: "https://example.com/" })).toBe("/");
    expect(backTarget({ from: "//example.com/" })).toBe("/");
  });
});

describe("returnEntry", () => {
  it("一覧の履歴エントリを次の動画へ渡し、閉じるときに一覧へ返す", () => {
    const back = returnState(
      "/?query=a",
      returnEntry({ from: "/?query=a", returnEntry: "k1" }),
    );

    expect(linkStateFrom(back, [3], 4)).toEqual({
      from: "/?query=a",
      returnEntry: "k1",
      trail: [3, 4],
    });
    expect(closePlayer(back)).toEqual({ to: "/?query=a", state: { returnEntry: "k1" } });
  });

  it("エントリが無い・文字列でないときは閉じても状態を渡さない", () => {
    expect(returnEntry({ returnEntry: 1 })).toBeUndefined();
    expect(returnEntry({ returnEntry: "" })).toBeUndefined();
    expect(closePlayer(returnState("/", returnEntry(null)))).toEqual({
      to: "/",
      state: undefined,
    });
  });
});

describe("autoplayRequested", () => {
  it("autoplay が true のときだけ真", () => {
    expect(autoplayRequested({ autoplay: true })).toBe(true);
    expect(autoplayRequested({ autoplay: "true" })).toBe(false);
    expect(autoplayRequested({})).toBe(false);
    expect(autoplayRequested(null)).toBe(false);
  });
});

describe("startFromSearch", () => {
  it("t の秒をミリ秒で返し、無い・読めない t は null にする", () => {
    expect(startFromSearch("?t=130")).toBe(130_000);
    expect(startFromSearch("?group=1&t=12.5")).toBe(12_500);
    expect(startFromSearch("")).toBeNull();
    expect(startFromSearch("?t=-3")).toBeNull();
    expect(startFromSearch("?t=abc")).toBeNull();
    expect(startFromSearch(`?t=${"9".repeat(400)}`)).toBeNull();
  });
});

describe("resumePosition", () => {
  const updatedAt = "2026-01-01T00:00:00Z";

  it("続きの位置から再生する", () => {
    expect(
      resumePosition(withProgress({ positionMs: 5000, completed: false, updatedAt })),
    ).toBe(5000);
  });

  it("位置が無い・見終えた・見始めたばかりなら先頭から再生する", () => {
    expect(resumePosition(withProgress(undefined))).toBe(0);
    expect(
      resumePosition(withProgress({ positionMs: 60000, completed: true, updatedAt })),
    ).toBe(0);
    expect(
      resumePosition(withProgress({ positionMs: 4999, completed: false, updatedAt })),
    ).toBe(0);
  });
});

describe("resumePosition（集まりのバージョン）", () => {
  const updatedAt = "2026-01-01T00:00:00Z";
  const version = (durationMs: number | undefined, positionMs: number): Video =>
    ({ durationMs, progress: { positionMs, completed: false, updatedAt } }) as Video;

  it("集まりの位置がこのバージョンの尺以上なら先頭から再生する（R-11）", () => {
    expect(resumePosition(version(60_000, 60_000))).toBe(0);
    expect(resumePosition(version(60_000, 90_000))).toBe(0);
  });

  it("尺より手前なら続きから、尺が分からなければ位置のまま再生する", () => {
    expect(resumePosition(version(60_000, 59_999))).toBe(59_999);
    expect(resumePosition(version(undefined, 90_000))).toBe(90_000);
  });
});
