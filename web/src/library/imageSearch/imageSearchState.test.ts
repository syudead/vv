import { describe, expect, it } from "vitest";

import { NetworkFailed, RequestFailed } from "../../api/client";
import type { ImageSearchMatch } from "../../api/imageSearch";
import {
  errorPhase,
  formatElapsed,
  matchedSection,
  matchPath,
  ratePercent,
  refuseFiles,
} from "./imageSearchState";

function file(name: string, type: string) {
  return new File(["x"], name, { type });
}

function refused(reason: string, limit?: number, status = 400) {
  return new RequestFailed(status, "invalid_request", "", { reason, limit });
}

describe("画像検索の送る前の拒否", () => {
  it("2 つ以上のファイルと、画像でも動画でもない型は送らずに断り、型のないファイルはサーバーに任せる", () => {
    expect(refuseFiles([file("a.png", "image/png"), file("b.png", "image/png")])).toEqual(
      {
        title: "Search with one file at a time",
        description: "2 files were given.",
      },
    );
    expect(refuseFiles([file("a.txt", "text/plain")])?.title).toBe(
      "This file can't be searched",
    );
    expect(refuseFiles([file("clip.mp4", "video/mp4")])).toBeNull();
    expect(refuseFiles([file("clip.mkv", "")])).toBeNull();
  });
});

describe("画像検索の失敗の表示", () => {
  const query = file("clip.mp4", "video/mp4");

  it("上限は画素数、バイト数、動画の長さをそれぞれの単位で出す", () => {
    expect(
      errorPhase(query, refused("image_search_too_large", 120_000_000, 413)),
    ).toEqual({
      kind: "refused",
      file: query,
      refusal: {
        title: "This file is too large",
        description: "The limit is 120 megapixels.",
      },
    });
    expect(
      errorPhase(query, refused("image_search_too_large", 4 * 1024 ** 3, 413)),
    ).toMatchObject({ refusal: { description: "The limit is 4.0 GB." } });
    expect(errorPhase(query, refused("image_search_too_long", 3 * 3_600_000))).toEqual({
      kind: "refused",
      file: query,
      refusal: { title: "This video is too long", description: "The limit is 3:00:00." },
    });
  });

  it("送り直して直る失敗だけを failed にする", () => {
    expect(errorPhase(query, new NetworkFailed(new Error("offline")))).toMatchObject({
      kind: "failed",
      refusal: { title: "The search failed" },
    });
    expect(
      errorPhase(query, new RequestFailed(503, "service_unavailable", "")).kind,
    ).toBe("failed");
    expect(
      errorPhase(query, new RequestFailed(400, "invalid_request", "two parts")).kind,
    ).toBe("refused");
  });
});

describe("画像検索の経過時間", () => {
  it("10 秒未満は 10 分の 1 秒まで、1 分未満は秒、それより後は m:ss で読む", () => {
    expect(formatElapsed(849)).toBe("0.8 s");
    expect(formatElapsed(12_400)).toBe("12 s");
    expect(formatElapsed(65_000)).toBe("1:05");
  });
});

/** An image match found at one frame, whose frames hold only that frame. */
function frameMatch(videoId: number, positionMs: number): ImageSearchMatch {
  return {
    videoId,
    rate: 1,
    kind: "frame",
    positionMs,
    band: "strong",
    frames: [{ positionMs, rate: 1 }],
    frameCount: 1,
  };
}

describe("画像検索の結果の表示", () => {
  it("一致率は四捨五入した整数の百分率で、再生は一致した版をその位置の秒で開く", () => {
    expect(ratePercent(51 / 63)).toBe(81);
    expect(ratePercent(0.5)).toBe(50);
    expect(matchPath(frameMatch(7, 12_500))).toBe("/videos/7?t=12.5");
  });
});

describe("画像検索の一致区間のレーン", () => {
  const match = {
    videoId: 8,
    rate: 1,
    kind: "partial",
    positionMs: 100_000,
    startMs: 100_000,
    endMs: 130_000,
    band: "strong",
    frames: [],
    frameCount: 0,
  } satisfies ImageSearchMatch;

  it("区間を一覧の動画の長さのレーンに置き、それより長い版で一致した区間の終わりまでレーンを伸ばす", () => {
    expect(matchedSection(match, 200_000)).toEqual({
      lengthSeconds: 200,
      span: { start: 100, end: 130 },
    });
    expect(matchedSection(match, 120_000)).toEqual({
      lengthSeconds: 130,
      span: { start: 100, end: 130 },
    });
  });

  it("画像の一致と長さの分からない動画にはレーンを出さない", () => {
    expect(matchedSection(frameMatch(8, 0), 200_000)).toBeNull();
    expect(matchedSection(match, undefined)).toBeNull();
  });
});
