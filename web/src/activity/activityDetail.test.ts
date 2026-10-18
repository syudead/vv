import { describe, expect, it } from "vitest";

import type { ActivityDetail, ActivityEntry } from "../api/activity";
import {
  detailNotice,
  headerVideo,
  listTarget,
  memberResult,
  memberTitle,
  stageLines,
} from "./activityDetail";

const at = "2026-10-18T10:00:00Z";

function detail(extra: Partial<ActivityDetail> = {}): ActivityDetail {
  return {
    id: 1,
    kind: "import",
    feature: "import",
    status: "done",
    outcome: "completed",
    decider: "rule",
    subject: { kind: "video", items: [{ label: "a.mp4", videoId: 7 }] },
    candidate: false,
    retryRun: false,
    undoable: false,
    undone: false,
    createdAt: at,
    updatedAt: at,
    ...extra,
  };
}

describe("stageLines", () => {
  it("reads every stage in ingest order, telling a stage left out from one that never ran", () => {
    const lines = stageLines(
      detail({
        status: "attention",
        reason: "failed",
        stagesRecorded: true,
        stages: [
          { stage: "probe", state: "done", updatedAt: at },
          {
            stage: "thumbnail",
            state: "failed",
            detail: "line 1\nline 2",
            updatedAt: at,
          },
          { stage: "preview", state: "waiting", updatedAt: at },
        ],
      }),
    );
    expect(lines).toEqual([
      { stage: "probe", view: "done", updatedAt: at },
      { stage: "thumbnail", view: "failed", updatedAt: at, message: "line 1\nline 2" },
      { stage: "seek_thumbnail", view: "absent" },
      { stage: "preview", view: "notRun", updatedAt: at },
      { stage: "fingerprint", view: "absent" },
    ]);
  });

  it("keeps a waiting stage waiting while its import runs", () => {
    const lines = stageLines(
      detail({
        status: "running",
        reason: "processing",
        stages: [{ stage: "preview", state: "waiting", updatedAt: at }],
      }),
    );
    expect(
      Array.isArray(lines) && lines.find((line) => line.stage === "preview")?.view,
    ).toBe("waiting");
  });

  it("says the stages were not recorded for an import from before them, and has none for other kinds", () => {
    expect(stageLines(detail({ stagesRecorded: false }))).toBe("notRecorded");
    expect(stageLines(detail({ kind: "auto_tag", feature: "auto_tagging" }))).toBeNull();
  });
});

describe("headerVideo", () => {
  it("opens a video still in the library and nothing for one that left it or a pair", () => {
    expect(headerVideo(detail())).toBe(7);
    expect(
      headerVideo(detail({ subject: { kind: "video", items: [{ label: "gone.mp4" }] } })),
    ).toBeUndefined();
    expect(
      headerVideo(
        detail({
          subject: {
            kind: "pair",
            items: [
              { label: "a.mp4", videoId: 7 },
              { label: "b.mp4", videoId: 8 },
            ],
          },
        }),
      ),
    ).toBeUndefined();
  });
});

describe("listTarget", () => {
  it("returns to the list with its filters, and to the plain list from anywhere else", () => {
    expect(listTarget({ from: "/activity?status=attention" })).toBe(
      "/activity?status=attention",
    );
    expect(listTarget({ from: "/videos/7" })).toBe("/activity");
    expect(listTarget({ from: "//example.com/activity" })).toBe("/activity");
    expect(listTarget(null)).toBe("/activity");
  });
});

describe("memberTitle", () => {
  it("names the other video of a pair member", () => {
    const pipeline = detail({ kind: "pipeline" });
    const pair: ActivityEntry = detail({
      kind: "version_judgement",
      feature: "duplicate_check",
      subject: {
        kind: "pair",
        items: [
          { label: "b.mp4", videoId: 8 },
          { label: "a.mp4", videoId: 7 },
        ],
      },
    });
    expect(memberTitle(pair, pipeline)).toBe("Duplicate check · with b.mp4");
    expect(memberTitle(detail({ kind: "auto_tag" }), pipeline)).toBe("Auto-tagging");
  });
});

describe("memberResult", () => {
  it("leaves a member's message to its own detail and marks a replaced member", () => {
    const failed = detail({
      status: "attention",
      reason: "failed",
      stage: "thumbnail",
      detail: "moov atom not found",
    });
    expect(memberResult(failed, new Date(at)).parts.map((part) => part.text)).toEqual([
      "Failed at thumbnail",
    ]);
    expect(
      memberResult({ ...failed, replaced: true }, new Date(at)).parts.map(
        (part) => part.text,
      ),
    ).toEqual(["Replaced by a newer run"]);
  });
});

describe("detailNotice", () => {
  it("names a pipeline's failing member and shows its full message without a retry", () => {
    const notice = detailNotice(
      detail({
        kind: "pipeline",
        status: "attention",
        reason: "failed",
        members: [
          detail({ id: 2, status: "done" }),
          detail({
            id: 3,
            kind: "auto_tag",
            feature: "auto_tagging",
            status: "attention",
            reason: "failed",
            detail: "connection refused",
            retryRun: true,
            replaced: true,
          }),
          detail({
            id: 4,
            kind: "auto_tag",
            feature: "auto_tagging",
            status: "attention",
            reason: "failed",
            detail: "model missing",
            retryRun: true,
          }),
        ],
      }),
      new Date(at),
    );
    expect(notice?.title.parts.map((part) => part.text)).toEqual([
      "Auto-tagging: Failed",
    ]);
    expect(notice?.message).toBe("model missing");
    expect(notice?.retry).toBe(false);
  });

  it("titles an entry's own failure without its message and offers retry when allowed", () => {
    const notice = detailNotice(
      detail({
        status: "attention",
        reason: "failed",
        stage: "thumbnail",
        detail: "x".repeat(2000),
        retryRun: true,
      }),
      new Date(at),
    );
    expect(notice?.title.parts.map((part) => part.text)).toEqual(["Failed at thumbnail"]);
    expect(notice?.message).toHaveLength(2000);
    expect(notice?.retry).toBe(true);
    expect(notice?.tone).toBe("destructive");
    expect(detailNotice(detail(), new Date(at))).toBeNull();
  });
});
