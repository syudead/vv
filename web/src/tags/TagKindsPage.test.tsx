import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { getTagKinds } from "../api/tagKinds";
import {
  install,
  kind,
  probe,
  renderPage,
  server,
  setUpTagsPageServer,
  tag,
} from "../testing/tagsPage";

setUpTagsPageServer();

type User = ReturnType<typeof userEvent.setup>;

function section(): HTMLElement {
  return screen.getByRole("region", { name: "All kinds" });
}

function kindRow(name: string): HTMLElement {
  return screen.getByTitle(name).closest<HTMLElement>("[data-kind-id]")!;
}

function kindNames(): string[] {
  return [...section().querySelectorAll<HTMLElement>("[data-kind-id] [title]")].map(
    (node) => node.textContent ?? "",
  );
}

async function openKindsPage(url = "/tags/kinds"): Promise<HTMLElement> {
  install();
  renderPage({ url });
  await screen.findByTitle("Character");
  return section();
}

async function openMenu(user: User, name: string, item: string) {
  await user.click(
    within(kindRow(name)).getByRole("button", {
      name: `More actions for the kind "${name}"`,
    }),
  );
  await user.click(await screen.findByRole("menuitem", { name: item }));
}

describe("TagKindsPage 種類のページ", () => {
  it("種類を名前の自然順に、色の点・名前・タグの数のリンクで並べ、戻るリンクと説明を出す", async () => {
    await openKindsPage();

    expect(kindNames()).toEqual(["Character", "Genre", "Place", "Work"]);
    const genre = kindRow("Genre");
    expect(
      genre.querySelector('[data-slot="kind-mark"]')?.getAttribute("data-kind-colour"),
    ).toBe("1");
    const count = within(genre).getByRole("link", {
      name: '0 tags of the kind "Genre"',
    });
    expect(count.textContent).toBe("0 tags");
    expect(count.getAttribute("href")).toBe("/tags?kind=1");
    expect(screen.getByRole("link", { name: "Tags" }).getAttribute("href")).toBe("/tags");
    expect(
      screen.getByText(
        "A tag has one kind or none. Set it from the tag list's selection bar. Video cards list tags of kinds marked First on cards before the rest. Deleting a kind leaves its tags without one.",
      ),
    ).toBeDefined();
  });

  it("「New kind」の欄で Enter を押すと作り、欄を空にして残す。名前を変え、消せる", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    const field = screen.getByRole("textbox", { name: "New kind name" });
    await user.type(field, "Studio{Enter}");
    await waitFor(() =>
      expect(kindNames()).toEqual(["Character", "Genre", "Place", "Studio", "Work"]),
    );
    expect((field as HTMLInputElement).value).toBe("");
    expect(document.activeElement).toBe(field);

    await openMenu(user, "Studio", "Rename");
    const input = screen.getByRole("textbox", { name: 'New name for "Studio"' });
    await user.clear(input);
    await user.type(input, "Anthology{Enter}");
    await waitFor(() =>
      expect(kindNames()).toEqual(["Anthology", "Character", "Genre", "Place", "Work"]),
    );
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(kindRow("Anthology")).getByRole("button", {
          name: 'More actions for the kind "Anthology"',
        }),
      ),
    );

    await openMenu(user, "Anthology", "Delete…");
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Delete the kind "Anthology"?',
    });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByTitle("Anthology")).toBeNull());
    expect(await screen.findByText('Deleted the kind "Anthology"')).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(kindRow("Character")).getByRole("button", {
          name: 'More actions for the kind "Character"',
        }),
      ),
    );
  });

  it("種類を消すと、その種類のタグはタグの一覧で種類なしになる", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 9, name: "Robot", kind: { id: 1, name: "Genre", cardFirst: false } }),
    ];
    await openKindsPage();
    await waitFor(() =>
      expect(within(kindRow("Genre")).getByRole("link").textContent).toBe("1 tag"),
    );

    await openMenu(user, "Genre", "Delete…");
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Delete the kind "Genre"?',
    });
    expect(
      within(dialog).getByText(
        "The 1 tag of this kind stays, without a kind. This can't be undone.",
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByTitle("Genre")).toBeNull());

    await user.click(screen.getByRole("link", { name: "Tags" }));
    const row = (await screen.findByTitle("Robot")).closest<HTMLElement>(
      "[data-tag-id]",
    )!;
    expect(within(row).queryByText("Genre")).toBeNull();
    expect(server.tags[0]!.kind).toBeNull();
  });

  it("タグの数のリンクを押すと、その種類で絞ったタグの一覧を開く", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 9, name: "Robot", kind: { id: 1, name: "Genre", cardFirst: false } }),
      tag({ id: 10, name: "Tokyo", kind: { id: 3, name: "Place", cardFirst: false } }),
    ];
    await openKindsPage();

    await user.click(
      await screen.findByRole("link", { name: '1 tag of the kind "Genre"' }),
    );
    expect(await screen.findByTitle("Robot")).toBeDefined();
    expect(probe.pathname).toBe("/tags");
    expect(new URLSearchParams(probe.search).get("kind")).toBe("1");
    expect(screen.queryByTitle("Tokyo")).toBeNull();
  });

  it("「First on cards」を切り替えると種類に保存し、スイッチが新しい値を示す", async () => {
    const user = userEvent.setup();
    await openKindsPage();
    const toggle = within(kindRow("Work")).getByRole("switch", {
      name: "First on cards",
    });
    expect(toggle.getAttribute("aria-checked")).toBe("false");

    await user.click(toggle);

    await waitFor(() => expect(toggle.getAttribute("aria-checked")).toBe("true"));
    expect(server.tagKinds.find((item) => item.name === "Work")?.cardFirst).toBe(true);
  });

  it("重複した名前は入力の下に出し、入力と文字を残し、種類は変わらない（Edge Case）", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    const create = screen.getByRole("textbox", { name: "New kind name" });
    await user.type(create, "Genre{Enter}");
    expect(await screen.findByText('A kind named "Genre" already exists.')).toBeDefined();
    expect((create as HTMLInputElement).value).toBe("Genre");
    expect(create.getAttribute("aria-invalid")).toBe("true");
    expect(server.tagKinds).toHaveLength(4);

    await openMenu(user, "Work", "Rename");
    const rename = screen.getByRole("textbox", { name: 'New name for "Work"' });
    await user.clear(rename);
    await user.type(rename, "Place{Enter}");
    expect(await screen.findByText('A kind named "Place" already exists.')).toBeDefined();
    expect((rename as HTMLInputElement).value).toBe("Place");
    expect(server.tagKinds.find((item) => item.id === 2)?.name).toBe("Work");
  });

  it("名前の変更・削除で種類がもう無ければ、閉じてトーストを出し、一覧を読み直す", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    await openMenu(user, "Place", "Delete…");
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Delete the kind "Place"?',
    });
    expect(
      within(dialog).getByText("No tags have this kind. This can't be undone."),
    ).toBeDefined();
    server.tagKinds = server.tagKinds.filter((item) => item.name !== "Place");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(
      await screen.findByText("This kind no longer exists, so the list was reloaded"),
    ).toBeDefined();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    await waitFor(() => expect(kindNames()).toEqual(["Character", "Genre", "Work"]));

    await openMenu(user, "Work", "Rename");
    server.tagKinds = server.tagKinds.filter((item) => item.name !== "Work");
    const rename = screen.getByRole("textbox", { name: 'New name for "Work"' });
    await user.type(rename, "s{Enter}");
    await waitFor(() => expect(kindNames()).toEqual(["Character", "Genre"]));
    expect(screen.queryByRole("textbox", { name: 'New name for "Work"' })).toBeNull();
  });

  it("種類が 0 なら空の状態の下の「New kind」の欄から作れる。読み込みの失敗は Retry で読み直す", async () => {
    const user = userEvent.setup();
    server.tagKinds = [];
    server.failTagKindGets = true;
    install();
    renderPage({ url: "/tags/kinds" });

    expect(await screen.findByText("Couldn't load the kinds")).toBeDefined();
    expect(screen.queryByRole("textbox", { name: "New kind name" })).toBeNull();
    server.failTagKindGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("No kinds yet")).toBeDefined();
    await user.type(
      screen.getByRole("textbox", { name: "New kind name" }),
      "Genre{Enter}",
    );
    await waitFor(() => expect(kindNames()).toEqual(["Genre"]));
    expect(screen.queryByText("No kinds yet")).toBeNull();
  });

  it("前に読んだ種類があっても開くたびに取り直し、失敗したら前の並びではなく失敗を出す", async () => {
    install();
    await getTagKinds();
    server.tags = [
      tag({ id: 9, name: "Robot", kind: { id: 1, name: "Genre", cardFirst: false } }),
    ];
    server.tagKinds = [kind({ id: 1, name: "Genre" })];
    const before = server.tagKindGets;
    const view = renderPage({ url: "/tags/kinds" });
    await waitFor(() =>
      expect(within(kindRow("Genre")).getByRole("link").textContent).toBe("1 tag"),
    );
    expect(server.tagKindGets).toBe(before + 1);
    view.unmount();

    server.failTagKindGets = true;
    renderPage({ url: "/tags/kinds" });
    expect(await screen.findByText("Couldn't load the kinds")).toBeDefined();
    expect(screen.queryByTitle("Genre")).toBeNull();
  });
});
