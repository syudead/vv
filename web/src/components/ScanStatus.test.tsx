import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Scan } from "../api/client";

/**
 * 取り込みの終わりを知らせる相手を選ぶ（contracts/screen-states.md 1.「固定の帯」）。
 *
 * `GET /api/scans/current` は実行中のものが無ければ**最後に終わったもの**を返す。
 * 最初の巡回で done が返るのはふつうの状態であって「いま終わった」のではない。
 * ここを取り違えると一覧を開くたびに読み直しが走り、一覧の復元（FR-016）が
 * 毎回捨てられる。逆に部品の中だけで覚えると、見ていない間に終わった取り込みを
 * 知らせそこなう。
 *
 * 「終わりを見届ける対象がある」という印は部品より長く生きる（モジュール変数）。
 * したがって**この 5 つは書いてある順に走る前提**である。どの検査も印を立てたら
 * 同じ検査の中で使い切るので、順に走るかぎり持ち越さない。
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

  it("見ていない間に終わった取り込みも、戻ってきたときに知らせる", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("running", 1));

    // 一覧で実行中を見る。
    render(<ScanStatus onFinished={onFinished} />);
    await settle();
    expect(onFinished.mock.calls).toHaveLength(0);

    // 再生画面へ移る（ScanStatus は捨てられる）。その間に取り込みが終わる。
    cleanup();
    getCurrentScan.mockResolvedValue(scan("done", 4));

    // 一覧へ戻る。新しい ScanStatus は完了済みしか見られないが、見届ける
    // 相手がいたことは覚えている。ここで知らせないと、控えから戻した一覧が
    // 増えた動画を出さないままになる。
    render(<ScanStatus onFinished={onFinished} />);
    await settle();

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

  // 印を持ち越さないことを確かめる検査なので、最後に置く。
  it("取り込みを始められなかったら印を残さない", async () => {
    const onFinished = vi.fn();
    getCurrentScan.mockResolvedValue(scan("done", 2));
    startScan.mockRejectedValue(new Error("つながりません"));

    const user = userEvent.setup({ delay: null, advanceTimers: vi.advanceTimersByTime });
    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    await user.click(screen.getByRole("button", { name: "取り込む" }));
    await settle();
    expect(
      screen.getByText(/取り込みの状態を取得できません|つながりません/),
    ).toBeDefined();

    // 始まっていないのだから、見届ける相手もいない。ここで印が残ると、
    // 次にこの部品が作られたときに前回の done を「いま終わった」と誤り、
    // 一覧の控えを捨ててしまう。
    cleanup();
    render(<ScanStatus onFinished={onFinished} />);
    await settle();

    expect(onFinished.mock.calls).toHaveLength(0);
  });
});
