import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import {
  draftChanged,
  draftFromVideo,
  newRowKey,
  toDetailsInput,
  withNamed,
} from "./draft";

const video = {
  id: 7,
  title: "Title",
  displayName: "Title",
  workInfo: {
    code: "AB-1",
    dates: [{ kind: "recorded", date: "2024" }],
    people: [],
    organizations: [{ role: "label", organization: { id: 3, name: "Studio" } }],
    urls: ["https://example.com/a"],
    externalIds: [{ source: "imdb", value: "tt1" }],
  },
} as unknown as Video;

describe("draft", () => {
  it("前後の空白と、足しただけの空の行は変更に数えない", () => {
    const loaded = draftFromVideo(video);
    const draft = {
      ...loaded,
      name: " Title ",
      urls: [...loaded.urls, { key: newRowKey("u"), url: " " }],
      externalIds: [
        ...loaded.externalIds,
        { key: newRowKey("i"), source: "", value: "" },
      ],
    };
    expect(draftChanged(draft, loaded)).toBe(false);
    expect(draftChanged({ ...draft, code: "AB-2" }, loaded)).toBe(true);
  });

  it("名前を付けた人物のチップは顔を保ったまま普通の人物になる", () => {
    const loaded = draftFromVideo(video);
    const naming = { key: "c1", id: 6, name: "Mio", naming: true, imageRevision: 5 };
    const draft = { ...loaded, people: { ...loaded.people, performer: [naming] } };
    expect(withNamed(draft, 6, "Mio").people.performer).toEqual([
      { key: "c1", id: 6, name: "Mio", imageRevision: 5 },
    ]);
  });

  it("送った一覧の位置で、誤りの経路から入力を引ける", () => {
    const loaded = draftFromVideo(video);
    const blank = { key: newRowKey("u"), url: "" };
    const draft = { ...loaded, urls: [blank, ...loaded.urls] };
    const { body, targets } = toDetailsInput(draft);
    expect(body).toEqual({
      displayName: "Title",
      workInfo: {
        code: "AB-1",
        description: null,
        dates: [{ kind: "recorded", date: "2024" }],
        people: [],
        organizations: [{ role: "label", organization: { id: 3 } }],
        urls: ["https://example.com/a"],
        externalIds: [{ source: "imdb", value: "tt1" }],
      },
    });
    // Empty rows are omitted, so urls[0] refers to the second input row.
    expect(targets.get("urls[0]")).toBe(`url-${loaded.urls[0]!.key}`);
    expect(targets.get("dates[0].date")).toBe("date-recorded");
    expect(targets.get("organizations[0].organization.id")).toBe(
      `chip-${loaded.organizations.label[0]!.key}`,
    );
    expect(targets.get("externalIds[0].value")).toBe(
      `id-value-${loaded.externalIds[0]!.key}`,
    );
  });
});
