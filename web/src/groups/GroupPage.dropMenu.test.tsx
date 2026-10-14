import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { emitServerEvent } from "../api/fakeEventSource";
import { dropOn, layoutCards, lift, moveTo, release } from "../testing/cardDrag";
import { renderPage, seed, server, setUpGroupPageServer } from "../testing/groupPage";

setUpGroupPageServer();

// グループのページで動画を動画に落とす（specs/050-drag-organize/ui-design.md「Drop menus」
// 「The new group's name」「Group page」、R-6）。「Season」は左から Episode 1、子グループ
// Extras、Episode 2 を持つ。

/** cardTitles は格子のカードの題名か名前の欄の値を左から返す。 */
function cardTitles(): string[] {
  return Array.from(document.querySelectorAll("article")).map(
    (card) =>
      card.querySelector("h3")?.textContent ?? card.querySelector("input")?.value ?? "",
  );
}

async function openGrid() {
  seed();
  layoutCards();
  renderPage("/groups/1");
  await screen.findByRole("heading", { level: 1, name: "Season" });
}

function organizeWrites() {
  return server.writes.filter((each) => each.path.startsWith("/api/organize/"));
}

describe("GroupPage drop a video on a video", () => {
  it("「Make a group」は from 付きで送り、新しいグループのカードを落とした先の位置に出す", async () => {
    await openGrid();
    dropOn("Episode 2", 2, 0);
    const menu = await screen.findByRole("menu");
    expect(within(menu).getByText('Dropped on "Episode 1"')).toBeDefined();
    await userEvent
      .setup()
      .click(within(menu).getByRole("menuitem", { name: "Make a group" }));

    expect(cardTitles()).toEqual(["Episode 1", "Extras"]);
    await waitFor(() =>
      expect(organizeWrites()).toEqual([
        {
          method: "POST",
          path: "/api/organize/group",
          body: { items: [{ kind: "video", id: 11 }], target: 10, from: 1 },
        },
      ]),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    expect((field as HTMLInputElement).value).toBe("Episode 1");
    expect(cardTitles()).toEqual(["Episode 1", "Extras"]);
  });

  it("名前の欄を開いている間にグループが読み直されても、欄は入力ごと残る", async () => {
    await openGrid();
    const user = userEvent.setup();
    dropOn("Episode 2", 2, 0);
    await user.click(
      within(await screen.findByRole("menu")).getByRole("menuitem", {
        name: "Make a group",
      }),
    );
    const field = await screen.findByRole("textbox", { name: "Group name" });
    await user.clear(field);
    await user.type(field, "Pilot");
    // 開いているグループの知らせで読み直す。サーバーの項目は Episode 1 の位置に新しいグループを持つ。
    await emitServerEvent("group", { ids: [60, 1] });
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const kept = screen.getByRole("textbox", { name: "Group name" });
    expect((kept as HTMLInputElement).value).toBe("Pilot");
    expect(document.activeElement).toBe(kept);
    expect(cardTitles()).toEqual(["Pilot", "Extras"]);
  });

  it("名前の欄の Esc は何も送らずに閉じ、読み直したグループを出す", async () => {
    await openGrid();
    const user = userEvent.setup();
    dropOn("Episode 2", 2, 0);
    await user.click(
      within(await screen.findByRole("menu")).getByRole("menuitem", {
        name: "Make a group",
      }),
    );
    await screen.findByRole("textbox", { name: "Group name" });
    await user.keyboard("{Escape}");

    await waitFor(() =>
      expect(screen.queryByRole("textbox", { name: "Group name" })).toBeNull(),
    );
    expect(server.writes.some((each) => each.method === "PATCH")).toBe(false);
    expect(await screen.findByRole("link", { name: /^Episode 1, / })).toBeDefined();
  });

  it("子グループを動画の上へ持っていくと拒否の印になり、離しても何も送らない", async () => {
    await openGrid();
    lift(/^Extras, /, 1);
    moveTo(0);
    expect(screen.getByLabelText("Can't drop here")).toBeDefined();
    release(0);

    expect(screen.queryByRole("menu")).toBeNull();
    expect(cardTitles()).toEqual(["Episode 1", "Extras", "Episode 2"]);
    expect(organizeWrites()).toEqual([]);
  });
});
