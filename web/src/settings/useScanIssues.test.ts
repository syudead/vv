import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import type { Scan } from "../api/client";
import { useScanIssues } from "./useScanIssues";

afterEach(() => {
  vi.unstubAllGlobals();
});

function scan(revision: number): Scan {
  return {
    id: 2,
    origin: "manual",
    status: "running",
    videos: { total: 1, settled: 0 },
    issues: { failed: 1, substituted: 0, revision },
    state: "running",
  };
}

test("a failing issues fetch retries on a few status refreshes per revision", async () => {
  // A distinct status per call makes each settled failure observable as new error text.
  let calls = 0;
  const fetch = vi.fn(() => {
    calls += 1;
    return Promise.resolve(new Response("", { status: 500 + calls }));
  });
  vi.stubGlobal("fetch", fetch);
  const { result, rerender } = renderHook(({ value }) => useScanIssues(value, () => {}), {
    initialProps: { value: scan(1) },
  });
  await waitFor(() => expect(result.current.error).not.toBeNull());

  for (let retry = 1; retry <= 3; retry += 1) {
    const failure = result.current.error;
    rerender({ value: scan(1) });
    await waitFor(() => expect(result.current.error).not.toBe(failure));
  }
  rerender({ value: scan(1) });
  await act(() => Promise.resolve());
  expect(fetch).toHaveBeenCalledTimes(4);

  rerender({ value: scan(2) });
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(5));
});
