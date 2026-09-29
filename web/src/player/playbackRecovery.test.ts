import { afterEach, describe, expect, it, vi } from "vitest";

import {
  classifyMediaError,
  reconnectDelay,
  reconnectDelaysMs,
  serverReachable,
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

describe("serverReachable", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("応答があれば、状態の番号によらず届く", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(new Response("", { status: 503 }))),
    );
    await expect(serverReachable()).resolves.toBe(true);
  });

  it("要求そのものが失敗したら届かない", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.reject(new TypeError("Failed to fetch"))),
    );
    await expect(serverReachable()).resolves.toBe(false);
  });

  it("端末が回線につながっていなければ要求を出さない", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    await expect(serverReachable()).resolves.toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
