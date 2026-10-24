import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import {
  applyDropOverlay,
  bundleOverLimit,
  nameFieldOutcome,
  provisionalGroup,
  provisionalGroupName,
  videoOnto,
} from "./videoDrop";

function video(id: number, extra: Partial<Video> = {}): Video {
  return {
    id,
    virtual: false,
    title: `Video ${String(id)}`,
    public: false,
    sizeBytes: 100,
    addedAt: `2026-09-0${String(id)}T00:00:00Z`,
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    fileModifiedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "pending",
    thumbnailUrl: `/t/${String(id)}`,
    durationMs: 1000,
    tags: [],
    ...extra,
  };
}

describe("videoOnto", () => {
  it("動画だけなら「Group or bundle…」、グループが混じれば拒否", () => {
    expect(videoOnto(true)).toEqual({ label: "Group or bundle…" });
    expect(videoOnto(false)).toBe("refused");
  });
});

describe("bundleOverLimit", () => {
  it("持ち上げたカードと落とした先で 20 枚を超えると押せない", () => {
    expect(bundleOverLimit(19)).toBe(false);
    expect(bundleOverLimit(20)).toBe(true);
  });
});

describe("provisionalGroupName", () => {
  it("題名の先頭 100 コードポイントを前後の空白を削って使う", () => {
    expect(provisionalGroupName("  Trip  ")).toBe("Trip");
    const long = `${"😀".repeat(99)} x${"y".repeat(10)}`;
    expect(Array.from(provisionalGroupName(long))).toHaveLength(99);
  });

  it("サーバーと同じく先頭の空白を除いてから 100 コードポイントに切る（domain.GroupNameFromTitle）", () => {
    expect(provisionalGroupName(`${" ".repeat(100)}Trip`)).toBe("Trip");
    expect(provisionalGroupName(`\u3000\tTrip${" ".repeat(100)}x`)).toBe("Trip");
    expect(provisionalGroupName(`${"a".repeat(99)} b`)).toBe("a".repeat(99));
  });
});

describe("nameFieldOutcome", () => {
  it("空か仮の名前と同じなら何も送らず、変えた名前は削って送る", () => {
    expect(nameFieldOutcome("  ", "Trip")).toEqual({ kind: "keep" });
    expect(nameFieldOutcome(" Trip ", "Trip")).toEqual({ kind: "keep" });
    expect(nameFieldOutcome(" Kyoto ", "Trip")).toEqual({
      kind: "rename",
      name: "Kyoto",
    });
  });

  it("100 コードポイントを超えると閉じない", () => {
    expect(nameFieldOutcome("a".repeat(100), "Trip").kind).toBe("rename");
    expect(nameFieldOutcome("a".repeat(101), "Trip")).toEqual({ kind: "invalid" });
  });
});

describe("provisionalGroup", () => {
  it("落とした先、続いて持ち上げた動画を持ち、名前は落とした先の題名", () => {
    const made = provisionalGroup(-1, video(2, { title: "B" }), [
      video(1),
      video(3, { thumbnailUrl: undefined, durationMs: undefined }),
    ]);
    expect(made).toMatchObject({
      id: -1,
      name: "B",
      videoCount: 3,
      videoIds: [2, 1, 3],
      openVideoId: 2,
      durationMs: 2000,
      sizeBytes: 300,
      addedAt: "2026-09-03T00:00:00Z",
      previews: [
        { videoId: 2, thumbnailUrl: "/t/2" },
        { videoId: 1, thumbnailUrl: "/t/1" },
      ],
    });
  });
});

describe("applyDropOverlay", () => {
  const items = ["a", "b", "c", "d"];
  const key = (item: string) => item;

  it("新しいグループは落とした先の位置を取り、持ち上げたカードは消える", () => {
    const overlay = { target: "c", lifted: new Set(["a"]), made: "G" };
    expect(applyDropOverlay(items, key, overlay)).toEqual(["b", "G", "d"]);
  });

  it("束ねたときは持ち上げたカードだけが消え、無ければそのまま", () => {
    const overlay = { target: "c", lifted: new Set(["a", "d"]), made: null };
    expect(applyDropOverlay(items, key, overlay)).toEqual(["b", "c"]);
    expect(applyDropOverlay(items, key, null)).toBe(items);
  });
});
