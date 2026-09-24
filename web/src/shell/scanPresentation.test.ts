import { describe, expect, it } from "vitest";

import type { Scan } from "../api/client";
import type { ScanContextValue } from "./ScanProvider";
import { presentScan } from "./scanPresentation";

function makeScan(state: Scan["state"], values: Partial<Scan> = {}): Scan {
  return {
    id: 1,
    state,
    total: 10,
    completed: state === "running" ? 4 : 10,
    failed: 0,
    ...values,
  };
}

function context(
  scan: Scan | null,
  values: Partial<ScanContextValue> = {},
): ScanContextValue {
  return {
    scan,
    loaded: true,
    processing: null,
    error: null,
    starting: false,
    running: scan?.state === "running",
    canStart: true,
    start: () => undefined,
    refresh: () => undefined,
    setFolderCount: () => undefined,
    finished: null,
    ...values,
  };
}

describe("presentScan", () => {
  it.each([
    ["not-run", context(null)],
    ["starting", context(null, { starting: true })],
    ["unknown-total", context(makeScan("running", { total: 0 }))],
    ["running", context(makeScan("running"))],
    ["done", context(makeScan("done"))],
    ["partial-failed", context(makeScan("done", { failed: 2 }))],
    ["failed", context(makeScan("failed", { error: "disk" }))],
    ["fetch-failed", context(null, { error: "network" })],
  ] as const)("maps %s", (expected, value) => {
    expect(presentScan(value).state).toBe(expected);
  });

  it("keeps the last scan visible while a refresh is failing", () => {
    const presentation = presentScan(context(makeScan("running"), { error: "network" }));
    expect(presentation.state).toBe("running");
    expect(presentation.refreshing).toBe(true);
    expect(presentation.progress).toBe(0.4);
  });

  it("treats a running zero total as unknown instead of zero items", () => {
    const presentation = presentScan(
      context(makeScan("running", { total: 0, completed: 0 })),
    );
    expect(presentation.state).toBe("unknown-total");
    expect(presentation.total).toBeNull();
    expect(presentation.progress).toBeNull();
  });

  it("スキャンが終わっても準備の残りがあれば、準備中として割合を出さない", () => {
    const presentation = presentScan(
      context(makeScan("done"), {
        processing: { probe: 2, thumbnail: 1, preview: 0 },
      }),
    );
    expect(presentation.state).toBe("preparing");
    expect(presentation.remaining).toBe(3);
    expect(presentation.progress).toBeNull();
    expect(presentation.description).toBe("取り込んだ動画を準備中（残り 3 件）");
  });

  it("準備の残りが無くなれば完了になる", () => {
    const presentation = presentScan(
      context(makeScan("done"), {
        processing: { probe: 0, thumbnail: 0, preview: 0 },
      }),
    );
    expect(presentation.state).toBe("done");
  });

  it("取り込み自体の失敗は、準備の残りがあっても失敗として示す", () => {
    const presentation = presentScan(
      context(makeScan("failed", { error: "読めません" }), {
        processing: { probe: 1, thumbnail: 0, preview: 0 },
      }),
    );
    expect(presentation.state).toBe("failed");
  });
});
