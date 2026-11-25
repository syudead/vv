import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { emitServerEvent } from "../api/fakeEventSource";
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

/** The rows of the Names section, main name first. */
function nameRows(): string[] {
  const section = screen.getByRole("region", { name: "Names" });
  return Array.from(
    section.querySelectorAll<HTMLElement>("[data-slot=page-section-body] > div"),
  )
    .map((row) => row.querySelector("span.wrap-anywhere")?.textContent ?? "")
    .filter((name) => name !== "");
}

async function openTag(id: number) {
  install();
  renderPage({ url: `/tags/${String(id)}`, tagPage: true });
  await screen.findByRole("heading", { level: 1 });
}

async function openNameMenu(user: User, name: string, item: string) {
  await user.click(screen.getByRole("button", { name: `More actions for "${name}"` }));
  await user.click(await screen.findByRole("menuitem", { name: item }));
}

async function chooseTarget(user: User, scope: HTMLElement, label: string, name: string) {
  await user.type(within(scope).getByRole("combobox", { name: label }), name);
  await user.click(await within(scope).findByRole("option", { name: new RegExp(name) }));
}

describe("TagPage 読み込み", () => {
  it("URL から直接開くと getTag で読み、なくなったタグはなくなった状態を出す", async () => {
    await openTag(2);
    expect(server.tagGets).toEqual([2]);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("Anime");
    expect(nameRows()).toEqual(["Anime", "アニメ"]);

    install();
    renderPage({ url: "/tags/50", tagPage: true });
    expect(await screen.findByText("This tag no longer exists")).toBeDefined();
    expect(screen.getByRole("link", { name: "Back to tags" }).getAttribute("href")).toBe(
      "/tags",
    );
  });

  it("tags イベントでタグを読み直し、その場で更新する", async () => {
    await openTag(2);
    server.tags[1]!.synonyms = ["アニメ", "animation"];

    await emitServerEvent("tags");

    await waitFor(() => expect(nameRows()).toEqual(["Anime", "アニメ", "animation"]));
    expect(server.tagGets).toEqual([2, 2]);
  });
});

describe("TagPage 名前の操作", () => {
  it("Main は名前を送り、新しい名前を題にして旧名を名前の中に出す", async () => {
    const user = userEvent.setup();
    await openTag(2);

    await user.click(screen.getByRole("button", { name: 'Make "アニメ" the main name' }));

    await waitFor(() =>
      expect(screen.getByRole("heading", { level: 1 }).textContent).toContain("アニメ"),
    );
    expect(server.synonymOps).toEqual([{ op: "promote", id: 2, name: "アニメ" }]);
    expect(nameRows()).toEqual(["アニメ", "Anime"]);
  });

  it("別のタグへ移すと、選んだタグの id を送り、名前が消える", async () => {
    const user = userEvent.setup();
    await openTag(2);

    await openNameMenu(user, "アニメ", "Move to another tag…");
    const dialog = await screen.findByRole("dialog", {
      name: 'Move "アニメ" to another tag',
    });
    await chooseTarget(user, dialog, "Tag to move it to", "Drama");
    await user.click(within(dialog).getByRole("button", { name: "Move" }));

    await waitFor(() => expect(nameRows()).toEqual(["Anime"]));
    expect(server.synonymOps).toEqual([
      { op: "move", id: 2, name: "アニメ", targetId: 3 },
    ]);
  });

  it("独立させると、名前が消えて新しいタグへのリンクが出る", async () => {
    const user = userEvent.setup();
    await openTag(2);

    await openNameMenu(user, "アニメ", "Make it a tag of its own");

    const link = await screen.findByRole("link", { name: 'Open "アニメ"' });
    expect(link.getAttribute("href")).toBe("/tags/100");
    expect(nameRows()).toEqual(["Anime"]);
    expect(server.synonymOps).toEqual([{ op: "detach", id: 2, name: "アニメ" }]);
  });

  it("名前がもうこのタグのものでなければ理由を出し、タグを読み直す", async () => {
    const user = userEvent.setup();
    await openTag(2);
    server.tags[1]!.synonyms = [];

    await user.click(screen.getByRole("button", { name: 'Make "アニメ" the main name' }));

    expect(
      await screen.findByText(
        '"アニメ" is no longer a name of this tag, so the tag was read again.',
      ),
    ).toBeDefined();
    await waitFor(() => expect(nameRows()).toEqual(["Anime"]));
    expect(server.tagGets).toEqual([2, 2]);
  });

  it("操作の相手のタグがもうなければ理由を出し、タグを読み直す", async () => {
    const user = userEvent.setup();
    await openTag(2);

    await openNameMenu(user, "アニメ", "Move to another tag…");
    const dialog = await screen.findByRole("dialog");
    await chooseTarget(user, dialog, "Tag to move it to", "Drama");
    server.tags = server.tags.filter((item) => item.id !== 3);
    await user.click(within(dialog).getByRole("button", { name: "Move" }));

    expect(
      await screen.findByText(
        "A tag this change named no longer exists, so the tag was read again.",
      ),
    ).toBeDefined();
    await waitFor(() => expect(server.tagGets).toEqual([2, 2]));
    expect(nameRows()).toEqual(["Anime", "アニメ"]);
  });
});

describe("TagPage 統合", () => {
  it("別のタグへ統合すると、来た一覧へ戻り、そのタグはもういない", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 2, name: "Anime", videoCount: 3 }),
      tag({ id: 3, name: "Drama", synonyms: ["ドラマ"] }),
    ];
    install();
    renderPage({ url: "/tags?sort=countDesc", tagPage: true });
    await user.click(await screen.findByTitle("Drama"));
    await screen.findByRole("heading", { level: 1, name: /Drama/ });

    const merge = screen.getByRole("region", { name: "Merge into another tag" });
    await chooseTarget(user, merge, "Tag to keep", "Anime");
    await user.click(within(merge).getByRole("button", { name: "Merge…" }));
    const confirm = await screen.findByRole("alertdialog");
    await user.click(within(confirm).getByRole("button", { name: "Merge" }));

    await waitFor(() => expect(probe.pathname).toBe("/tags"));
    expect(probe.search).toBe("?sort=countDesc");
    expect(await screen.findByTitle("Anime")).toBeDefined();
    expect(screen.queryByTitle("Drama")).toBeNull();
  });
});
