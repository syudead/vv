import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import type { WorkInfo } from "../../api/workInfo";
import type { WorkInfoFetch, WorkInfoMatch } from "../../api/workInfoPlugins";
import {
  initialWorkSelection,
  wholeRows,
  workChoice,
  type WorkSelection,
} from "./workSelection";

function info(overrides: Partial<WorkInfo> = {}): WorkInfo {
  return {
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    ...overrides,
  };
}

function videoWith(workInfo: WorkInfo, title = "今の題名"): Video {
  return {
    id: 7,
    title,
    tags: [{ id: 1, name: "既存", manual: true, fromFolder: false, tentative: false }],
    workInfo,
  } as unknown as Video;
}

function result(overrides: Partial<WorkInfoFetch> = {}): WorkInfoFetch {
  return {
    fetchId: "f",
    expiresAt: 0,
    tags: [],
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    covers: [],
    ...overrides,
  };
}

function performer(name: string, match: WorkInfoMatch) {
  return { role: "performer" as const, name, externalIds: [], match };
}

const none: WorkSelection = {
  fields: [],
  tags: [],
  cover: null,
  people: [],
  organizations: [],
  urls: [],
  externalIds: [],
  samePeople: [],
};

describe("wholeRows", () => {
  it("空の値は fill、違う値は replace、同じ値は same にし、結果に無い項目は出さない", () => {
    const video = videoWith(
      info({ code: "A-1", dates: [{ kind: "release", date: "2024-01-01" }] }),
    );
    expect(
      wholeRows(
        video,
        result({
          title: "新しい題名",
          code: "B-2",
          description: "説明",
          dates: [{ kind: "release", date: "2024-01-01" }],
        }),
      ),
    ).toEqual([
      { field: "title", kind: "replace" },
      { field: "code", kind: "replace" },
      { field: "description", kind: "fill" },
      { field: "dates.release", kind: "same" },
    ]);
    expect(wholeRows(video, result({ title: "今の題名" }))).toEqual([
      { field: "title", kind: "same" },
    ]);
  });
});

describe("initialWorkSelection", () => {
  it("同じ値と動画にある値を除いて全部を選び、表紙があれば最初の表紙を選ぶ", () => {
    const video = videoWith(
      info({
        code: "A-1",
        people: [
          {
            role: "performer",
            person: { id: 9, name: "既存の人", provisional: false },
            provisional: false,
          },
        ],
        urls: ["https://a.example/1"],
        externalIds: [{ source: "x", value: "1" }],
      }),
    );
    expect(
      initialWorkSelection(
        video,
        result({
          title: "新しい題名",
          code: "A-1",
          tags: ["既存", "新規"],
          covers: [{ width: 800, height: 600 }],
          people: [
            performer("既存の人", { state: "existing", id: 9, name: "既存の人" }),
            performer("新しい人", { state: "new" }),
            performer("同名の人", { state: "ambiguous" }),
          ],
          urls: ["https://a.example/1", "https://b.example/2"],
          externalIds: [
            { source: "x", value: "1" },
            { source: "x", value: "2" },
          ],
        }),
      ),
    ).toEqual({
      fields: ["title"],
      tags: ["新規"],
      cover: 0,
      people: [1],
      organizations: [],
      urls: ["https://b.example/2"],
      externalIds: [{ source: "x", value: "2" }],
      samePeople: [],
    });
  });

  it("表紙の無い結果では表紙を選ばない", () => {
    expect(initialWorkSelection(videoWith(info()), result()).cover).toBeNull();
  });
});

describe("workChoice", () => {
  it("空の一覧を送らない", () => {
    expect(workChoice({ ...none, fields: ["code"] })).toEqual({ fields: ["code"] });
  });

  it("選んだタグと表紙を項目に加え、名前と位置を送る", () => {
    expect(
      workChoice({
        ...none,
        fields: ["title"],
        tags: ["新規"],
        cover: 0,
        people: [2],
        organizations: [0],
        urls: ["https://b.example/2"],
        externalIds: [{ source: "x", value: "2" }],
      }),
    ).toEqual({
      fields: ["title", "tags", "cover"],
      tags: ["新規"],
      coverIndex: 0,
      people: [2],
      organizations: [0],
      urls: ["https://b.example/2"],
      externalIds: [{ source: "x", value: "2" }],
    });
  });
});
