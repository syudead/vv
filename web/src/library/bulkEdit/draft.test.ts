import { describe, expect, it } from "vitest";

import type { VideoEditSummary } from "../../api/videoEdits";
import {
  buildChanges,
  changeLines,
  cycleValue,
  emptyDraft,
  type FieldId,
  nextChoice,
  pickValue,
  presentCredits,
  presentTags,
  type PresentValue,
  setDate,
  setPublic,
} from "./draft";

const summary: VideoEditSummary = {
  total: 4,
  public: 1,
  tags: [
    {
      tag: { id: 1, name: "4K", tentative: false, kind: null },
      count: 4,
      manualCount: 4,
    },
    {
      tag: { id: 2, name: "Travel", tentative: false, kind: null },
      count: 2,
      manualCount: 1,
    },
    {
      tag: { id: 3, name: "Folder", tentative: false, kind: null },
      count: 3,
      manualCount: 0,
    },
  ],
  people: [
    { role: "performer", person: { id: 7, name: "Alice", provisional: false }, count: 1 },
    { role: "director", person: { id: 8, name: "", provisional: true }, count: 4 },
  ],
  organizations: [],
  dates: [
    { kind: "release", count: 4, date: "2024-05" },
    { kind: "recorded", count: 2 },
  ],
};

function presentOf(source: VideoEditSummary): Map<FieldId, PresentValue[]> {
  return new Map<FieldId, PresentValue[]>([
    ["tags", presentTags(source)],
    ["person:performer", presentCredits(source, "person", "performer")],
    ["person:director", presentCredits(source, "person", "director")],
  ]);
}

describe("nextChoice", () => {
  it("offers Add only while some videos lack the value and Remove only while some can lose it", () => {
    const steps = (value: PresentValue) => {
      const out: (string | undefined)[] = [];
      let choice = nextChoice(value, 4, undefined);
      out.push(choice);
      while (choice !== undefined) {
        choice = nextChoice(value, 4, choice);
        out.push(choice);
      }
      return out;
    };
    const [all, partial, folderOnly] = presentTags(summary);
    expect(steps(all!)).toEqual(["remove", undefined]);
    expect(steps(partial!)).toEqual(["add", "remove", undefined]);
    expect(steps(folderOnly!)).toEqual(["add", undefined]);
  });
});

describe("changeLines and buildChanges", () => {
  it("count the videos each choice changes and send one edit per role with both lists", () => {
    const present = presentOf(summary);
    const [, travel] = present.get("tags")!;
    const [alice] = present.get("person:performer")!;
    let draft = cycleValue(emptyDraft, "tags", travel!, 4); // add Travel
    draft = cycleValue(draft, "tags", travel!, 4); // then remove it
    draft = pickValue(
      draft,
      "tags",
      { key: "n:New", label: "New", newName: "New" },
      present.get("tags")!,
    );
    draft = cycleValue(draft, "person:performer", alice!, 4); // add Alice to all
    draft = pickValue(
      draft,
      "person:performer",
      { key: "new:k1", label: "Eve", newName: "Eve" },
      present.get("person:performer")!,
    );
    draft = setDate(draft, "release", { mode: "set", value: " 2024-05 " });
    draft = setDate(draft, "recorded", { mode: "clear" });
    draft = setPublic(draft, true);

    expect(changeLines(draft, summary, present)).toEqual([
      { field: "tags", label: "Travel", action: "remove", videos: 1 },
      { field: "tags", label: "New", action: "add", videos: 4 },
      { field: "person:performer", label: "Alice", action: "add", videos: 3 },
      { field: "person:performer", label: "Eve", action: "add", videos: 4 },
      { field: "recorded", label: "", action: "clear", videos: 2 },
      { field: "public", label: "", action: "public", videos: 3 },
    ]);

    const { changes, removedTags } = buildChanges(
      draft,
      summary,
      present,
      new Map([["new:k1", 50]]),
    );
    expect(changes).toEqual({
      tags: { add: [{ name: "New" }], remove: [2] },
      credits: [{ kind: "person", role: "performer", add: [7, 50] }],
      dates: [
        { kind: "release", date: "2024-05" },
        { kind: "recorded", date: null },
      ],
      public: true,
    });
    expect(removedTags.map((tag) => tag.id)).toEqual([2]);
  });

  it("marks a picked value the selection already has for every video instead of listing it twice", () => {
    const present = presentOf(summary);
    const draft = pickValue(
      emptyDraft,
      "person:performer",
      { key: "c7", label: "Alice", id: 7 },
      present.get("person:performer")!,
    );
    expect(draft.fields.get("person:performer")).toEqual({
      choices: new Map([["c7", "add"]]),
      picked: [],
    });
  });

  it("drops a remove whose value the narrowed selection no longer has", () => {
    const present = presentOf(summary);
    const [travel] = present.get("tags")!.slice(1);
    const draft = cycleValue(
      cycleValue(emptyDraft, "tags", travel!, 4),
      "tags",
      travel!,
      4,
    );
    const narrowed: VideoEditSummary = { ...summary, tags: [] };
    const narrowedPresent = presentOf(narrowed);
    expect(changeLines(draft, narrowed, narrowedPresent)).toEqual([]);
    expect(buildChanges(draft, narrowed, narrowedPresent, new Map()).changes).toEqual({});
  });
});
