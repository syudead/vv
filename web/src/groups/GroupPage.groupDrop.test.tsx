import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { layoutCards, lift, moveTo, release } from "../testing/cardDrag";
import {
  group,
  renderPage,
  seed,
  server,
  setUpGroupPageServer,
  video,
} from "../testing/groupPage";

setUpGroupPageServer();

// 子グループを含むカードを子グループに落としたときのメニュー（specs/050-drag-organize/
// ui-design.md「Drop menus」の Group on group、「Group page」）。

function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) => card.querySelector("h3")?.textContent ?? "",
  );
}

function writesTo(path: string) {
  return server.writes.filter((each) => each.path === path).map((each) => each.body);
}

/** openSeason は「Season」（Episode 1、Extras、Episode 2、Clips）を開く。 */
async function openSeason() {
  seed();
  const clips = group(6, "Clips", { videoCount: 1, videoIds: [14], openVideoId: 14 });
  server.groups.set(6, {
    group: clips,
    items: [{ id: 301, kind: "video", video: video(14, { title: "Clip" }) }],
  });
  server.groups.get(1)?.items.push({ id: 104, kind: "group", group: clips });
  layoutCards();
  renderPage("/groups/1");
  await screen.findByRole("heading", { level: 1, name: "Season" });
}

/** dropClipsOnExtras は「Clips」を「Extras」に落とし、開いたメニューを返す。 */
async function dropClipsOnExtras() {
  lift(/^Clips/, 3);
  moveTo(1);
  expect(screen.getByText("Put in or merge…")).toBeDefined();
  release(1);
  // dnd-kit は落とした後 50ms のクリックを止めるので、その後に項目を押す。
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 60));
  });
  const menu = await screen.findByRole("menu");
  expect(within(menu).getByText('Dropped on "Extras"')).toBeDefined();
  return menu;
}

describe("GroupPage group on group", () => {
  it("「Merge」は into の merge を送り、マージしたグループのカードが消える", async () => {
    await openSeason();
    const user = userEvent.setup();
    const menu = await dropClipsOnExtras();
    expect(
      Array.from(menu.querySelectorAll('[role="menuitem"]')).map(
        (item) => item.textContent,
      ),
    ).toEqual(["Put in as a child group", "Merge"]);

    await user.click(screen.getByRole("menuitem", { name: "Merge" }));

    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
    await waitFor(() =>
      expect(writesTo("/api/organize/merge")).toEqual([
        { items: [{ kind: "group", id: 6 }], into: 5 },
      ]),
    );
    expect(await screen.findByText('Merged 1 group into "Extras"')).toBeDefined();
    expect(server.groups.get(5)?.items.map((item) => item.video?.id)).toEqual([12, 14]);
  });

  it("「Put in as a child group」は from と to の move を送る", async () => {
    await openSeason();
    const user = userEvent.setup();
    await dropClipsOnExtras();

    await user.click(screen.getByRole("menuitem", { name: "Put in as a child group" }));

    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
    await waitFor(() =>
      expect(writesTo("/api/organize/move")).toEqual([
        { items: [{ kind: "group", id: 6 }], from: 1, to: 5 },
      ]),
    );
    expect(await screen.findByText('Moved 1 item to "Extras"')).toBeDefined();
  });

  it("動画とグループの選択は、すべてのカードを画面の順で 1 回の merge に送る", async () => {
    await openSeason();
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: 'Select the group "Clips"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "Episode 2"' }));
    await dropClipsOnExtras();

    await user.click(screen.getByRole("menuitem", { name: "Merge" }));

    await waitFor(() =>
      expect(writesTo("/api/organize/merge")).toEqual([
        {
          items: [
            { kind: "video", id: 11 },
            { kind: "group", id: 6 },
          ],
          into: 5,
        },
      ]),
    );
    expect(await screen.findByText('Merged 2 items into "Extras"')).toBeDefined();
    expect(server.writes).toHaveLength(1);
  });

  it("Esc でメニューを閉じると何も送らず、カードは元のまま", async () => {
    await openSeason();
    const user = userEvent.setup();
    await dropClipsOnExtras();

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    await act(async () => {
      await Promise.resolve();
    });
    expect(server.writes).toEqual([]);
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2", "Clips"]);
    expect(screen.queryByText("Put in or merge…")).toBeNull();
  });

  it("Undo でマージしたグループのカードが戻る", async () => {
    await openSeason();
    const user = userEvent.setup();
    await dropClipsOnExtras();
    await user.click(screen.getByRole("menuitem", { name: "Merge" }));
    await screen.findByText('Merged 1 group into "Extras"');

    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    expect(await screen.findByText("Undone")).toBeDefined();
    await waitFor(() =>
      expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2", "Clips"]),
    );
  });
});
