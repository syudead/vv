import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { emitServerEvent } from "../api/fakeEventSource";
import {
  crumbs,
  folderCard,
  renderPage,
  revealCards,
  seed,
  server,
  setUpSmartFolderServer,
  video,
} from "../testing/smartFolderPage";

setUpSmartFolderServer();

/** cardNames は格子のカードの読み上げ名を上から返す。 */
function cardNames(): string[] {
  return screen
    .getAllByRole("article")
    .map(
      (card) => within(card).getAllByRole("link")[0]?.getAttribute("aria-label") ?? "",
    );
}

describe("スマートフォルダのページ（specs/052-smart-folders/ui-design.md「Smart folder page」）", () => {
  it("子フォルダ、続けて応答の順の動画を並べ、見出しに本数と Play を出す", async () => {
    seed();
    renderPage("/smart-folders/1");

    expect(
      await screen.findByRole("heading", { level: 1, name: "タグ: 猫" }),
    ).toBeTruthy();
    expect(screen.getByText("2 videos · 1:32:10")).toBeTruthy();
    expect(cardNames()).toEqual(["未視聴, smart folder", "Cat 1", "Cat 2"]);
    expect(
      screen.getByRole("link", { name: 'Play "タグ: 猫"' }).getAttribute("href"),
    ).toBe("/videos/10?smartFolder=1");
    expect(screen.getAllByRole("link", { name: "Cat 1" })[0]?.getAttribute("href")).toBe(
      "/videos/10?smartFolder=1",
    );
    expect(document.title).toBe("タグ: 猫");
  });

  it("子を開くと帯が伸びて親の名前が出て、親の条件の行が親のページへつながる", async () => {
    const user = userEvent.setup();
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });
    expect(crumbs()).toEqual(["Library", "タグ: 猫"]);
    expect(screen.getByText("Tag: 猫")).toBeTruthy();

    await user.click(screen.getByRole("link", { name: "未視聴, smart folder" }));

    expect(await screen.findByRole("heading", { level: 1, name: "未視聴" })).toBeTruthy();
    expect(crumbs()).toEqual(["Library", "タグ: 猫", "未視聴"]);
    expect(screen.getByText("Conditions:")).toBeTruthy();
    expect(screen.getByText("Unwatched")).toBeTruthy();
    const fromParent = screen.getByRole("link", { name: 'From "タグ: 猫":' });
    expect(fromParent.getAttribute("href")).toBe("/smart-folders/1");
    // 祖先の条件はその祖先を読んで引く。
    expect(await screen.findByText("Tag: 猫")).toBeTruthy();
  });

  it("一致するものの無いフォルダは空だと言い、Play を出さない", async () => {
    server.folders.set(2, {
      smartFolder: folderCard(2, "空", {
        videoCount: 0,
        durationMs: undefined,
        openVideoId: undefined,
      }),
      settings: { rule: { q: "none" }, sort: "addedDesc", missing: [] },
      trail: [],
      items: [],
    });
    renderPage("/smart-folders/2");

    expect(await screen.findByText("Nothing matches these conditions yet")).toBeTruthy();
    expect(screen.getByText("0 videos")).toBeTruthy();
    expect(screen.queryByRole("link", { name: /^Play/ })).toBeNull();
  });

  it("消えたタグを名指し、欠けた条件の状態を出す", async () => {
    server.folders.set(3, {
      smartFolder: folderCard(3, "消えた", { videoCount: 0, openVideoId: undefined }),
      settings: {
        rule: { tags: [99], watch: "watched" },
        sort: "addedDesc",
        missing: [{ kind: "tag", id: 99 }],
      },
      trail: [],
      items: [],
    });
    renderPage("/smart-folders/3");

    expect(await screen.findByText("A condition no longer works")).toBeTruthy();
    expect(screen.getByText("Tag: deleted")).toBeTruthy();
    expect(screen.getByText("Watched")).toBeTruthy();
  });

  it("表示中の id を運ぶ smartFolder の知らせで、骨組みなしに読み直す", async () => {
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });
    expect(server.reads.get(1)).toBe(1);

    await emitServerEvent("smartFolder", { ids: [7] });
    expect(server.reads.get(1)).toBe(1);

    const stored = server.folders.get(1);
    if (stored === undefined) throw new Error("no folder");
    stored.items = [
      ...stored.items,
      { kind: "video", video: video(12, { title: "Cat 3" }) },
    ];
    await emitServerEvent("smartFolder", { ids: [1, 5] });

    expect(await screen.findByRole("link", { name: "Cat 3" })).toBeTruthy();
    expect(server.reads.get(1)).toBe(2);
    expect(screen.queryByText("Loading…")).toBeNull();
  });

  it("ゲストは条件の行・選択なしでページを見て、Play を押せる", async () => {
    seed();
    const guestFolder = server.folders.get(1);
    if (guestFolder === undefined) throw new Error("no folder");
    server.folders.set(1, { ...guestFolder, settings: undefined });
    renderPage("/smart-folders/1", "guest");

    expect(
      await screen.findByRole("heading", { level: 1, name: "タグ: 猫" }),
    ).toBeTruthy();
    expect(screen.queryByText("Conditions:")).toBeNull();
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
    expect(screen.getByRole("link", { name: 'Play "タグ: 猫"' })).toBeTruthy();
  });

  it("無いフォルダは無くなった状態で、帯はライブラリだけ", async () => {
    renderPage("/smart-folders/40");

    expect(await screen.findByText("This smart folder no longer exists")).toBeTruthy();
    expect(crumbs()).toEqual(["Library"]);
    expect(screen.getByRole("link", { name: "Back to library" })).toBeTruthy();
  });

  it("子フォルダのカードは画面に入ってから値を読み、本数を出す", async () => {
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });
    expect(server.cardRequests).toEqual([]);

    await revealCards();

    expect(
      await screen.findByRole("link", { name: "未視聴, smart folder of 1 video" }),
    ).toBeTruthy();
    expect(server.cardRequests).toEqual([[5]]);
  });
});
