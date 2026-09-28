import { describe, expect, it } from "vitest";

import type { Scan } from "../api/client";
import type { UiText } from "../i18n";
import type { ScanContextValue } from "./ScanProvider";
import { presentScan } from "./scanPresentation";

function makeScan(state: Scan["state"], values: Partial<Scan> = {}): Scan {
  return {
    id: 1,
    status: state,
    issues: { failed: 0, substituted: 0, revision: 0 },
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

const idle = { probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 };

describe("presentScan", () => {
  it.each([
    ["not-run", context(null)],
    ["starting", context(null, { starting: true })],
    ["unknown-total", context(makeScan("running", { total: 0 }))],
    ["running", context(makeScan("running"))],
    ["done", context(makeScan("done"), { processing: idle })],
    ["partial-failed", context(makeScan("done", { failed: 2 }), { processing: idle })],
    ["failed", context(makeScan("failed", { error: "disk" }))],
    ["fetch-failed", context(null, { error: "network" as UiText })],
  ] as const)("maps %s", (expected, value) => {
    expect(presentScan(value).state).toBe(expected);
  });

  it("keeps the last scan visible while a refresh is failing", () => {
    const presentation = presentScan(
      context(makeScan("running"), { error: "network" as UiText }),
    );
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
        processing: { probe: 2, thumbnail: 1, seekThumbnail: 0, preview: 0 },
      }),
    );
    expect(presentation.state).toBe("preparing");
    expect(presentation.remaining).toBe(3);
    expect(presentation.progress).toBeNull();
    expect(presentation.description).toBe("Preparing the scanned videos (3 items left)");
  });

  it("シーク用サムネイルだけが残っていても準備中とし、残りに数える", () => {
    const presentation = presentScan(
      context(makeScan("done"), {
        processing: { probe: 0, thumbnail: 0, seekThumbnail: 2, preview: 0 },
      }),
    );
    expect(presentation.state).toBe("preparing");
    expect(presentation.remaining).toBe(2);
    expect(presentation.description).toBe("Preparing the scanned videos (2 items left)");
  });

  it("準備の残りが無くなれば完了になる", () => {
    const presentation = presentScan(
      context(makeScan("done"), {
        processing: { probe: 0, thumbnail: 0, seekThumbnail: 0, preview: 0 },
      }),
    );
    expect(presentation.state).toBe("done");
  });

  it("準備の残りをまだ得ていなければ、完了とせずに確認中として示す", () => {
    const presentation = presentScan(context(makeScan("done")));
    expect(presentation.state).toBe("preparing");
    expect(presentation.description).toBe(
      "Checking what's left to prepare for the scanned videos",
    );
  });

  it("取り込み自体の失敗は、準備の残りがあっても失敗として示す", () => {
    const presentation = presentScan(
      context(makeScan("failed", { error: "読めません" }), {
        processing: { probe: 1, thumbnail: 0, seekThumbnail: 0, preview: 0 },
      }),
    );
    expect(presentation.state).toBe("failed");
    // 自由文（アップグレード前の日本語）は出さず、一般的な英語の概要にする。
    expect(presentation.description).toBe("The scan failed.");
  });

  it("取り込みの失敗は errorCode と errorPath から説明を作る", () => {
    const presentation = presentScan(
      context(
        makeScan("failed", {
          error: "open /media: permission denied",
          errorCode: "media_folder_unreadable",
          errorPath: "/media",
        }),
      ),
    );
    expect(presentation.description).toBe(
      "The scan failed: The media folder couldn't be read: /media",
    );
  });
});
