import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import type { WorkInfo } from "../../api/workInfo";
import { availableTabs, creditedPeopleCount, nextTab } from "./videoTabs";

const video: Video = {
  id: 1,
  title: "a",
  sizeBytes: 1,
  addedAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  fileCreatedAt: "2026-01-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  tags: [],
  public: true,
};

const empty: WorkInfo = {
  dates: [],
  people: [],
  organizations: [],
  urls: [],
  externalIds: [],
};

function person(id: number, role: WorkInfo["people"][number]["role"]) {
  return {
    role,
    provisional: false,
    person: { id, name: `p${String(id)}`, provisional: false },
  };
}

const performers: WorkInfo = {
  ...empty,
  description: "about",
  people: [person(1, "performer"), person(2, "performer")],
};

describe("availableTabs", () => {
  it("lists only Overview, Thumbnail and File & media for an owner's bare video", () => {
    expect(availableTabs(video, true, false)).toEqual(["overview", "thumbnail", "file"]);
  });

  it("lists only Overview and File & media for a guest's bare video", () => {
    expect(availableTabs(video, false, true)).toEqual(["overview", "file"]);
  });

  it("lists Work for an owner with an item other than the description and the performers", () => {
    const tabs = (workInfo: WorkInfo) =>
      availableTabs({ ...video, workInfo }, true, false).includes("work");
    expect(tabs(performers)).toBe(false);
    expect(tabs({ ...empty, code: "A-1" })).toBe(true);
    expect(tabs({ ...empty, people: [person(3, "director")] })).toBe(true);
    expect(tabs({ ...empty, urls: ["https://example.com"] })).toBe(true);
  });

  it("lists Work for an owner's empty work only once an approved plugin is known", () => {
    expect(availableTabs(video, true, false)).not.toContain("work");
    expect(availableTabs(video, true, true)).toEqual([
      "overview",
      "work",
      "thumbnail",
      "file",
    ]);
  });

  it("lists People for an owner with credited people and never for a guest", () => {
    const credited = { ...video, workInfo: performers };
    expect(availableTabs(credited, true, false)).toContain("people");
    expect(availableTabs(credited, false, false)).not.toContain("people");
    expect(availableTabs(credited, false, false)).not.toContain("work");
  });

  it("lists Versions from two versions for owner and guest alike", () => {
    const versions = (count: number) => ({ count, representativeId: 1 });
    expect(availableTabs({ ...video, versions: versions(1) }, true, false)).not.toContain(
      "versions",
    );
    expect(availableTabs({ ...video, versions: versions(2) }, false, false)).toEqual([
      "overview",
      "versions",
      "file",
    ]);
  });

  it("orders all six tabs as the strip shows them", () => {
    const full = {
      ...video,
      workInfo: { ...performers, code: "A-1" },
      versions: { count: 3, representativeId: 1 },
    };
    expect(availableTabs(full, true, false)).toEqual([
      "overview",
      "work",
      "people",
      "versions",
      "thumbnail",
      "file",
    ]);
  });
});

describe("creditedPeopleCount", () => {
  it("counts a person of two roles once", () => {
    expect(
      creditedPeopleCount({
        ...empty,
        people: [person(1, "performer"), person(1, "director"), person(2, "performer")],
      }),
    ).toBe(2);
  });
});

describe("nextTab", () => {
  it("keeps the selected tab while the new list has it and returns to Overview otherwise", () => {
    expect(nextTab("versions", ["overview", "versions", "file"])).toBe("versions");
    expect(nextTab("work", ["overview", "thumbnail", "file"])).toBe("overview");
  });
});
