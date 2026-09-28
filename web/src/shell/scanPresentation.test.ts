import { describe, expect, it } from "vitest";

import type { Scan } from "../api/client";
import type { UiText } from "../i18n";
import type { ScanContextValue } from "./ScanProvider";
import { presentScan, statusAnnouncement } from "./scanPresentation";

function makeScan(status: Scan["status"], values: Partial<Scan> = {}): Scan {
  return {
    id: 1,
    status,
    videos:
      status === "finding"
        ? undefined
        : { total: 10, settled: status === "running" ? 4 : 10 },
    issues: { failed: 0, substituted: 0, revision: 0 },
    state: status === "finding" ? "running" : status === "failed" ? "failed" : "done",
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
    activity: scan?.activity ?? null,
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
    ["finding", context(makeScan("finding"))],
    ["running", context(makeScan("running"))],
    ["done", context(makeScan("done"))],
    [
      "partial",
      context(
        makeScan("partial", { issues: { failed: 2, substituted: 0, revision: 1 } }),
      ),
    ],
    ["failed", context(makeScan("failed", { error: "disk" }))],
    ["fetch-failed", context(null, { error: "network" as UiText })],
  ] as const)("maps %s", (expected, value) => {
    expect(presentScan(value).state).toBe(expected);
  });

  it("finding のあいだは割合も数字も出さず、不確定のバーと「Looking for files…」にする", () => {
    const presentation = presentScan(context(makeScan("finding")));
    expect(presentation.videos).toBeNull();
    expect(presentation.progressText).toBeNull();
    expect(presentation.bar).toBe("indeterminate");
    expect(presentation.statusText).toBe("Scanning");
    expect(presentation.detail?.text).toBe("Looking for files…");
  });

  it("running のあいだは本数による1つの進み具合と今の処理を示す", () => {
    const presentation = presentScan(
      context(
        makeScan("running", {
          // 走査は閉じ、準備だけが残っている。
          state: "done",
          activity: {
            kind: "thumbnail",
            fileName: "movie.mp4",
            folder: { rootId: 1, path: "trips/2026", rootName: "media" },
            videoId: 7,
          },
        }),
      ),
    );
    expect(presentation.statusText).toBe("Scanning");
    expect(presentation.videos).toEqual({ settled: 4, total: 10 });
    expect(presentation.progressText).toBe("4 of 10 videos done");
    expect(presentation.bar).toBe("determinate");
    expect(presentation.detail).toEqual({
      text: "Creating the thumbnail · movie.mp4",
      title: "media / trips/2026 / movie.mp4",
    });
  });

  it("videos.total = 0 の完了は、変化が無かったことを示し、バーも数字も出さない", () => {
    const presentation = presentScan(
      context(
        makeScan("done", {
          videos: { total: 0, settled: 0 },
          settledAt: "2026-09-28T06:04:00Z",
        }),
      ),
    );
    expect(presentation.progressText).toBe("No changed files were found.");
    expect(presentation.videos).toBeNull();
    expect(presentation.bar).toBe("none");
    expect(presentation.detail?.text).toMatch(/^Finished /);
  });

  it("完了の時刻は settledAt から書き、実行中は時刻を出さない", () => {
    const done = presentScan(
      context(makeScan("done", { settledAt: "2026-09-28T06:04:00Z" })),
    );
    expect(done.detail?.text).toMatch(/^Finished .*2026/);
    const running = presentScan(context(makeScan("running")));
    expect(running.detail).toBeNull();
  });

  it("失敗では時刻の代わりに、終われなかったことを示す", () => {
    const presentation = presentScan(context(makeScan("failed")));
    expect(presentation.statusText).toBe("Scan failed");
    expect(presentation.detail?.text).toBe("The scan couldn't finish.");
  });

  it("仕事の件数や段階ごとの内訳を持たない", () => {
    const presentation = presentScan(context(makeScan("running")));
    expect(Object.keys(presentation).sort()).toEqual(
      [
        "bar",
        "detail",
        "error",
        "issues",
        "progressText",
        "refreshing",
        "scan",
        "state",
        "statusText",
        "videos",
      ].sort(),
    );
  });

  it("keeps the last scan visible while a refresh is failing", () => {
    const presentation = presentScan(
      context(makeScan("running"), { error: "network" as UiText }),
    );
    expect(presentation.state).toBe("running");
    expect(presentation.refreshing).toBe(true);
    expect(presentation.videos).toEqual({ settled: 4, total: 10 });
  });
});

describe("statusAnnouncement", () => {
  it("完了・一部失敗・失敗だけを読み上げる", () => {
    expect(statusAnnouncement(presentScan(context(null, { starting: true })))).toBeNull();
    expect(statusAnnouncement(presentScan(context(makeScan("finding"))))).toBeNull();
    expect(statusAnnouncement(presentScan(context(makeScan("running"))))).toBeNull();
    expect(statusAnnouncement(presentScan(context(makeScan("done"))))).toBe(
      "The scan is complete.",
    );
    expect(
      statusAnnouncement(
        presentScan(
          context(
            makeScan("done", { issues: { failed: 0, substituted: 2, revision: 1 } }),
          ),
        ),
      ),
    ).toBe("The scan is complete. 2 videos are worth checking.");
    expect(
      statusAnnouncement(
        presentScan(
          context(
            makeScan("partial", { issues: { failed: 1, substituted: 0, revision: 1 } }),
          ),
        ),
      ),
    ).toBe("The scan finished with some failures. 1 video may not be usable.");
    expect(statusAnnouncement(presentScan(context(makeScan("failed"))))).toBe(
      "The scan failed.",
    );
  });

  it("今の処理が変わっても、読み上げは変わらない", () => {
    const first = presentScan(
      context(makeScan("running", { activity: { kind: "probe", fileName: "a.mp4" } })),
    );
    const second = presentScan(
      context(
        makeScan("running", {
          videos: { total: 10, settled: 5 },
          activity: { kind: "preview", fileName: "b.mp4" },
        }),
      ),
    );
    expect(statusAnnouncement(first)).toBeNull();
    expect(statusAnnouncement(second)).toBeNull();
  });
});
