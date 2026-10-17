import { describe, expect, it } from "vitest";

import type { LibraryGroupCard, Video } from "../api/client";
import { favoriteMark, noteFavoriteOf } from "../api/favorites";
import type { HomeShelf, HomeShelfItem } from "../api/home";
import {
  drawnShelves,
  loadedShelfKeys,
  nextShelfBatch,
  organizingLineIndex,
  seeAllPath,
  shelfBatchSize,
  shelfHeadingPath,
  shelfItemPath,
  type ShelfState,
  shelfTitle,
  withFavorites,
  withFavoritesSince,
  withShelfItems,
} from "./homeShelves";

const folder: HomeShelf = {
  key: "folder:3",
  kind: "folder",
  name: "Travel",
  folderId: 3,
};
const recent: HomeShelf = { key: "recent", kind: "recent", name: "" };
const unshelved: HomeShelf = { key: "unshelved", kind: "unshelved", name: "" };
const continueShelf: HomeShelf = { key: "continue", kind: "continue", name: "" };
const recommended: HomeShelf = { key: "recommended", kind: "recommended", name: "" };

function videoItem(id: number): Extract<HomeShelfItem, { kind: "video" }> {
  return { kind: "video", video: { id, title: `Video ${String(id)}` } as Video };
}

function groupItem(
  id: number,
  openVideoId: number,
): Extract<HomeShelfItem, { kind: "group" }> {
  return {
    kind: "group",
    group: { id, name: `Group ${String(id)}`, openVideoId } as LibraryGroupCard,
  };
}

function ready(items: HomeShelfItem[]): ShelfState {
  return { status: "ready", items, total: items.length };
}

function folders(count: number): HomeShelf[] {
  return Array.from({ length: count }, (_, index) => ({
    key: `folder:${String(index + 1)}`,
    kind: "folder" as const,
    name: `Folder ${String(index + 1)}`,
    folderId: index + 1,
  }));
}

describe("棚の見出しと行き先（docs/design-docs/home.md「Home」）", () => {
  it("フォルダの棚はフォルダの名前で、見出しと See all はフォルダのページを開く", () => {
    expect(shelfTitle(folder)).toBe("Travel");
    expect(shelfHeadingPath(folder)).toBe("/smart-folders/3");
    expect(seeAllPath(folder)).toBe("/smart-folders/3");
  });

  it("決まった棚は画面の名前で、見出しはリンクにしない", () => {
    expect(shelfTitle(recent)).toBe("Recently added");
    expect(shelfTitle(continueShelf)).toBe("Continue watching");
    expect(shelfHeadingPath(recent)).toBeUndefined();
  });

  it("See all は続きから見るで履歴、最近追加でライブラリ、棚に無いもので unshelved、おすすめには無い", () => {
    expect(seeAllPath(continueShelf)).toBe("/history");
    expect(seeAllPath(recent)).toBe("/library");
    expect(seeAllPath(unshelved)).toBe("/library?unshelved=1");
    expect(seeAllPath(recommended)).toBeUndefined();
  });
});

describe("棚のカードが開く場所（research.md R-14）", () => {
  it("フォルダの棚の動画は ?smartFolder 付きの再生画面を開く", () => {
    expect(shelfItemPath(folder, videoItem(5))).toBe("/videos/5?smartFolder=3");
  });

  it("決まった棚の動画はその再生画面を開く", () => {
    expect(shelfItemPath(recent, videoItem(5))).toBe("/videos/5");
  });

  it("グループは開いている動画を ?group 付きで開く", () => {
    expect(shelfItemPath(folder, groupItem(7, 21))).toBe("/videos/21?group=7");
  });
});

describe("読む棚（research.md R-12）", () => {
  it("1 回に読む棚は画面に入る分とその次の 2 つで、8 を超えない", () => {
    expect(shelfBatchSize(240)).toBe(3);
    expect(shelfBatchSize(900)).toBe(6);
    expect(shelfBatchSize(4000)).toBe(8);
  });

  it("まだ読みに行っていない先頭の棚から数えて選ぶ", () => {
    const shelves = folders(10);
    const states = new Map<string, ShelfState>([
      ["folder:1", ready([videoItem(1)])],
      ["folder:2", ready([])],
    ]);
    expect(nextShelfBatch(shelves, states, 3)).toEqual([
      "folder:3",
      "folder:4",
      "folder:5",
    ]);
  });

  it("先頭の棚を読んでいる間は何も選ばない", () => {
    const states = new Map<string, ShelfState>([["folder:1", { status: "loading" }]]);
    expect(nextShelfBatch(folders(4), states, 3)).toEqual([]);
  });

  it("読み直す棚は読みに行った棚（読み込み中を含む）を 8 つずつに分ける", () => {
    const shelves = folders(11);
    const states = new Map<string, ShelfState>(
      shelves.slice(0, 10).map((shelf) => [shelf.key, ready([videoItem(1)])]),
    );
    states.set("folder:10", { status: "loading" });
    expect(loadedShelfKeys(shelves, states).map((keys) => keys.length)).toEqual([8, 2]);
  });
});

describe("描く棚（ui-design.md「Home」）", () => {
  it("項目の無い棚は描かず、状態の決まっていない最初の棚を読み込み中で描いてその下は描かない", () => {
    const shelves = folders(5);
    const states = new Map<string, ShelfState>([
      ["folder:1", ready([videoItem(1)])],
      ["folder:2", ready([])],
      ["folder:3", { status: "failed", reason: "offline" } as ShelfState],
    ]);
    const drawn = drawnShelves(shelves, states);
    expect(
      drawn.map((shelf) => [shelf.shelf.key, shelf.state.status, shelf.frontier]),
    ).toEqual([
      ["folder:1", "ready", false],
      ["folder:3", "failed", false],
      ["folder:4", "loading", true],
    ]);
  });

  it("失敗した棚を読み直す間も、その棚と下の棚を描き続ける", () => {
    const shelves = folders(3);
    const states = new Map<string, ShelfState>([
      ["folder:1", { status: "loading", retry: true }],
      ["folder:2", ready([videoItem(2)])],
      ["folder:3", { status: "failed", reason: "offline" } as ShelfState],
    ]);
    expect(
      drawnShelves(shelves, states).map((shelf) => [
        shelf.shelf.key,
        shelf.state.status,
        shelf.frontier,
      ]),
    ).toEqual([
      ["folder:1", "loading", false],
      ["folder:2", "ready", false],
      ["folder:3", "failed", false],
    ]);
    expect(nextShelfBatch(folders(4), states, 3)).toEqual(["folder:4"]);
  });

  it("応答に無い鍵の棚は項目 0 件として描かない", () => {
    const states = withShelfItems(
      new Map(),
      ["folder:1", "recent"],
      [{ key: "recent", items: [videoItem(9)], total: 1 }],
    );
    expect(
      drawnShelves([folders(1)[0] as HomeShelf, recent], states).map((d) => d.shelf.key),
    ).toEqual(["recent"]);
  });
});

describe("整理役の行の位置（ui-design.md「Home」）", () => {
  const ready: ShelfState = { status: "ready", items: [videoItem(1)], total: 1 };
  const second: HomeShelf = { ...folder, key: "folder:4", name: "Food", folderId: 4 };

  it("最後のフォルダの棚の直後に置く", () => {
    const drawn = [folder, second, recent].map((shelf) => ({
      shelf,
      state: ready,
      frontier: false,
    }));
    expect(organizingLineIndex(drawn)).toBe(2);
  });

  it("フォルダの棚を描いていなければ先頭に置く", () => {
    expect(organizingLineIndex([{ shelf: recent, state: ready, frontier: false }])).toBe(
      0,
    );
    expect(organizingLineIndex([])).toBe(0);
  });
});

describe("お気に入りの付け外し", () => {
  it("反映された動画と要求に含めたグループの印だけを変える", () => {
    const states = new Map<string, ShelfState>([
      ["recent", ready([videoItem(1), videoItem(2), groupItem(7, 21)])],
    ]);
    const next = withFavorites(states, { videoIds: [2], groupIds: [7], favorite: true });
    const items = (next.get("recent") as Extract<ShelfState, { status: "ready" }>).items;
    expect(
      items.map((item) =>
        item.kind === "video"
          ? item.video.favorite
          : item.kind === "group"
            ? item.group.favorite
            : undefined,
      ),
    ).toEqual([undefined, true, true]);
  });
});

describe("読んでいる間のお気に入りの付け外し", () => {
  it("読みに行った後に反映した付け外しで、届いた項目の印を直す", () => {
    const mark = favoriteMark();
    noteFavoriteOf([4101], true, Number.MAX_SAFE_INTEGER);
    const [shelf] = withFavoritesSince(
      [{ key: "recent", items: [videoItem(4101), videoItem(4102)], total: 2 }],
      mark,
    );
    expect(
      shelf?.items.map((item) => (item.kind === "video" ? item.video.favorite : null)),
    ).toEqual([true, undefined]);
  });
});
