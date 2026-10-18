import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { GroupItem } from "../api/groups";
import { emitServerEvent } from "../api/fakeEventSource";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  video,
  group,
  type StoredGroup,
  server,
  json,
  renderPage,
  seed,
  summarize,
  trailBand,
  crumbs,
  layoutRows,
  listTitles,
  openList,
  openMenu,
  moveFirstDown,
  reveal,
  setUpGroupPageServer,
} from "../testing/groupPage";

setUpGroupPageServer();

describe("GroupPage", () => {
  it("子グループと動画を応答の順に並べ、見出しに名前と本数と長さを出す", async () => {
    seed();
    renderPage("/groups/1");
    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(screen.getByText("3 videos · 1:32:10")).toBeDefined();
    const cards = Array.from(document.querySelectorAll("article"));
    expect(cards.map((card) => card.querySelector("h3")?.textContent)).toEqual([
      "Episode 1",
      "Extras",
      "Episode 2",
    ]);
    expect(crumbs()).toEqual(["Library", "Season"]);
    expect(document.title).toBe("Season");
  });

  it("子グループを開くと帯が 1 段伸び、帯のリンクで上へ戻れる", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await user.click(
      await screen.findByRole("link", { name: "Extras, group of 1 video" }),
    );
    expect(
      await screen.findByRole("heading", { level: 1, name: "Extras" }),
    ).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season", "Extras"]);
    expect(screen.getByRole("link", { name: "Bonus" }).getAttribute("href")).toBe(
      "/videos/12?group=5",
    );

    await user.click(within(trailBand()).getByRole("link", { name: "Season" }));
    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season"]);
  });

  it("もう親でないグループの段は、何も言わずに帯から外す", async () => {
    seed();
    server.groups.set(9, { group: group(9, "Old parent"), items: [] });
    renderPage("/groups/5?trail=9,1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    expect(crumbs()).toEqual(["Library", "Season", "Extras"]);
  });

  it("ページから開いた動画と「Play」は ?group を運び、戻る先はこのページ", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    const play = await screen.findByRole("link", { name: 'Play "Season"' });
    expect(play.getAttribute("href")).toBe("/videos/10?group=1");

    await user.click(screen.getByRole("link", { name: "Episode 2" }));
    expect(
      await screen.findByText('Opened /videos/11?group=1 {"from":"/groups/1"}'),
    ).toBeDefined();
  });

  it("表示中の id を運ぶ group の知らせで、選択を残したまま読み直す", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode 1"' }));
    expect(server.reads.get(1)).toBe(1);

    await emitServerEvent("group", { ids: [99] });
    expect(server.reads.get(1)).toBe(1);

    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, name: "Season (renamed)" };
    stored.items = [...stored.items].reverse();
    await emitServerEvent("group", { ids: [1, 99] });

    expect(
      await screen.findByRole("heading", { level: 1, name: "Season (renamed)" }),
    ).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season (renamed)"]);
    const titles = Array.from(document.querySelectorAll("article h3")).map(
      (heading) => heading.textContent,
    );
    expect(titles).toEqual(["Episode 2", "Extras", "Episode 1"]);
    expect(
      (
        screen.getByRole("checkbox", { name: 'Select "Episode 1"' }) as HTMLButtonElement
      ).getAttribute("aria-checked"),
    ).toBe("true");
  });

  it("読み直しで外れた動画は選択から外れ、増えた動画は「Select all」でまた選べる", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode 1"' }));
    const selectAll = screen.getByRole("button", { name: "Select all" });
    await user.click(selectAll);
    expect(screen.getByText("3 videos selected")).toBeDefined();
    expect(selectAll).toHaveProperty("disabled", true);

    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, videoCount: 3, videoIds: [12, 11, 13] };
    stored.items = [
      stored.items[1] as GroupItem,
      stored.items[2] as GroupItem,
      { id: 104, kind: "video", video: video(13, { title: "Episode 3" }) },
    ];
    await emitServerEvent("group", { ids: [1] });

    await screen.findByRole("checkbox", { name: 'Select "Episode 3"' });
    expect(await screen.findByText("2 videos selected")).toBeDefined();
    expect(screen.getByRole("button", { name: "Select all" })).toHaveProperty(
      "disabled",
      false,
    );
    await user.click(screen.getByRole("button", { name: "Select all" }));
    expect(screen.getByText("3 videos selected")).toBeDefined();
  });

  it("404 ならグループがもう無いと示し、名前を出さない", async () => {
    renderPage("/groups/404?trail=1");
    expect(await screen.findByText("This group no longer exists")).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Back to library" }).getAttribute("href"),
    ).toBe("/");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Library");
    expect(crumbs()).toEqual(["Library"]);
    expect(screen.queryByRole("link", { name: /^Play/ })).toBeNull();
  });

  it("ゲストは編集の操作なしでページを見る", async () => {
    seed();
    renderPage("/groups/1", "guest");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    expect(screen.getByRole("link", { name: 'Play "Season"' })).toBeDefined();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
    expect(screen.queryByRole("button", { name: /^Favorite/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /to a playlist$/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();

    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: "List" }));
    const table = screen.getByRole("table", { name: "Items" });
    expect(within(table).getAllByRole("row")).toHaveLength(3);
    expect(within(table).queryAllByRole("checkbox")).toHaveLength(0);
    expect(within(table).getByText("1")).toBeDefined();
  });

  it("自動グループはフォルダの行を出し、手で作ったグループは出さない", async () => {
    seed();
    const user = userEvent.setup();
    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, folder: { rootId: 1, path: "anime/season" } };
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    expect(screen.getByText("From the folder")).toBeDefined();
    const folderLink = await screen.findByRole("link", { name: "Media/anime/season" });
    expect(folderLink.getAttribute("href")).toBe("/folders/1/anime/season");

    await user.click(screen.getByRole("link", { name: "Extras, group of 1 video" }));
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    expect(screen.queryByText("From the folder")).toBeNull();
  });

  it("番兵が近づくと nextCursor で続きを読み、失敗は Retry で読み直す", async () => {
    seed();
    const stored = server.groups.get(1) as StoredGroup;
    stored.nextCursor = "c1";
    server.pages.set("c1", {
      items: [{ id: 104, kind: "video", video: video(13, { title: "Episode 3" }) }],
    });
    server.pagesFail = true;
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });

    act(() => reveal());
    expect(await screen.findByText("Couldn't load more of the group")).toBeDefined();
    server.pagesFail = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("link", { name: "Episode 3" })).toBeDefined();
    expect(server.pageRequests).toEqual(["c1", "c1"]);
    expect(document.querySelectorAll("article")).toHaveLength(4);
  });

  it("キーボードで項目を動かすと、上にある項目で reorder を送り、読み直しても新しい順で出る", async () => {
    seed();
    layoutRows();
    const user = userEvent.setup();
    await openList(user);
    expect(listTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);

    await moveFirstDown(user);

    await waitFor(() =>
      expect(server.writes).toEqual([
        {
          method: "POST",
          path: "/api/organize/reorder",
          body: { groupId: 1, itemIds: [101], after: 102 },
        },
      ]),
    );
    expect(listTitles()).toEqual(["Extras", "Episode 1", "Episode 2"]);

    cleanup();
    renderPage("/groups/1");
    await screen.findByRole("table", { name: "Items" });
    expect(listTitles()).toEqual(["Extras", "Episode 1", "Episode 2"]);
  });

  it("グリッドにはハンドルも × も無い", async () => {
    seed();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    expect(screen.queryByRole("button", { name: /^Reorder/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /from the group$/ })).toBeNull();
  });

  it("並べ替えの失敗は最後に認めた順へ戻して知らせる", async () => {
    seed();
    layoutRows();
    const user = userEvent.setup();
    await openList(user);
    const fetchMock = vi.mocked(fetch);
    const original = fetchMock.getMockImplementation() as typeof fetch;
    fetchMock.mockImplementation((input, init) =>
      String(input).endsWith("/organize/reorder")
        ? Promise.resolve(json({ code: "internal", message: "x" }, 500))
        : original(input, init),
    );

    await moveFirstDown(user);

    expect(await screen.findByText(/^Couldn't reorder:/)).toBeDefined();
    expect(listTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
  });

  it("行の × は確認なしに外し、件数を変え、フォーカスを次の行の × へ移す", async () => {
    seed();
    const user = userEvent.setup();
    await openList(user);
    await user.click(
      screen.getByRole("button", { name: 'Remove "Episode 1" from the group' }),
    );

    await waitFor(() => expect(listTitles()).toEqual(["Extras", "Episode 2"]));
    expect(server.writes).toEqual([
      { method: "POST", path: "/api/groups/1/items/remove", body: { itemIds: [101] } },
    ]);
    expect(screen.getByText("2 videos · 1:32:10")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement?.getAttribute("aria-label")).toBe(
        'Remove "Extras" from the group',
      ),
    );
  });

  it("続きのページが残るグループで項目を外すと、詰めた位置に合わせて先頭から読み直す", async () => {
    seed();
    const stored = server.groups.get(1) as StoredGroup;
    stored.nextCursor = "c1";
    const user = userEvent.setup();
    await openList(user);
    expect(server.reads.get(1)).toBe(1);

    await user.click(
      screen.getByRole("button", { name: 'Remove "Episode 1" from the group' }),
    );

    await waitFor(() => expect(server.reads.get(1)).toBe(2));
    await waitFor(() => expect(listTitles()).toEqual(["Extras", "Episode 2"]));
    expect(server.pageRequests).toEqual([]);
  });

  it("送っている移動の間に届いた知らせの読み直しは、移動が答えてから読み、行を戻さない", async () => {
    seed();
    layoutRows();
    const user = userEvent.setup();
    await openList(user);
    const fetchMock = vi.mocked(fetch);
    const original = fetchMock.getMockImplementation() as typeof fetch;
    let answer: (() => void) | null = null;
    fetchMock.mockImplementation((input, init) =>
      String(input).endsWith("/organize/reorder")
        ? new Promise<Response>((resolve) => {
            answer = () => resolve(original(input, init));
          })
        : original(input, init),
    );

    await moveFirstDown(user);
    await waitFor(() => expect(answer).not.toBeNull());
    // Deliver a notification before the queued move reaches the server.
    await emitServerEvent("group", { ids: [1] });
    expect(server.reads.get(1)).toBe(1);
    expect(listTitles()).toEqual(["Extras", "Episode 1", "Episode 2"]);

    await act(async () => {
      answer?.();
      await Promise.resolve();
    });
    await waitFor(() => expect(server.reads.get(1)).toBe(2));
    expect(listTitles()).toEqual(["Extras", "Episode 1", "Episode 2"]);
  });

  it("行の × で最後の項目を外すとグループは解散され、道筋の親へ移ってトーストを出す", async () => {
    seed();
    const user = userEvent.setup();
    await openList(user, "/groups/5?trail=1");
    await user.click(
      screen.getByRole("button", { name: 'Remove "Bonus" from the group' }),
    );

    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(screen.getByText('Dissolved "Extras"')).toBeDefined();
    expect(screen.queryByText(/^Removed /)).toBeNull();
    expect(screen.queryByText(/Couldn't remove/)).toBeNull();
    expect(server.writes).toEqual([
      { method: "POST", path: "/api/groups/5/items/remove", body: { itemIds: [201] } },
    ]);
    expect(crumbs()).toEqual(["Library", "Season"]);
  });

  it("「Remove from group」で全部の項目を外すとグループは解散され、ライブラリへ移る", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/5");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "Bonus"' }));
    await user.click(screen.getByRole("button", { name: "Remove from group" }));

    expect(await screen.findByText("Library")).toBeDefined();
    expect(screen.getByText('Dissolved "Extras"')).toBeDefined();
    expect(screen.queryByText(/^Removed /)).toBeNull();
    expect(server.groups.has(5)).toBe(false);
  });

  it("別のタブで解散されたグループは、知らせの読み直しの 404 で無くなったと見せる", async () => {
    seed();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });

    server.groups.delete(5);
    await emitServerEvent("group", { ids: [1, 5] });
    expect(await screen.findByText("This group no longer exists")).toBeDefined();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
    expect(screen.queryByRole("link", { name: /^Play/ })).toBeNull();
  });

  it("選んだ 2 項目を「Remove from group」で外し、件数を知らせて選択を外す", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode 1"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "Extras"' }));
    await user.click(screen.getByRole("button", { name: "Remove from group" }));

    expect(await screen.findByText('Removed 2 items from "Season"')).toBeDefined();
    expect(server.writes).toEqual([
      {
        method: "POST",
        path: "/api/groups/1/items/remove",
        body: { itemIds: [101, 102] },
      },
    ]);
    expect(
      Array.from(document.querySelectorAll("article h3")).map((each) => each.textContent),
    ).toEqual(["Episode 2"]);
    expect(screen.queryByRole("region", { name: "Selection actions" })).toBeNull();
  });

  it("対象が途中で消えていたら、編集の失敗を知らせてグループを読み直す", async () => {
    seed();
    const user = userEvent.setup();
    await openList(user);
    const stored = server.groups.get(1) as StoredGroup;
    stored.items = stored.items.filter((item) => item.id !== 103);
    summarize(stored);

    await user.click(
      screen.getByRole("button", { name: 'Remove "Episode 2" from the group' }),
    );
    expect(
      await screen.findByText("The group changed in the meantime, so it was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(listTitles()).toEqual(["Episode 1", "Extras"]));
    expect(server.reads.get(1)).toBe(2);
  });

  it("改名は前後を削った名前を送り、空では送れない", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    await openMenu(user, "Rename…");

    const dialog = await screen.findByRole("dialog", { name: "Rename group" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    expect(within(dialog).getByRole("button", { name: "Save" })).toHaveProperty(
      "disabled",
      true,
    );
    await user.clear(field);
    await user.type(field, "   ");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(server.writes).toEqual([]);

    await user.clear(field);
    await user.type(field, "  Bonus clips  ");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "Bonus clips" }),
    ).toBeDefined();
    expect(server.writes).toEqual([
      { method: "PATCH", path: "/api/groups/5", body: { name: "Bonus clips" } },
    ]);
    expect(crumbs()).toEqual(["Library", "Season", "Bonus clips"]);
    expect(document.title).toBe("Bonus clips");
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "More" })),
    );
  });

  it("解散はまず確かめ、確定すると道筋の親へ移ってトーストを出す", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    await openMenu(user, "Dissolve…");

    const dialog = await screen.findByRole("alertdialog", { name: 'Dissolve "Extras"?' });
    expect(
      within(dialog).getByText(
        "The group is dissolved. Its videos and groups stay in the library.",
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(server.writes).toEqual([]);

    await openMenu(user, "Dissolve…");
    await user.click(await screen.findByRole("button", { name: "Dissolve" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(server.writes).toEqual([{ method: "DELETE", path: "/api/groups/5" }]);
    expect(screen.getByText('Dissolved "Extras"')).toBeDefined();
    expect(crumbs()).toEqual(["Library", "Season"]);
  });

  it("道筋の無い自動グループの解散は 2 文目を足し、ライブラリへ移る", async () => {
    seed();
    const stored = server.groups.get(1) as StoredGroup;
    stored.group = { ...stored.group, folder: { rootId: 1, path: "anime/season" } };
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await openMenu(user, "Dissolve…");
    const dialog = await screen.findByRole("alertdialog", { name: 'Dissolve "Season"?' });
    expect(within(dialog).getByText(/Choose Automatic on the folder page/)).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Dissolve" }));
    expect(await screen.findByText("Library")).toBeDefined();
  });

  it("タグに変えると確かめてから送り、道筋の親へ移って結果を知らせる", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/5?trail=1");
    await screen.findByRole("heading", { level: 1, name: "Extras" });
    await openMenu(user, "Turn the group into a tag…");
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Turn "Extras" into a tag?',
    });
    await user.click(within(dialog).getByRole("button", { name: "Turn into a tag" }));

    expect(
      await screen.findByRole("heading", { level: 1, name: "Season" }),
    ).toBeDefined();
    expect(
      screen.getByText('Created the tag "Extras" and dissolved the group'),
    ).toBeDefined();
    expect(server.writes).toEqual([{ method: "POST", path: "/api/groups/5/tag" }]);
  });

  it("ゲストのリスト表示にはハンドルも × も無い", async () => {
    seed();
    const user = userEvent.setup();
    renderPage("/groups/1", "guest");
    await screen.findByRole("heading", { level: 1, name: "Season" });
    await user.click(screen.getByRole("radio", { name: "List" }));
    expect(screen.queryByRole("button", { name: /^Reorder/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /from the group$/ })).toBeNull();
  });

  it("画面の文言はカタログか利用者のデータだけ", async () => {
    seed();
    enablePseudoLocale();
    renderPage("/groups/5?trail=1");
    await waitFor(() => expect(document.querySelectorAll("article")).toHaveLength(1));
    expectCatalogTextOnly(document.body, ["Season", "Extras", "Bonus", "1:00"]);
  });

  it("「Add to group」はこのグループへ足したときだけ読み直し、カードのグループ自身はピッカーに出さない", async () => {
    seed();
    const base = globalThis.fetch;
    const adds: { path: string; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>((input, init) => {
        const url = new URL(String(input), "http://localhost");
        if (url.pathname === "/api/groups" && (init?.method ?? "GET") === "GET") {
          return Promise.resolve(
            json({
              groups: [server.groups.get(5)?.group, server.groups.get(1)?.group],
            }),
          );
        }
        if (init?.method === "POST") {
          adds.push({ path: url.pathname, body: JSON.parse(String(init.body)) });
          return Promise.resolve(json(server.groups.get(1)?.group));
        }
        return base(input, init);
      }),
    );
    const user = userEvent.setup();
    renderPage("/groups/1");
    await screen.findByRole("heading", { level: 1, name: "Season" });

    await user.click(
      screen.getByRole("button", { name: 'Add group "Extras" to a group' }),
    );
    const list = await screen.findByRole("listbox", { name: "Groups" });
    expect(within(list).queryByRole("option", { name: "Extras" })).toBeNull();
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: 'Add "Episode 1" to a group' }));
    await user.click(await screen.findByRole("option", { name: "Extras" }));
    expect(await screen.findByText('Added to "Extras"')).toBeDefined();
    expect(server.reads.get(1)).toBe(1);

    await user.click(screen.getByRole("button", { name: 'Add "Episode 2" to a group' }));
    await user.click(await screen.findByRole("option", { name: "Season" }));
    await waitFor(() => expect(server.reads.get(1)).toBe(2));
    expect(adds).toEqual([
      { path: "/api/groups/5/items", body: { videoIds: [10], groupIds: [] } },
      { path: "/api/groups/1/items", body: { videoIds: [11], groupIds: [] } },
    ]);
  });
});
