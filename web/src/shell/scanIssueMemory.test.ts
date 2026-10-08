import { afterEach, describe, expect, it, vi } from "vitest";

import type { ScanIssue } from "../api/client";
import {
  failureKeys,
  forgetFailureKeys,
  hasNewFailure,
  previousFailureKeys,
  readFailureKeys,
  rememberFailureKeys,
} from "./scanIssueMemory";

function issue(
  fileName: string,
  kinds: ScanIssue["kinds"],
  severity: ScanIssue["severity"] = "failed",
  path = "a",
): ScanIssue {
  return { severity, kinds, fileName, folder: { rootId: 1, path } };
}

describe("failureKeys and hasNewFailure", () => {
  it("counts only failed items, keyed by path and kind", () => {
    const keys = failureKeys([
      issue("x.mp4", ["probe_failed"]),
      issue("y.mp4", ["thumbnail_first_frame"], "substituted"),
    ]);
    expect(keys.size).toBe(1);
  });

  it("announces when there is no earlier list", () => {
    expect(hasNewFailure(failureKeys([]), null)).toBe(true);
  });

  it("does not announce a failure that was already read", () => {
    const earlier = failureKeys([issue("x.mp4", ["probe_failed"])]);
    expect(hasNewFailure(failureKeys([issue("x.mp4", ["probe_failed"])]), earlier)).toBe(
      false,
    );
    expect(hasNewFailure(failureKeys([]), earlier)).toBe(false);
  });

  it("announces a failure at a new path or with a changed kind", () => {
    const earlier = failureKeys([issue("x.mp4", ["probe_failed"])]);
    expect(
      hasNewFailure(
        failureKeys([issue("x.mp4", ["probe_failed"]), issue("z.mp4", ["unreadable"])]),
        earlier,
      ),
    ).toBe(true);
    expect(
      hasNewFailure(failureKeys([issue("x.mp4", ["preview_failed"])]), earlier),
    ).toBe(true);
    expect(
      hasNewFailure(
        failureKeys([issue("x.mp4", ["probe_failed"], "failed", "b")]),
        earlier,
      ),
    ).toBe(true);
  });
});

describe("readFailureKeys", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    forgetFailureKeys();
  });

  it("reads every page and stops when the scan changed", async () => {
    const pages = [
      { scanId: 5, items: [issue("a.mp4", ["unreadable"])], nextCursor: "c1" },
      { scanId: 5, items: [issue("b.mp4", ["unreadable"])] },
    ];
    let call = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify(pages[call++]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      ),
    );
    const keys = await readFailureKeys(5, new AbortController().signal);
    expect(keys?.size).toBe(2);

    call = 0;
    expect(await readFailureKeys(6, new AbortController().signal)).toBeNull();
  });

  it("follows every cursor, however many pages there are", async () => {
    let call = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(() => {
        call += 1;
        return Promise.resolve(
          new Response(
            JSON.stringify({
              scanId: 5,
              items: [issue(`f${String(call)}.mp4`, ["unreadable"])],
              ...(call < 40 ? { nextCursor: `c${String(call)}` } : {}),
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }),
    );
    const keys = await readFailureKeys(5, new AbortController().signal);
    expect(keys?.size).toBe(40);
  });

  it("rejects a list whose cursor does not advance instead of reading it as complete", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ scanId: 5, items: [], nextCursor: "same" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      ),
    );
    await expect(readFailureKeys(5, new AbortController().signal)).rejects.toThrow();
  });

  it("does not let an older scan overwrite a newer list", () => {
    rememberFailureKeys(9, new Set(["new"]));
    rememberFailureKeys(8, new Set(["old"]));
    expect(previousFailureKeys()?.has("new")).toBe(true);
  });

  it("remembers the last list", () => {
    expect(previousFailureKeys()).toBeNull();
    rememberFailureKeys(3, new Set(["k"]));
    expect(previousFailureKeys()?.has("k")).toBe(true);
  });
});
