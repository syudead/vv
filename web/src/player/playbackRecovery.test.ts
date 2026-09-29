import { afterEach, describe, expect, it, vi } from "vitest";

import {
  classifyMediaError,
  reconnectDelay,
  reconnectDelaysMs,
  probeMediaSource,
} from "./playbackRecovery";

describe("classifyMediaError", () => {
  it("通信の失敗と打ち切りは network、読めないデータは decode", () => {
    expect(classifyMediaError(1)).toBe("network");
    expect(classifyMediaError(2)).toBe("network");
    expect(classifyMediaError(3)).toBe("decode");
  });

  it("形式に対応していない・番号の無い誤りは番号だけでは決めない", () => {
    expect(classifyMediaError(4)).toBe("ambiguous");
    expect(classifyMediaError(-1)).toBe("ambiguous");
    expect(classifyMediaError(undefined)).toBe("ambiguous");
  });
});

describe("reconnectDelay", () => {
  it("間を広げながら約 30 秒試し、使い切ったら null", () => {
    expect(reconnectDelaysMs.reduce((sum, delay) => sum + delay, 0)).toBe(30_000);
    expect(reconnectDelay(0)).toBe(1000);
    expect(reconnectDelay(4)).toBe(15_000);
    expect(reconnectDelay(5)).toBeNull();
  });
});

describe("probeMediaSource", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("直接再生は動画本体の先頭 1 バイトを要求し、中身が返れば ok", async () => {
    const fetchMock = vi.fn(() => Promise.resolve(new Response("x", { status: 206 })));
    vi.stubGlobal("fetch", fetchMock);
    await expect(probeMediaSource("/api/videos/7/stream")).resolves.toBe("ok");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/videos/7/stream",
      expect.objectContaining({ headers: { Range: "bytes=0-0" } }),
    );
  });

  it("届いたが誤りの状態なら error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(new Response("", { status: 404 }))),
    );
    await expect(probeMediaSource("/api/videos/7/stream")).resolves.toBe("error");
  });

  it("変換では /api/health でサーバーに届くかだけを確かめる", async () => {
    const fetchMock = vi.fn(() => Promise.resolve(new Response("{}")));
    vi.stubGlobal("fetch", fetchMock);
    await expect(probeMediaSource(null)).resolves.toBe("ok");
    expect(fetchMock).toHaveBeenCalledWith("/api/health", expect.anything());
  });

  it("要求そのものが失敗したら届かない", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.reject(new TypeError("Failed to fetch"))),
    );
    await expect(probeMediaSource(null)).resolves.toBe("unreachable");
  });

  it("端末が回線につながっていなければ要求を出さない", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    await expect(probeMediaSource("/api/videos/7/stream")).resolves.toBe("unreachable");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
