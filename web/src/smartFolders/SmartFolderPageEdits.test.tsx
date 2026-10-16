import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { emitServerEvent } from "../api/fakeEventSource";
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { group, json } from "../testing/groupPage";
import {
  folderCard,
  renderPage,
  revealCards,
  revealSentinel,
  seed,
  server,
  setUpSmartFolderServer,
  video,
} from "../testing/smartFolderPage";

// スマートフォルダのページの所有者の操作と、読み直しの間の振る舞い
// （specs/052-smart-folders/ui-design.md「Header」「Editing」「New child」「Deleting」「Body」「States」）。

setUpSmartFolderServer();

beforeEach(() => {
  __resetTagsForTest();
  __resetTagKindsForTest();
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.releasePointerCapture ??= () => undefined;
  Element.prototype.setPointerCapture ??= () => undefined;
  Element.prototype.scrollIntoView ??= () => undefined;
});

async function openMenuItem(name: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "More" }));
  await user.click(await screen.findByRole("menuitem", { name }));
  return user;
}

/** emptyFolder は一致するものの無いフォルダ（id 2）を置く。 */
function emptyFolder(missing = false) {
  server.folders.set(2, {
    smartFolder: folderCard(2, "空", { videoCount: 0, openVideoId: undefined }),
    settings: {
      rule: missing ? { tags: [99] } : { q: "none" },
      sort: "addedDesc",
      missing: missing ? [{ kind: "tag", id: 99 }] : [],
    },
    trail: [],
    items: [],
  });
}

describe("見出しの More（所有者）", () => {
  it("Edit… で名前を直すと、トーストを出して見出しと題を読み直し、More へ戻る", async () => {
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });
    expect(server.reads.get(1)).toBe(1);

    const user = await openMenuItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });
    const name = within(dialog).getByRole("textbox", { name: "Name" });
    await user.clear(name);
    await user.type(name, "タグ: ねこ{Enter}");

    expect(await screen.findByText('Saved "タグ: ねこ"')).toBeTruthy();
    expect(
      await screen.findByRole("heading", { level: 1, name: "タグ: ねこ" }),
    ).toBeTruthy();
    expect(server.writes).toEqual([
      { method: "PATCH", path: "/api/smart-folders/1", body: { name: "タグ: ねこ" } },
    ]);
    expect(server.reads.get(1)).toBe(2);
    expect(document.title).toBe("タグ: ねこ");
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "More" })),
    );
  });

  it("New child smart folder… で作ると、トーストを出して本体を読み直す", async () => {
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    const user = await openMenuItem("New child smart folder…");
    const dialog = await screen.findByRole("dialog", {
      name: 'New smart folder in "タグ: 猫"',
    });
    await user.click(within(dialog).getByRole("checkbox", { name: "Playable only" }));
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "好き{Enter}");

    expect(await screen.findByText('Created "好き"')).toBeTruthy();
    expect(server.writes[0]).toMatchObject({
      method: "POST",
      body: { name: "好き", parentId: 1 },
    });
    await waitFor(() => expect(server.reads.get(1)).toBe(2));
  });

  it("子フォルダを Delete… で消すと、親のページへ移ってトーストを出す", async () => {
    seed();
    renderPage("/smart-folders/5");
    await screen.findByRole("heading", { level: 1, name: "未視聴" });

    const user = await openMenuItem("Delete…");
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "未視聴"?' });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "タグ: 猫" }),
    ).toBeTruthy();
    expect(screen.getByText('Deleted "未視聴"')).toBeTruthy();
    expect(server.writes).toEqual([
      { method: "DELETE", path: "/api/smart-folders/5", body: undefined },
    ]);
  });

  it("親の無いフォルダがもう無かった（404）なら、トーストなしでライブラリへ移る", async () => {
    seed();
    server.writeAnswer = () => json({ code: "not_found", message: "x" }, 404);
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    const user = await openMenuItem("Delete…");
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "タグ: 猫"?' });
    // 子孫の数を数え終わるまで押せない。
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "Delete" }).hasAttribute("disabled"),
      ).toBe(false),
    );
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Library")).toBeTruthy();
    expect(screen.queryByText(/^Deleted/)).toBeNull();
  });

  it("Edit… でフォルダがもう無かった（404）なら、無くなった状態にする", async () => {
    seed();
    server.writeAnswer = () => json({ code: "not_found", message: "x" }, 404);
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    const user = await openMenuItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "!{Enter}");

    expect(await screen.findByText("This smart folder no longer exists")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("空の状態の Edit conditions…", () => {
  it("一致するものが無ければ、所有者に直す窓を開く操作を出す", async () => {
    emptyFolder();
    renderPage("/smart-folders/2");
    await screen.findByText("Nothing matches these conditions yet");

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Edit conditions…" }));

    expect(await screen.findByRole("dialog", { name: "Edit smart folder" })).toBeTruthy();
  });

  it("欠けた条件の状態にも出す", async () => {
    emptyFolder(true);
    renderPage("/smart-folders/2");
    await screen.findByText("A condition no longer works");

    expect(screen.getByRole("button", { name: "Edit conditions…" })).toBeTruthy();
  });

  it("条件の届かない応答（ゲスト）には More も Edit conditions… も出さない", async () => {
    emptyFolder();
    const stored = server.folders.get(2);
    if (stored === undefined) throw new Error("no folder");
    server.folders.set(2, { ...stored, settings: undefined });
    renderPage("/smart-folders/2", "guest");
    await screen.findByText("Nothing matches these conditions yet");

    expect(screen.queryByRole("button", { name: "Edit conditions…" })).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
  });
});

describe("選択", () => {
  it("読み直して並ばなくなった動画は選択から外れ、一括の操作に入らない", async () => {
    const user = userEvent.setup();
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    await user.click(screen.getByRole("checkbox", { name: 'Select "Cat 2"' }));
    expect(screen.getByText("1 video selected")).toBeTruthy();

    // Cat 2 からタグが外れて条件に合わなくなった。
    const stored = server.folders.get(1);
    if (stored === undefined) throw new Error("no folder");
    stored.items = stored.items.filter(
      (item) => item.kind !== "video" || item.video?.id !== 11,
    );
    await emitServerEvent("smartFolder", { ids: [1] });

    await waitFor(() => expect(screen.queryByRole("link", { name: "Cat 2" })).toBeNull());
    expect(screen.queryByText("1 video selected")).toBeNull();
  });

  it("並んだまま読み直した動画の選択は残る", async () => {
    const user = userEvent.setup();
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    await user.click(screen.getByRole("checkbox", { name: 'Select "Cat 1"' }));
    await emitServerEvent("smartFolder", { ids: [1] });

    await waitFor(() => expect(server.reads.get(1)).toBe(2));
    expect(screen.getByText("1 video selected")).toBeTruthy();
  });
});

describe("カードのタグ", () => {
  it("動画を選んでいる間も、子フォルダのカードのタグを押すとそのタグで絞ったライブラリを開く", async () => {
    const user = userEvent.setup();
    seed();
    server.cards.set(
      5,
      folderCard(5, "未視聴", {
        videoCount: 1,
        tags: [
          {
            id: 3,
            name: "猫",
            manual: true,
            fromFolder: false,
            tentative: false,
            kind: null,
          },
        ],
      }),
    );
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });
    await revealCards([5]);
    const folder = await screen.findByRole("link", {
      name: "未視聴, smart folder of 1 video",
    });

    await user.click(screen.getByRole("checkbox", { name: 'Select "Cat 1"' }));
    const card = folder.closest("article") as HTMLElement;
    await user.click(within(card).getByRole("button", { name: "Filter by 猫" }));

    expect(await screen.findByText("Library")).toBeTruthy();
  });
});

describe("ページ送り", () => {
  /** pagedChild は「未視聴」（id 5）に続きのページを 1 つ持たせる。 */
  function pagedChild() {
    seed();
    const child = server.folders.get(5);
    if (child === undefined) throw new Error("no folder");
    child.nextCursor = "c1";
    server.pages.set("5:c1", {
      items: [{ kind: "video", video: video(13, { title: "Cat 4" }) }],
    });
  }

  it("続きを足しても祖先の条件は読み直さず、読み直したときだけ読む", async () => {
    pagedChild();
    renderPage("/smart-folders/5");
    await screen.findByRole("heading", { level: 1, name: "未視聴" });
    await screen.findByText("Tag: 猫");
    expect(server.reads.get(1)).toBe(1);

    await revealSentinel();
    expect(await screen.findByRole("link", { name: "Cat 4" })).toBeTruthy();
    expect(server.reads.get(1)).toBe(1);

    await emitServerEvent("smartFolder", { ids: [5] });
    await waitFor(() => expect(server.reads.get(1)).toBe(2));
  });

  it("続きが読めなければ LoadMoreRow を出し、Retry で読み直す", async () => {
    const user = userEvent.setup();
    pagedChild();
    renderPage("/smart-folders/5");
    await screen.findByRole("heading", { level: 1, name: "未視聴" });

    server.failPage = true;
    await revealSentinel();
    expect(
      await screen.findByText("Couldn't load more of the smart folder"),
    ).toBeTruthy();

    server.failPage = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("link", { name: "Cat 4" })).toBeTruthy();
  });

  it("続きの 404 は無くなった状態にする", async () => {
    pagedChild();
    server.pages.delete("5:c1");
    renderPage("/smart-folders/5");
    await screen.findByRole("heading", { level: 1, name: "未視聴" });

    await revealSentinel();
    expect(await screen.findByText("This smart folder no longer exists")).toBeTruthy();
  });
});

describe("読めなかったとき", () => {
  it("最初の読み込みの失敗は ErrorState で、Retry で読み直す", async () => {
    const user = userEvent.setup();
    seed();
    server.failDetail = true;
    renderPage("/smart-folders/1");
    expect(await screen.findByText("Couldn't load the smart folder")).toBeTruthy();

    server.failDetail = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "タグ: 猫" }),
    ).toBeTruthy();
  });

  it("静かな読み直しの失敗は並んでいる中身を残す", async () => {
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    server.failDetail = true;
    await emitServerEvent("smartFolder", { ids: [1] });
    await waitFor(() => expect(server.reads.get(1)).toBe(2));

    expect(screen.getByRole("heading", { level: 1, name: "タグ: 猫" })).toBeTruthy();
    expect(screen.queryByText("Couldn't load the smart folder")).toBeNull();
  });

  it("つなぎ直したら読み直す", async () => {
    seed();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    // 最初の接続では読み直さず、2 度目（つなぎ直し）で読み直す。
    await emitServerEvent("open");
    expect(server.reads.get(1)).toBe(1);
    await emitServerEvent("open");
    await waitFor(() => expect(server.reads.get(1)).toBe(2));
  });
});

/** withGroup は「タグ: 猫」（id 1）の動画の後にグループ「Season」（id 20、動画 30 と 31）を足す。 */
function withGroup() {
  seed();
  const stored = server.folders.get(1);
  if (stored === undefined) throw new Error("no folder");
  const fur = {
    id: 7,
    name: "毛",
    manual: true,
    fromFolder: false,
    tentative: false,
    kind: null,
  };
  stored.items = [
    ...stored.items.map((item) =>
      item.kind === "video" && item.video?.id === 11
        ? { kind: "video" as const, video: video(11, { title: "Cat 2", tags: [fur] }) }
        : item,
    ),
    {
      kind: "group",
      group: group(20, "Season", {
        videoIds: [30, 31],
        tags: [
          {
            id: 4,
            name: "冬",
            manual: true,
            fromFolder: false,
            tentative: false,
            kind: null,
          },
        ],
      }),
    },
  ];
}

describe("動画とグループの選択", () => {
  it("グループを選び、Select all で並んだ動画とグループを選び、Esc で解く", async () => {
    const user = userEvent.setup();
    withGroup();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    await user.click(screen.getByRole("checkbox", { name: 'Select the group "Season"' }));
    expect(screen.getByText("2 videos selected")).toBeTruthy();

    // 選択中にグループのカードのタグを押すと、グループの選択を切り替える。
    const season = screen
      .getByRole("checkbox", { name: 'Select the group "Season"' })
      .closest("article") as HTMLElement;
    const [winter] = within(season).getAllByText("冬");
    if (winter === undefined) throw new Error("no chip");
    await user.click(winter);
    expect(screen.queryByText("2 videos selected")).toBeNull();

    await user.click(screen.getByRole("checkbox", { name: 'Select "Cat 1"' }));
    // 選択中に動画のカードのタグを押すと、その動画の選択を切り替える。
    const cat2 = screen
      .getByRole("checkbox", { name: 'Select "Cat 2"' })
      .closest("article") as HTMLElement;
    const [chip] = within(cat2).getAllByText("毛");
    if (chip === undefined) throw new Error("no chip");
    await user.click(chip);
    expect(screen.getByText("2 videos selected")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Select all" }));
    expect(screen.getByText("4 videos selected")).toBeTruthy();

    await user.keyboard("{Escape}");
    expect(screen.queryByText(/videos? selected/)).toBeNull();
  });
});

describe("リスト表示", () => {
  it("子フォルダ、動画、グループをライブラリの行で並べる", async () => {
    localStorage.setItem(
      "vv.view.v2",
      JSON.stringify({ zoom: 1, view: "list", sort: "addedDesc" }),
    );
    withGroup();
    renderPage("/smart-folders/1");
    await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });

    const table = screen.getByRole("table", { name: "Items" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(4);
    expect(
      within(rows[0] as HTMLElement)
        .getByRole("link")
        .getAttribute("href"),
    ).toBe("/smart-folders/5");
    expect(within(rows[3] as HTMLElement).getByText("Season")).toBeTruthy();
    localStorage.clear();
  });
});

describe("経路と欠けた条件", () => {
  it("読めない id は読まずに無くなった状態にする", async () => {
    renderPage("/smart-folders/abc");

    expect(await screen.findByText("This smart folder no longer exists")).toBeTruthy();
    expect(server.reads.size).toBe(0);
  });

  it("欠けた参照が祖先の条件にあれば、その祖先の名前を説明に足す", async () => {
    server.folders.set(3, {
      smartFolder: folderCard(3, "消えた", { videoCount: 0, openVideoId: undefined }),
      settings: {
        rule: { tags: [99] },
        sort: "addedDesc",
        missing: [{ kind: "tag", id: 99 }],
      },
      trail: [],
      items: [],
    });
    server.folders.set(6, {
      smartFolder: folderCard(6, "子", { videoCount: 0, openVideoId: undefined }),
      settings: {
        rule: { watch: "watched" },
        sort: "addedDesc",
        missing: [{ kind: "tag", id: 99 }],
      },
      trail: [{ id: 3, name: "消えた" }],
      items: [],
    });
    renderPage("/smart-folders/6");

    expect(
      await screen.findByText(
        'A tag, kind, person or organization in the conditions was deleted, so nothing matches. The condition is in "消えた".',
      ),
    ).toBeTruthy();
  });
});

describe("祖先の条件", () => {
  it("祖先を読めなければ、その行は骨組みのまま", async () => {
    seed();
    server.folders.delete(1);
    renderPage("/smart-folders/5");
    await screen.findByRole("heading", { level: 1, name: "未視聴" });
    await waitFor(() => expect(server.reads.get(1)).toBe(1));

    expect(screen.getByRole("link", { name: 'From "タグ: 猫":' })).toBeTruthy();
    expect(screen.queryByText("Tag: 猫")).toBeNull();
  });
});
