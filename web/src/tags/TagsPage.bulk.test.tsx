import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { maxTagBatch } from "../api/tags";
import { t } from "../i18n";
import {
  install,
  probe,
  renderPage,
  server,
  setUpTagsPageServer,
  tag,
} from "../testing/tagScreens";

setUpTagsPageServer();

type User = ReturnType<typeof userEvent.setup>;

function recordCard(name: string): HTMLElement {
  return screen.getByTitle(name).closest<HTMLElement>('[data-slot="record-card"]')!;
}

function check(user: User, name: string) {
  return user.click(screen.getByRole("checkbox", { name: t.tags.row.select(name) }));
}

function bar(): HTMLElement {
  return screen.getByRole("region", { name: t.tags.selection.region });
}

function barButton(name: string): HTMLElement {
  return within(bar()).getByRole("button", { name });
}

async function chooseMore(user: User, item: string) {
  await user.click(barButton(t.tags.selection.more));
  await user.click(await screen.findByRole("menuitem", { name: item }));
}

function listPageRequests(): URLSearchParams[] {
  return server.pageRequests.filter((params) => params.get("limit") === "100");
}

describe("TagsPage カードの選択", () => {
  it("Shift で範囲を選び、選択中にカードを押すと開かずに選び、Escape で外す", async () => {
    const user = userEvent.setup();
    server.tags = ["A", "B", "C", "D"].map((name, index) => tag({ id: index + 1, name }));
    install();
    renderPage();
    await screen.findByTitle("A");

    await check(user, "A");
    await user.keyboard("{Shift>}");
    await check(user, "C");
    await user.keyboard("{/Shift}");
    expect(within(bar()).getByText(t.tags.selection.count(3))).toBeDefined();

    await user.click(screen.getByTitle("D"));
    expect(probe.pathname).toBe("/tags");
    expect(within(bar()).getByText(t.tags.selection.count(4))).toBeDefined();
    expect(recordCard("D").hasAttribute("data-selected")).toBe(true);

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("region", { name: t.tags.selection.region })).toBeNull();
  });

  it("一致するものをすべて選ぶと、残りのページを読み込んでからすべてを選ぶ", async () => {
    const user = userEvent.setup();
    // Rows measure one card high, so the first page's end is not drawn and no more is read.
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(function (
      this: HTMLElement,
    ) {
      return this.hasAttribute("data-index") ? 108 : 0;
    });
    server.tags = Array.from({ length: 150 }, (_, index) =>
      tag({ id: index + 1, name: `Tag${String(index + 1).padStart(3, "0")}` }),
    );
    install();
    renderPage();
    await screen.findByTitle("Tag001");

    await check(user, "Tag001");
    await user.click(barButton(t.tags.selection.selectAll));
    expect(
      await screen.findByRole("menuitem", { name: t.tags.selection.allLoaded(100) }),
    ).toBeDefined();
    await user.click(
      screen.getByRole("menuitem", { name: t.tags.selection.allMatching(150) }),
    );

    expect(await within(bar()).findByText(t.tags.selection.count(150))).toBeDefined();
    const rest = server.pageRequests.filter((params) => params.get("limit") === "200");
    expect(rest).toHaveLength(1);
    expect(rest[0]!.has("cursor")).toBe(true);
    expect(server.batchCalls).toEqual([]);
  });

  it(`${String(maxTagBatch)} を超えて選ぶと、リクエストの前に操作を止めて理由を出す`, async () => {
    const user = userEvent.setup();
    const count = maxTagBatch + 1;
    server.tags = Array.from({ length: count }, (_, index) =>
      tag({ id: index + 1, name: `Tag${String(index + 1)}`, tentative: true }),
    );
    // One page holds every tag, so every one of them is loaded; only the top rows are drawn.
    server.pageLimitOverride = count;
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(function (
      this: HTMLElement,
    ) {
      return this.hasAttribute("data-index") ? 108 : 0;
    });
    install();
    renderPage();
    await screen.findByTitle("Tag1");

    await check(user, "Tag1");
    await user.click(barButton(t.tags.selection.selectAll));
    expect(
      (
        await screen.findByRole("menuitem", {
          name: t.tags.selection.allMatching(count),
        })
      ).getAttribute("aria-disabled"),
    ).toBe("true");
    await user.click(
      screen.getByRole("menuitem", { name: t.tags.selection.allLoaded(count) }),
    );

    expect(await within(bar()).findByText(t.tags.selection.count(count))).toBeDefined();
    const limit = t.tags.selection.overLimit(maxTagBatch);
    expect(within(bar()).getByText(limit)).toBeDefined();
    for (const name of [
      t.tags.selection.mergeInto,
      t.tags.selection.setKind,
      t.tags.selection.confirm,
      t.tags.selection.more,
    ]) {
      expect(barButton(name)).toHaveProperty("disabled", true);
    }
    await user.click(barButton(t.tags.selection.confirm));
    expect(server.batchCalls).toEqual([]);
    expect(server.impactCalls).toEqual([]);
  });
});

describe("TagsPage まとめての操作", () => {
  it("確定は選んだタグを送り、カードを確定にして選択から外す", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true }),
      tag({ id: 2, name: "Beta", tentative: true }),
      tag({ id: 3, name: "Gamma" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await check(user, "Alpha");
    await check(user, "Beta");
    await user.click(barButton(t.tags.selection.confirm));

    expect(await screen.findByText(t.tags.selection.confirmed(2, 0))).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "confirm", ids: [1, 2] }]);
    expect(recordCard("Alpha").hasAttribute("data-tentative")).toBe(false);
    expect(recordCard("Beta").hasAttribute("data-tentative")).toBe(false);
    expect(screen.queryByRole("region", { name: t.tags.selection.region })).toBeNull();
  });

  it("種類を選ぶとその種類を送り、カードにその種類を出す", async () => {
    const user = userEvent.setup();
    server.tags = [tag({ id: 1, name: "Alpha" }), tag({ id: 2, name: "Beta" })];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await check(user, "Alpha");
    await check(user, "Beta");
    await user.click(barButton(t.tags.selection.setKind));
    await user.click(await screen.findByRole("menuitemradio", { name: "Genre" }));

    expect(
      await screen.findByText(t.tags.selection.kindSet("Genre", 2, 0)),
    ).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "setKind", ids: [1, 2], kindId: 1 }]);
    expect(within(recordCard("Alpha")).getByText("Genre")).toBeDefined();
    expect(within(recordCard("Beta")).getByText("Genre")).toBeDefined();
  });

  it("却下は影響する数を出してから送り、却下したカードを外し、確定のタグは選んだまま残す", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await check(user, "Alpha");
    await check(user, "Beta");
    await chooseMore(user, t.tags.selection.reject);
    const dialog = await screen.findByRole("alertdialog");
    expect(
      await within(dialog).findByText(t.tags.bulkDialog.rejectSome(1, 2, 2)),
    ).toBeDefined();
    expect(server.impactCalls).toEqual([{ action: "reject", ids: [1, 2] }]);
    await user.click(
      within(dialog).getByRole("button", { name: t.tags.rejectDialog.submit }),
    );

    expect(await screen.findByText(t.tags.selection.rejected(1, 1))).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "reject", ids: [1, 2] }]);
    expect(screen.queryByTitle("Alpha")).toBeNull();
    expect(recordCard("Beta").hasAttribute("data-selected")).toBe(true);
  });

  it("削除で notFoundIds が返れば古いものとして伝え、一覧を読み直す", async () => {
    const user = userEvent.setup();
    server.tags = [tag({ id: 1, name: "Alpha" }), tag({ id: 2, name: "Beta" })];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await check(user, "Alpha");
    await check(user, "Beta");
    await chooseMore(user, t.tags.selection.delete);
    const dialog = await screen.findByRole("alertdialog");
    await within(dialog).findByText(t.tags.bulkDialog.deleteAll(2, 0));
    // Beta is merged or deleted elsewhere after the count.
    server.tags = server.tags.filter((item) => item.id !== 2);
    const reads = listPageRequests().length;
    await user.click(
      within(dialog).getByRole("button", { name: t.tags.deleteDialog.submit }),
    );

    expect(await screen.findByText(t.tags.selection.stale)).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "delete", ids: [1, 2] }]);
    await waitFor(() => expect(listPageRequests().length).toBe(reads + 1));
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    expect(screen.queryByTitle("Alpha")).toBeNull();
  });

  it("1 つのタグへの統合は選んだタグを送り、統合先のカードに名前を足し、消えていた元は古いものとして読み直す", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Alpha", videoCount: 1 }),
      tag({ id: 2, name: "Beta", videoCount: 2 }),
      tag({ id: 3, name: "Gamma", videoCount: 3 }),
    ];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await check(user, "Alpha");
    await check(user, "Beta");
    await user.click(barButton(t.tags.selection.mergeInto));
    const dialog = await screen.findByRole("dialog", {
      name: t.tags.mergeDialog.titleMany(2),
    });
    await user.type(
      within(dialog).getByRole("combobox", { name: t.tags.mergeDialog.target }),
      "Gamma",
    );
    await user.click(await within(dialog).findByRole("option", { name: /Gamma/ }));
    await within(dialog).findByText(t.tags.mergeDialog.warningMany(2, 3, "Gamma"));
    expect(server.impactCalls).toEqual([{ action: "merge", ids: [1, 2] }]);
    server.tags = server.tags.filter((item) => item.id !== 2);
    const reads = listPageRequests().length;
    await user.click(
      within(dialog).getByRole("button", { name: t.tags.mergeDialog.submit }),
    );

    expect(await screen.findByText(t.tags.merged("Alpha", "Gamma"))).toBeDefined();
    expect(await screen.findByText(t.tags.selection.stale)).toBeDefined();
    expect(screen.queryByTitle("Alpha")).toBeNull();
    expect(within(recordCard("Gamma")).getByText("Alpha")).toBeDefined();
    await waitFor(() => expect(listPageRequests().length).toBe(reads + 1));
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    expect(screen.queryByRole("region", { name: t.tags.selection.region })).toBeNull();
  });
});
