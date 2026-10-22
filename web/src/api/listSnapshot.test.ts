import { beforeEach, describe, expect, it } from "vitest";

import type { LibraryItem, Video } from "./client";
import { itemVideos, videoItem } from "./libraryItems";
import {
  applyProgressToListSnapshot,
  clearListSnapshot,
  holdListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "./listSnapshot";
import { nextVideoTagsSequence, recordAppliedVideoTags } from "./videoTagsEvents";

function item(id: number): Video {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    updatedAt: "2026-09-13T00:00:00Z",
    fileCreatedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    tags: [],
  };
}

function body(ids: number[], scrollY = 0) {
  return {
    items: ids.map((id) => videoItem(item(id))),
    total: ids.length,
    cursor: "cursor-1",
    hasMore: true,
    scrollY,
  };
}

beforeEach(() => {
  clearListSnapshot();
});

describe("ListSnapshot（一覧の復元状態）", () => {
  it("保存した鍵と同じ鍵で取れる", () => {
    saveListSnapshot({ query: "ねこ", sort: "titleAsc" }, body([1, 2], 640));

    const restored = takeListSnapshot({ query: "ねこ", sort: "titleAsc" });
    expect(itemVideos(restored?.items ?? []).map((video) => video.id)).toEqual([1, 2]);
    expect(restored?.total).toBe(2);
    expect(restored?.cursor).toBe("cursor-1");
    expect(restored?.hasMore).toBe(true);
    expect(restored?.scrollY).toBe(640);
  });

  it("鍵が違えば取れない", () => {
    saveListSnapshot({ query: "ねこ", sort: "titleAsc" }, body([1, 2]));

    expect(takeListSnapshot({ query: "いぬ", sort: "titleAsc" })).toBeUndefined();
    expect(takeListSnapshot({ query: "ねこ", sort: "addedDesc" })).toBeUndefined();
  });

  it("favorite を含む鍵は、絞らない鍵と一致しない（/?fav=1 と / の控えを区別する）", () => {
    saveListSnapshot({ query: "", sort: "addedDesc", favorite: true }, body([1]));

    expect(takeListSnapshot({ query: "", sort: "addedDesc" })).toBeUndefined();
    expect(
      takeListSnapshot({ query: "", sort: "addedDesc", favorite: false }),
    ).toBeUndefined();
    expect(
      takeListSnapshot({ query: "", sort: "addedDesc", favorite: true }),
    ).toBeDefined();

    saveListSnapshot({ query: "", sort: "addedDesc", favorite: false }, body([2]));
    expect(takeListSnapshot({ query: "", sort: "addedDesc" })).toBeDefined();
    expect(
      takeListSnapshot({ query: "", sort: "addedDesc", favorite: true }),
    ).toBeUndefined();
  });

  it("tags を含む鍵は、tags の無い鍵や別の tags の鍵と一致しない（/?tag=1 と / の控えを区別する）", () => {
    saveListSnapshot({ query: "", sort: "addedDesc", tags: [1] }, body([1]));

    expect(takeListSnapshot({ query: "", sort: "addedDesc" })).toBeUndefined();
    expect(takeListSnapshot({ query: "", sort: "addedDesc", tags: [2] })).toBeUndefined();
    expect(
      itemVideos(
        takeListSnapshot({ query: "", sort: "addedDesc", tags: [1] })?.items ?? [],
      ).map((video) => video.id),
    ).toEqual([1]);
  });

  it("tagKind を含む鍵は、tagKind の無い鍵や別の種類の鍵と一致しない（/?tagKind=1 と / の控えを区別する）", () => {
    saveListSnapshot({ query: "", sort: "addedDesc", tagKind: 1 }, body([1]));

    expect(takeListSnapshot({ query: "", sort: "addedDesc" })).toBeUndefined();
    expect(
      takeListSnapshot({ query: "", sort: "addedDesc", tagKind: 2 }),
    ).toBeUndefined();
    expect(takeListSnapshot({ query: "", sort: "addedDesc", tagKind: 1 })).toBeDefined();
  });

  it("tags の並びは正規化するので、選んだ順が違っても同じ鍵になる", () => {
    saveListSnapshot({ query: "", sort: "addedDesc", tags: [3, 1] }, body([1]));

    expect(
      takeListSnapshot({ query: "", sort: "addedDesc", tags: [1, 3] }),
    ).toBeDefined();
  });

  it("clearListSnapshot のあとは取れない", () => {
    saveListSnapshot({ query: "", sort: "addedDesc" }, body([1]));
    expect(takeListSnapshot({ query: "" })).toBeDefined();

    clearListSnapshot();
    expect(takeListSnapshot({ query: "" })).toBeUndefined();
  });

  it("2 件目を保存すると 1 件目は捨てられる", () => {
    saveListSnapshot({ query: "ねこ" }, body([1, 2]));
    saveListSnapshot({ query: "いぬ" }, body([3]));

    expect(
      itemVideos(takeListSnapshot({ query: "いぬ" })?.items ?? []).map(
        (video) => video.id,
      ),
    ).toEqual([3]);
    expect(takeListSnapshot({ query: "ねこ" })).toBeUndefined();
  });

  it("鍵の正規化が検索語の前後の空白と並び順の既定値を吸収する", () => {
    saveListSnapshot({ query: "  ねこ  ", sort: "addedDesc" }, body([1]));

    expect(takeListSnapshot({ query: "ねこ", sort: "addedDesc" })).toBeDefined();
    expect(takeListSnapshot({ query: "ねこ" })).toBeDefined();
    expect(takeListSnapshot({ query: "ねこ", sort: "titleAsc" })).toBeUndefined();
  });
});

describe("控えを止める印（holdListSnapshot）", () => {
  it("止めている間は今の控えを捨て、保存しても取れない。すべて解けば保存できる", () => {
    saveListSnapshot({ query: "" }, body([1]));
    const first = holdListSnapshot();
    const second = holdListSnapshot();
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    saveListSnapshot({ query: "" }, body([1]));
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    first();
    first();
    saveListSnapshot({ query: "" }, body([1]));
    expect(takeListSnapshot({ query: "" })).toBeUndefined();

    second();
    saveListSnapshot({ query: "" }, body([1]));
    expect(
      itemVideos(takeListSnapshot({ query: "" })?.items ?? []).map((video) => video.id),
    ).toEqual([1]);
  });
});

describe("フォルダ画面の控え", () => {
  it("フォルダを含む鍵は、同じ検索語と並び順のライブラリ一覧や別のフォルダと一致しない", () => {
    saveListSnapshot({ query: "", sort: "addedDesc", folder: "3\0A" }, body([1, 2], 40));

    expect(takeListSnapshot({ query: "", sort: "addedDesc" })).toBeUndefined();
    expect(
      takeListSnapshot({ query: "", sort: "addedDesc", folder: "3\0A/B" }),
    ).toBeUndefined();
    expect(
      takeListSnapshot({ query: "", sort: "titleAsc", folder: "3\0A" }),
    ).toBeUndefined();
    expect(takeListSnapshot({ query: "", folder: "3\0A" })?.scrollY).toBe(40);
  });

  it("ライブラリ一覧の控えはフォルダ画面で拾わない", () => {
    saveListSnapshot({ query: "" }, body([1]));
    expect(takeListSnapshot({ query: "", folder: "3\0" })).toBeUndefined();
  });
});

describe("付け外しの結果の反映", () => {
  it("recordAppliedVideoTagsのaddで、対象の項目のtagsだけが変わる", () => {
    saveListSnapshot({ query: "" }, body([1, 2]));

    recordAppliedVideoTags(
      [2],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "add",
      nextVideoTagsSequence(),
    );

    const restored = takeListSnapshot({ query: "" });
    expect(
      itemVideos(restored?.items ?? []).find((video) => video.id === 2)?.tags,
    ).toEqual([
      {
        id: 5,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
    ]);
    expect(
      itemVideos(restored?.items ?? []).find((video) => video.id === 1)?.tags,
    ).toEqual([]);
    expect(restored?.total).toBe(2);
    expect(restored?.cursor).toBe("cursor-1");
  });

  it("recordAppliedVideoTagsのremoveで、付いていたタグだけを外す", () => {
    saveListSnapshot(
      { query: "" },
      {
        items: [
          videoItem({
            ...item(1),
            tags: [
              {
                kind: null,
                id: 5,
                name: "旅行",
                manual: true,
                fromFolder: false,
                tentative: false,
              },
            ],
          }),
          videoItem({
            ...item(2),
            tags: [
              {
                kind: null,
                id: 5,
                name: "旅行",
                manual: true,
                fromFolder: false,
                tentative: false,
              },
            ],
          }),
        ],
        total: 2,
        cursor: "cursor-1",
        hasMore: true,
        scrollY: 0,
      },
    );

    recordAppliedVideoTags(
      [2],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "remove",
      nextVideoTagsSequence(),
    );

    const restored = takeListSnapshot({ query: "" });
    expect(
      itemVideos(restored?.items ?? []).find((video) => video.id === 2)?.tags,
    ).toEqual([]);
    expect(
      itemVideos(restored?.items ?? []).find((video) => video.id === 1)?.tags,
    ).toEqual([
      {
        id: 5,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
    ]);
  });
});

describe("控えの中のグループの項目", () => {
  const groupItem: LibraryItem = {
    kind: "group",
    group: {
      id: 7,
      folder: { rootId: 3, path: "A/B" },
      name: "B",
      videoCount: 2,
      sizeBytes: 2048,
      addedAt: "2026-09-13T00:00:00Z",
      previews: [],
      openVideoId: 10,
      videoIds: [10, 11],
      tags: [],
    },
  };

  function saveWithGroup() {
    saveListSnapshot(
      { query: "" },
      {
        items: [videoItem(item(1)), groupItem],
        total: 2,
        hasMore: false,
        scrollY: 0,
      },
    );
  }

  it("メンバーの再生位置の保存で、グループに取り直しの印を付け、項目は変えない", () => {
    saveWithGroup();
    applyProgressToListSnapshot(11, {
      positionMs: 1000,
      completed: false,
      updatedAt: "2026-09-23T00:00:00Z",
    });

    const restored = takeListSnapshot({ query: "" });
    expect(restored?.staleGroups).toEqual([7]);
    expect(restored?.items[1]).toBe(groupItem);
    expect(itemVideos(restored?.items ?? [])[0]?.progress).toBeUndefined();
  });

  it("メンバーのタグの付け外しで、グループに取り直しの印を1つだけ付ける", () => {
    saveWithGroup();
    recordAppliedVideoTags(
      [10, 11],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "add",
      nextVideoTagsSequence(),
    );
    recordAppliedVideoTags(
      [11],
      { kind: null, id: 6, name: "山", tentative: false },
      "add",
      nextVideoTagsSequence(),
    );

    const restored = takeListSnapshot({ query: "" });
    expect(restored?.staleGroups).toEqual([7]);
    expect(restored?.items[1]).toBe(groupItem);
  });

  it("メンバーでない動画の変化では印を付けない", () => {
    saveWithGroup();
    applyProgressToListSnapshot(1, {
      positionMs: 1000,
      completed: true,
      updatedAt: "2026-09-23T00:00:00Z",
    });

    const restored = takeListSnapshot({ query: "" });
    expect(restored?.staleGroups).toBeUndefined();
    expect(itemVideos(restored?.items ?? [])[0]?.progress?.completed).toBe(true);
  });
});
