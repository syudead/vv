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
});
