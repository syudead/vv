import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";
import { ScanProvider, useScan } from "./ScanProvider";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function scan(id: number, state: Scan["state"]): Scan {
  return { id, state, total: 1, completed: state === "running" ? 0 : 1, failed: 0 };
}

function Harness() {
  const value = useScan();
  return (
    <>
      <button type="button" onClick={value.start}>
        開始
      </button>
      <button type="button" onClick={value.refresh}>
        更新
      </button>
      <button type="button" onClick={() => value.setFolderCount(1)}>
        フォルダ追加を反映
      </button>
      <p>{value.error ?? "エラーなし"}</p>
      <p>完了: {value.finished?.id ?? "なし"}</p>
      <p>開始可否: {value.canStart ? "可" : "不可"}</p>
    </>
  );
}

describe("ScanProvider", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("開始要求の失敗を状態取得の成功で消さない", async () => {
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") {
        return Promise.resolve(
          json({ code: "internal", message: "開始できません" }, 500),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );
    await screen.findByText("エラーなし");

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(
      await screen.findByText("取り込みを始められません: 開始できません"),
    ).toBeDefined();
    const currentCalls = fetchMock.mock.calls.filter(
      ([input]) => String(input) === "/api/scans/current",
    );
    expect(currentCalls.length).toBeGreaterThanOrEqual(2);
  });

  it("開始応答を失っても新しい取り込みの完了を追跡する", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") {
        return Promise.resolve(
          json({ code: "internal", message: "応答を失いました" }, 500),
        );
      }
      currentCalls += 1;
      return Promise.resolve(json(scan(currentCalls === 1 ? 1 : 2, "done")));
    });
    const user = userEvent.setup();
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );
    await waitFor(() => expect(currentCalls).toBe(1));

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(await screen.findByText("完了: 2")).toBeDefined();
    expect(screen.getByText("エラーなし")).toBeDefined();
  });

  it("初回状態取得前に開始して高速完了した取り込みを通知する", async () => {
    let currentCalls = 0;
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") return Promise.resolve(json(scan(2, "running"), 202));
      currentCalls += 1;
      if (currentCalls === 1) return new Promise<Response>(() => undefined);
      return Promise.resolve(json(scan(2, "done")));
    });
    const user = userEvent.setup();
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );
    await waitFor(() => expect(currentCalls).toBe(1));

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(await screen.findByText("完了: 2")).toBeDefined();
  });

  it("通常の取り込み追跡は一時的な状態取得失敗後も再開する", async () => {
    vi.useFakeTimers();
    let currentCalls = 0;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      currentCalls += 1;
      if (currentCalls === 1) return Promise.resolve(json(scan(3, "running")));
      if (currentCalls === 2) return Promise.reject(new Error("一時的な失敗"));
      return Promise.resolve(json(scan(3, "done")));
    });
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );
    await act(async () => Promise.resolve());

    await act(async () => vi.advanceTimersByTimeAsync(2000));
    expect(currentCalls).toBe(2);
    await act(async () => vi.advanceTimersByTimeAsync(2000));

    expect(screen.getByText("完了: 3")).toBeDefined();
    vi.useRealTimers();
  });

  it("再確認で別タブが完了した取り込みを通知する", async () => {
    let currentId = 1;
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders"
          ? json([{}])
          : json(scan(currentId, "done")),
      ),
    );
    const user = userEvent.setup();
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );
    await screen.findByText("完了: なし");

    currentId = 2;
    await user.click(screen.getByRole("button", { name: "更新" }));

    expect(await screen.findByText("完了: 2")).toBeDefined();
  });

  it("フォルダ0件では設定画面を開かなくても開始要求を送らない", async () => {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input) === "/api/media-folders" ? json([]) : json({}, 404)),
    );
    const user = userEvent.setup();
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );

    expect(await screen.findByText("開始可否: 不可")).toBeDefined();
    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
  });

  it("windowへ戻ったとき別タブで削除された最後のフォルダを反映する", async () => {
    let folders = [{}];
    fetchMock.mockImplementation((input) =>
      Promise.resolve(
        String(input) === "/api/media-folders" ? json(folders) : json({}, 404),
      ),
    );
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );
    expect(await screen.findByText("開始可否: 可")).toBeDefined();

    folders = [];
    window.dispatchEvent(new Event("focus"));

    expect(await screen.findByText("開始可否: 不可")).toBeDefined();
  });

  it("開始時にフォルダ未設定なら開始可否も不可へ同期する", async () => {
    fetchMock.mockImplementation((input, init) => {
      if (String(input) === "/api/media-folders") return Promise.resolve(json([{}]));
      if (init?.method === "POST") {
        return Promise.resolve(
          json(
            {
              code: "media_folders_not_configured",
              message: "メディアフォルダが設定されていません",
            },
            409,
          ),
        );
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );
    expect(await screen.findByText("開始可否: 可")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "開始" }));

    expect(await screen.findByText("開始可否: 不可")).toBeDefined();
  });

  it("起動時の遅い応答で後発のフォルダ件数を上書きしない", async () => {
    let resolveFolders: ((response: Response) => void) | undefined;
    fetchMock.mockImplementation((input) => {
      if (String(input) === "/api/media-folders") {
        return new Promise<Response>((resolve) => {
          resolveFolders = resolve;
        });
      }
      return Promise.resolve(json({}, 404));
    });
    const user = userEvent.setup();
    render(
      <ScanProvider>
        <Harness />
      </ScanProvider>,
    );

    await user.click(screen.getByRole("button", { name: "フォルダ追加を反映" }));
    expect(screen.getByText("開始可否: 可")).toBeDefined();
    resolveFolders?.(json([]));

    await waitFor(() => expect(screen.getByText("開始可否: 可")).toBeDefined());
  });
});
