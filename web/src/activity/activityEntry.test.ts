import { describe, expect, it } from "vitest";

import type { ActivityEntry } from "../api/activity";
import {
  cardTone,
  entryResult,
  entryTime,
  openActions,
  statusMark,
} from "./activityEntry";

function entry(extra: Partial<ActivityEntry> = {}): ActivityEntry {
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
    createdAt: "2026-10-16T05:00:00Z",
    updatedAt: "2026-10-16T05:00:00Z",
    ...extra,
  };
}

function resultText(value: ActivityEntry, now = new Date("2026-10-16T06:00:00Z")) {
  return entryResult(value, now)
    .parts.map((part) => part.text)
    .join(" ");
}

describe("cardTone and statusMark", () => {
  it("colours a card by its status and reason", () => {
    expect(cardTone(entry({ status: "running", reason: "processing" }))).toBe("primary");
    expect(cardTone(entry({ status: "running", reason: "waiting" }))).toBe("default");
    expect(cardTone(entry({ status: "attention", reason: "failed" }))).toBe(
      "destructive",
    );
    expect(cardTone(entry({ status: "attention", reason: "interrupted" }))).toBe(
      "destructive",
    );
    expect(cardTone(entry({ status: "attention", reason: "no_connection" }))).toBe(
      "warning",
    );
    expect(cardTone(entry({ status: "attention", reason: "undecided" }))).toBe("warning");
    expect(cardTone(entry())).toBe("default");
  });

  it("marks an entry decided by the owner as done by you", () => {
    expect(statusMark(entry({ decider: "owner", kind: "tag_attach" }))).toBe("doneByYou");
    expect(statusMark(entry())).toBe("done");
    expect(statusMark(entry({ status: "running", reason: "waiting" }))).toBe("waiting");
  });
});

describe("entryResult", () => {
  it("names the stage a failed import gave up at", () => {
    const failed = entry({
      status: "attention",
      reason: "failed",
      stage: "thumbnail",
      detail: "ffmpeg exited",
    });
    expect(resultText(failed)).toBe("Failed at thumbnail: ffmpeg exited");
    expect(entryResult(failed, new Date()).tone).toBe("destructive");
  });

  it("shows the running import's stage", () => {
    expect(
      resultText(
        entry({
          status: "running",
          reason: "processing",
          progress: { stage: "preview", done: 3, total: 5 },
        }),
      ),
    ).toBe("Preview");
  });

  it("gives the AI's percentage and the rule apart", () => {
    const same = entry({
      kind: "version_judgement",
      feature: "duplicate_check",
      outcome: "same",
      decider: "ai",
      probability: 0.91,
      subject: { kind: "pair", items: [{ label: "a" }, { label: "b" }] },
    });
    expect(resultText(same)).toBe("Same video · bundled by AI 91%");
    expect(entryResult(same, new Date()).parts.at(-1)).toEqual({
      text: "91%",
      strong: true,
    });
    expect(resultText({ ...same, decider: "rule", probability: undefined })).toBe(
      "Same video · bundled by rule",
    );
  });

  it("lists the auto-tagging tags and marks a suggested one", () => {
    expect(
      resultText(
        entry({
          kind: "auto_tag",
          feature: "auto_tagging",
          decider: "ai",
          tags: [
            { id: 1, name: "cat", suggested: false, probability: 0.91, undone: false },
            { id: 2, name: "dog", suggested: true, probability: 0.6, undone: false },
          ],
        }),
      ),
    ).toBe("+ cat 91% · + dog 60% (suggested)");
  });

  it("adds the undone and not-in-the-library suffixes", () => {
    expect(
      resultText(
        entry({
          undone: true,
          subject: { kind: "video", items: [{ label: "a.mp4" }] },
        }),
      ),
    ).toBe("Done · Undone by you · Not in the library");
  });

  it("describes the owner's tag operation", () => {
    expect(
      resultText(
        entry({ kind: "tag_detach", feature: "tags", decider: "owner", detail: "cat" }),
      ),
    ).toBe("− cat · by you");
  });

  it("reads an assistant run's operations and counts from its detail", () => {
    const run: Partial<ActivityEntry> = {
      kind: "assistant_run",
      feature: "assistant",
      decider: "owner",
      subject: { kind: "proposal", items: [{ label: "これにタグを付けて" }] },
    };
    expect(
      resultText(
        entry({
          ...run,
          outcome: "partial",
          detail:
            '{"total":1201,"applied":1001,"failed":200,"steps":[' +
            '{"operation":"createTag","status":"done","total":1,"applied":1,"failed":0},' +
            '{"operation":"updateVideoTags.add","status":"partial","total":1200,"applied":1000,"failed":200}]}',
        }),
      ),
    ).toBe("Create tag · Add tags · 1,001 changed · 200 failed");
    expect(
      resultText(
        entry({
          ...run,
          outcome: "stale",
          detail: '{"operation":"mergeTag","total":1,"applied":0,"failed":0}',
        }),
      ),
    ).toBe(
      "Merge tags · Something changed since this was proposed, so nothing was done. Ask again.",
    );
  });
});

describe("openActions", () => {
  it("opens the video page of a video subject in the library", () => {
    expect(openActions(entry()).map((action) => action.to)).toEqual(["/videos/7"]);
  });

  it("offers nothing to open when the subject has no video", () => {
    expect(
      openActions(entry({ subject: { kind: "video", items: [{ label: "gone.mp4" }] } })),
    ).toEqual([]);
    expect(
      openActions(
        entry({
          kind: "scan",
          feature: "scan",
          status: "attention",
          reason: "failed",
          subject: { kind: "scan", items: [{ label: "manual" }] },
        }),
      ),
    ).toEqual([]);
  });

  it("sends a waiting pair to Settings and Duplicates", () => {
    expect(
      openActions(
        entry({
          kind: "version_judgement",
          status: "attention",
          reason: "no_connection",
          candidate: true,
          subject: {
            kind: "pair",
            items: [
              { label: "a", videoId: 1 },
              { label: "b", videoId: 2 },
            ],
          },
        }),
      ).map((action) => action.id),
    ).toEqual(["settings", "duplicates"]);
  });

  it("opens the work info editor for a fetch", () => {
    expect(
      openActions(entry({ kind: "work_info_fetch", feature: "work_info" })).map(
        (action) => action.to,
      ),
    ).toEqual(["/videos/7/edit", "/videos/7"]);
  });
});

describe("entryTime", () => {
  const now = new Date(2026, 9, 16, 14, 30);

  it("counts minutes and hours today", () => {
    expect(entryTime(new Date(2026, 9, 16, 14, 18).toISOString(), now)).toMatchObject({
      long: "12 minutes ago",
      short: "12m",
    });
    expect(entryTime(new Date(2026, 9, 16, 11, 0).toISOString(), now)).toMatchObject({
      long: "3 hours ago",
      short: "3h",
    });
    expect(entryTime(now.toISOString(), now).long).toBe("Just now");
  });

  it("names yesterday and dates before it", () => {
    expect(entryTime(new Date(2026, 9, 15, 23, 0).toISOString(), now).short).toBe(
      "Yesterday",
    );
    expect(entryTime(new Date(2026, 9, 7, 9, 5).toISOString(), now).short).toBe("Oct 7");
  });
});
