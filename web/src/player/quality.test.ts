import { afterEach, describe, expect, it } from "vitest";

import {
  readPlaybackQuality,
  writePlaybackQuality,
} from "../preferences/playbackQuality";
import { effectiveQuality, qualityOptions, sourceShortSide } from "./quality";

describe("qualityOptions", () => {
  it("動画の短辺より小さい画質だけを大きい順に出す", () => {
    expect(qualityOptions({ width: 1920, height: 1080 })).toEqual([
      "720p",
      "480p",
      "360p",
    ]);
    expect(qualityOptions({ width: 1080, height: 1920 })).toEqual([
      "720p",
      "480p",
      "360p",
    ]);
    expect(qualityOptions({ width: 3840, height: 2160 })).toEqual([
      "1080p",
      "720p",
      "480p",
      "360p",
    ]);
  });

  it("短辺が 360 以下か寸法の無い動画は空にする", () => {
    expect(qualityOptions({ width: 640, height: 360 })).toEqual([]);
    expect(qualityOptions({})).toEqual([]);
    expect(qualityOptions({ width: 1920 })).toEqual([]);
  });

  it("短辺だけで決め、変換の枠による縮小は考えない", () => {
    expect(qualityOptions({ width: 1200, height: 12000 })).toEqual([
      "1080p",
      "720p",
      "480p",
      "360p",
    ]);
  });
});

describe("effectiveQuality", () => {
  afterEach(() => window.localStorage.clear());

  it("選択肢にある覚えた画質はそのまま使い、元の画質は元の画質のまま", () => {
    expect(effectiveQuality("480p", { width: 1920, height: 1080 })).toBe("480p");
    expect(effectiveQuality("original", { width: 1920, height: 1080 })).toBe("original");
  });

  it("覚えた 480p は 640×360 の動画では元の画質になり、保存値は変わらない", () => {
    writePlaybackQuality("480p");
    expect(effectiveQuality(readPlaybackQuality(), { width: 640, height: 360 })).toBe(
      "original",
    );
    expect(effectiveQuality("1080p", { width: 1920, height: 1080 })).toBe("original");
    expect(readPlaybackQuality()).toBe("480p");
  });
});

describe("sourceShortSide", () => {
  it("表示の短辺を画質の名前の形で返し、寸法が無ければ undefined", () => {
    expect(sourceShortSide({ width: 1920, height: 1080 })).toBe("1080p");
    expect(sourceShortSide({ width: 1080, height: 1920 })).toBe("1080p");
    expect(sourceShortSide({ width: 3840, height: 2160 })).toBe("2160p");
    expect(sourceShortSide({})).toBeUndefined();
    expect(sourceShortSide({ width: 0, height: 360 })).toBeUndefined();
  });
});
