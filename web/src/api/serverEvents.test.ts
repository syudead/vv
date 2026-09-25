import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { reloadPage } from "../auth/pageNavigation";
import { setRenderedAudience } from "./client";
import {
  currentEventSource,
  FakeEventSource,
  installFakeEventSource,
} from "./fakeEventSource";
import { subscribeServerEvents } from "./serverEvents";

vi.mock("../auth/pageNavigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../auth/pageNavigation")>()),
  reloadPage: vi.fn(),
}));

function session(state: string): Response {
  return new Response(JSON.stringify({ state }), {
    headers: { "Content-Type": "application/json" },
  });
}

/** close はブラウザがつなぎ直しを諦めた（応答が 200 でない）ことを再現する。 */
function close(): void {
  const source = currentEventSource();
  source.readyState = FakeEventSource.CLOSED;
  source.dispatch("error");
}

describe("subscribeServerEvents の接続が閉じたとき", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let unsubscribe: () => void = () => undefined;

  beforeEach(() => {
    vi.useFakeTimers();
    installFakeEventSource();
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(reloadPage).mockClear();
    setRenderedAudience("owner");
  });

  afterEach(() => {
    unsubscribe();
    fetchMock.mockReset();
    setRenderedAudience(null);
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("セッションが失効して所有者でなくなっていれば、張り直さずに1度だけ読み直す", async () => {
    fetchMock.mockImplementation(() => Promise.resolve(session("guest")));
    unsubscribe = subscribeServerEvents({});
    expect(FakeEventSource.instances).toHaveLength(1);

    close();
    await vi.advanceTimersByTimeAsync(30_000);

    expect(fetchMock).toHaveBeenCalledWith("/api/auth/session", { signal: undefined });
    expect(reloadPage).toHaveBeenCalledOnce();
    expect(FakeEventSource.instances).toHaveLength(1);
  });

  it("所有者のままなら、少し待って張り直す", async () => {
    fetchMock.mockImplementation(() => Promise.resolve(session("owner")));
    unsubscribe = subscribeServerEvents({});

    close();
    await vi.advanceTimersByTimeAsync(3000);

    expect(reloadPage).not.toHaveBeenCalled();
    expect(FakeEventSource.instances).toHaveLength(2);
  });

  it("状態を確かめられなければ（サーバーが落ちている）、少し待って張り直す", async () => {
    fetchMock.mockImplementation(() => Promise.reject(new TypeError("failed to fetch")));
    unsubscribe = subscribeServerEvents({});

    close();
    await vi.advanceTimersByTimeAsync(3000);

    expect(reloadPage).not.toHaveBeenCalled();
    expect(FakeEventSource.instances).toHaveLength(2);
  });
});
