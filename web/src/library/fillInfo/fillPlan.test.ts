import { describe, expect, it } from "vitest";

import { fetchResult, video } from "../../testing/workInfoFetchPage";
import { fillPlan } from "./fillPlan";

const fetched = fetchResult({
  title: "Fetched",
  tags: ["drama"],
  code: "ABC-1",
  description: "notes",
  covers: [{ width: 640, height: 360 }],
});

describe("fillPlan", () => {
  it("名前のない動画には名前・タグ・空の項目・画像を入れる", () => {
    const plan = fillPlan({ ...video, tags: [] }, fetched);
    expect(plan.lines.map((line) => line.field)).toEqual([
      "title",
      "code",
      "description",
      "tags",
      "cover",
    ]);
    expect(plan.selection).toMatchObject({
      fields: ["title", "code", "description"],
      tags: ["drama"],
      cover: 0,
    });
  });

  it("表示名・タグ・値・貼った画像がある動画には、その項目を入れない", () => {
    const filled = {
      ...video,
      displayName: "Owner's name",
      thumbnailImage: "uploaded" as const,
      workInfo: {
        code: "OWN-1",
        dates: [],
        people: [],
        organizations: [],
        urls: [],
        externalIds: [],
      },
    };
    const plan = fillPlan(filled, fetched);
    expect(plan.lines.map((line) => line.field)).toEqual(["description"]);
    expect(plan.selection).toMatchObject({
      fields: ["description"],
      tags: [],
      cover: null,
    });
  });

  it("仮想動画の題名は入れない", () => {
    const plan = fillPlan({ ...video, virtual: true, tags: [] }, fetched);
    expect(plan.selection.fields).not.toContain("title");
  });
});
