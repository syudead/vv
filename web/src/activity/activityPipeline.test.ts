import { describe, expect, it } from "vitest";

import type { ActivityListItem, ActivityMemberSummary } from "../api/activity";
import { pipelineProgress, pipelineSummary } from "./activityPipeline";

const now = new Date("2026-10-18T10:05:00Z");
const video = { kind: "video" as const, items: [{ label: "clip.mp4", videoId: 7 }] };

function member(
  id: number,
  extra: Partial<ActivityMemberSummary> = {},
): ActivityMemberSummary {
  return {
    id,
    kind: "import",
    feature: "import",
    subject: video,
    status: "done",
    decider: "rule",
    retryRun: false,
    undoable: false,
    undone: false,
    replaced: false,
    ...extra,
  };
}

function pipeline(
  extra: Partial<ActivityListItem>,
  members: ActivityMemberSummary[],
): ActivityListItem {
  return {
    id: 1,
    kind: "pipeline",
    feature: "import",
    status: "done",
    decider: "rule",
    subject: video,
    candidate: false,
    retryRun: false,
    undoable: false,
    undone: false,
    createdAt: now.toISOString(),
    updatedAt: now.toISOString(),
    members,
    ...extra,
  };
}

function summary(item: ActivityListItem): string {
  return pipelineSummary(item, now)
    .parts.map((part) => part.text)
    .join(" ");
}

describe("pipelineSummary", () => {
  it("names the task in progress, not the import, once the import is done", () => {
    const item = pipeline({ status: "running", reason: "processing" }, [
      member(2),
      member(3, { kind: "auto_tag", status: "running", reason: "processing" }),
    ]);
    expect(summary(item)).toBe("Auto-tagging in progress · 1 of 2 tasks done");
    expect(pipelineProgress(item)).toBeUndefined();
  });

  it("counts only the tasks the card shows, leaving replaced ones out", () => {
    const item = pipeline({}, [
      member(2),
      member(3, {
        kind: "auto_tag",
        status: "attention",
        reason: "failed",
        replaced: true,
      }),
      member(4, { kind: "auto_tag" }),
    ]);
    expect(summary(item)).toBe("Finished · 2 tasks");
  });

  it("keeps the outcome phrase of a run whose video left the library", () => {
    expect(summary(pipeline({ outcome: "gone" }, [member(2)]))).toBe(
      "A video left the library",
    );
  });
});
