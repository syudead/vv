import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";

/**
 * 取り込みの終わりを知らせる相手を選ぶ（contracts/screen-states.md 1.「固定の帯」）。
 *
 * `GET /api/scans/current` は実行中のものが無ければ**最後に終わったもの**を返す。
 * 最初の巡回で done が返るのはふつうの状態であって「いま終わった」のではない。
 * ここを取り違えると一覧を開くたびに読み直しが走り、一覧の復元（FR-016）が
 * 毎回捨てられる。
 */

const { getCurrentScan, startScan } = vi.hoisted(() => ({
  getCurrentScan: vi.fn(),
  startScan: vi.fn(),
}));

vi.mock("../api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/client")>()),
  getCurrentScan,
  startScan,
}));

const { default: ScanStatus } = await import("./ScanStatus");

/** pollInterval は ScanStatus の巡回間隔と同じ値である。 */
const pollInterval = 2000;

/** scan は取り込みの状態を作る。 */
function scan(state: Scan["state"], completed = 0): Scan {
  return {
    id: 1,
    state,
    total: 3,
    completed,
    failed: 0,
    startedAt: "2026-09-13T00:00:00Z",
  };
}

/** settle は巡回 1 回ぶんの往復を終わらせる。 */
async function settle(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  getCurrentScan.mockReset();
  startScan.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("ScanStatus の onFinished", () => {
  it("最初から done なら知らせない（前回の取り込みが残っているだけ）", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 3));

    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    expect(screen.getByText("前回の取り込みで 3 件を反映")).toBeDefined();
    // ここで知らせると、一覧を開くたびに読み直しが走る。
    expect(onFinished.mock.calls).toHaveLength(0);
  });

  it("実行中を見たあとに終わったら知らせる", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValueOnce(scan("running", 1));
    getCurrentScan.mockResolvedValue(scan("done", 3));

    render(<ScanStatus onFinished={onFinished} />);
    await settle();
    expect(onFinished.mock.calls).toHaveLength(0);

    await act(async () => {
      vi.advanceTimersByTime(pollInterval);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(onFinished.mock.calls).toHaveLength(1);
  });

  it("利用者が促した取り込みは、巡回より先に終わっていても知らせる", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 2));
    startScan.mockResolvedValue(scan("running", 0));

    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(<ScanStatus onFinished={onFinished} />);
    await settle();
    expect(onFinished.mock.calls).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "取り込む" }));
    await settle();

    // 小さなライブラリでは、押した直後の巡回で既に done になっている。
    expect(onFinished.mock.calls).toHaveLength(1);
  });
});
