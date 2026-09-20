import { render, screen, waitFor } from "@testing-library/react";
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
      <p>{value.error ?? "エラーなし"}</p>
      <p>完了: {value.finished?.id ?? "なし"}</p>
    </>
  );
}

describe("ScanProvider", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("開始要求の失敗を状態取得の成功で消さない", async () => {
    fetchMock.mockImplementation((input, init) => {
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
});
