import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";

/**
 * ScanStatus が知らせるのは「終わっている取り込みを観測した」ことだけである
 * （contracts/screen-states.md 1.「固定の帯」）。
 *
 * `GET /api/scans/current` は実行中のものが無ければ最後に終わったものを返すので、
 * この部品からは「いま終わった」のか「前回の残り」なのかを区別できない。区別は
 * 一覧側（LibraryPage）が id で行う ── そちらの検査は
 * `src/pages/LibraryPage.restore.test.tsx` にある。
 *
 * ここで確かめるのは、観測を取りこぼさないことと、押した操作の失敗が消えない
 * ことである。**この部品は画面をまたぐ状態を持たない**ので、検査の順序にも
 * 依存しない。
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
function scan(state: Scan["state"], completed = 0, id = 1): Scan {
  return {
    id,
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

/** ids は知らせてきた取り込みの id を並べる。 */
function ids(mock: ReturnType<typeof vi.fn>): number[] {
  return mock.mock.calls.map((call) => (call[0] as Scan).id);
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
  it("終わっている取り込みを観測したら、そのまま知らせる", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 3, 10));

    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    expect(screen.getByText("前回の取り込みで 3 件を反映")).toBeDefined();
    // 「いま終わった」のか「前回の残り」なのかは、ここでは決めない。
    expect(ids(onFinished)).toEqual([10]);
  });

  it("実行中のあいだは知らせず、終わってから知らせる", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValueOnce(scan("running", 1, 11));
    getCurrentScan.mockResolvedValue(scan("done", 3, 11));

    render(<ScanStatus onFinished={onFinished} />);
    await settle();
    expect(ids(onFinished)).toEqual([]);
    expect(screen.getByText("取り込み中 1 / 3 件")).toBeDefined();

    await act(async () => {
      vi.advanceTimersByTime(pollInterval);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(ids(onFinished)).toEqual([11]);
  });

  it("状態を取得できなければ何も知らせない", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockRejectedValue(new Error("つながりません"));

    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    expect(ids(onFinished)).toEqual([]);
    expect(
      screen.getByText("取り込みの状態を取得できません: つながりません"),
    ).toBeDefined();
  });
});

describe("ScanStatus の「取り込む」", () => {
  it("始められなかったことは、直後の巡回が成功しても消えない", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 2, 10));
    startScan.mockRejectedValue(new Error("つながりません"));

    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    await user.click(screen.getByRole("button", { name: "取り込む" }));
    await settle();

    // 消えると、押した利用者には始まったように見える。状態の取得の失敗とは
    // 別の失敗なので、別の文言で残す。
    expect(screen.getByText("取り込みを始められません: つながりません")).toBeDefined();
  });

  it("実は始まっていたら、失敗の文言を引っ込めて進捗を出す", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 2, 10));
    // サーバーは取り込みを始めたが、202 が返る前に接続が切れた。
    startScan.mockRejectedValue(new Error("つながりません"));

    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    getCurrentScan.mockResolvedValue(scan("running", 1, 11));
    await user.click(screen.getByRole("button", { name: "取り込む" }));
    await settle();

    // 進んでいるのに「始められません」と出し続けると、利用者は失敗したと
    // 思って押し直す。要求の失敗は「始まらなかった」ことを保証しない。
    expect(screen.queryByText(/取り込みを始められません/)).toBeNull();
    expect(screen.getByText("取り込み中 1 / 3 件")).toBeDefined();
  });

  it("同じ取り込みのままなら、失敗の文言は残る", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 2, 10));
    startScan.mockRejectedValue(new Error("つながりません"));

    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    // 巡回は成功するが、見えるのは押す前と同じ取り込みである。
    await user.click(screen.getByRole("button", { name: "取り込む" }));
    await settle();

    expect(screen.getByText("取り込みを始められません: つながりません")).toBeDefined();
  });

  it("応答が失われても見張り直すので、始まっていれば気付ける", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 2, 10));
    // サーバーは取り込みを始めたが、202 が返る前に接続が切れた。
    startScan.mockRejectedValue(new Error("つながりません"));

    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(<ScanStatus onFinished={onFinished} />);
    await settle();
    expect(ids(onFinished)).toEqual([10]);

    // 押したあとの巡回では、始まっている取り込み（別の id）が既に終わっている。
    getCurrentScan.mockResolvedValue(scan("done", 5, 11));
    await user.click(screen.getByRole("button", { name: "取り込む" }));
    await settle();

    // 要求の失敗は「始まらなかった」ことを保証しない。見張り直さないと、
    // 始まった取り込みに気付く機会が無くなる。
    expect(ids(onFinished)).toEqual([10, 11]);
  });

  it("始められたら、終わったときに知らせる", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 2, 10));
    startScan.mockResolvedValue(scan("running", 0, 11));

    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    getCurrentScan.mockResolvedValue(scan("done", 4, 11));
    await user.click(screen.getByRole("button", { name: "取り込む" }));
    await settle();

    expect(ids(onFinished)).toEqual([10, 11]);
  });
});
