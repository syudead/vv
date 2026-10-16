import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { getTagKinds } from "../api/tagKinds";
import {
  historyBack,
  hold,
  install,
  kind,
  kindDeleteReleases,
  kindsTab,
  probe,
  renderPage,
  server,
  setUpTagsPageServer,
  tag,
  tagsTab,
} from "../testing/tagsPage";

setUpTagsPageServer();

type User = ReturnType<typeof userEvent.setup>;

/** openKinds は「Kinds」のタブへ移り、種類の表を返す。 */
async function openKinds(user: User): Promise<HTMLElement> {
  await user.click(kindsTab());
  const panel = await screen.findByRole("tabpanel", { name: /Kinds/ });
  return within(panel).findByRole("table", { name: "Kinds" });
}

function kindRow(name: string): HTMLElement {
  return screen.getByTitle(name).closest<HTMLElement>("[data-kind-id]")!;
}

function kindNames(table: HTMLElement): string[] {
  return within(table)
    .getAllByRole("link")
    .map((link) => link.textContent ?? "");
}

describe("TagsPage「Kinds」のタブ（specs/051-tag-kinds/ui-design.md「Kinds tab」）", () => {
  it("タブは「Kinds 4」で、開くと種類を名前の自然順に並べ、URL は tab=kinds、見出しの行は「New kind」", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");
    await waitFor(() => expect(kindsTab().textContent).toBe("Kinds4"));

    const table = await openKinds(user);
    expect(kindNames(table)).toEqual(["Character", "Genre", "Place", "Work"]);
    expect(within(kindRow("Genre")).getByText("0 tags")).toBeDefined();
    expect(new URLSearchParams(probe.search).get("tab")).toBe("kinds");
    expect(screen.getByRole("button", { name: "New kind" })).toBeDefined();
    expect(screen.queryByRole("button", { name: "New tag" })).toBeNull();
    expect(
      screen.getByText(
        "A tag has one kind or none. Set it from the Tags tab's selection bar. Deleting a kind leaves its tags without one.",
      ),
    ).toBeDefined();
  });

  it("種類を足し、名前を変え、消せる。改名・削除のあとに「Tags」へ戻るとタグの一覧を読み直す（受け入れ条件 3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage({ url: "/tags?tab=kinds" });
    const table = await screen.findByRole("table", { name: "Kinds" });

    // 作成: 名前の位置に入り、フォーカスはその名前へ。
    await user.click(screen.getByRole("button", { name: "New kind" }));
    await user.type(
      screen.getByRole("textbox", { name: "New kind name" }),
      "Studio{Enter}",
    );
    await waitFor(() =>
      expect(kindNames(table)).toEqual(["Character", "Genre", "Place", "Studio", "Work"]),
    );
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: 'Show the tags of the kind "Studio"' }),
      ),
    );
    expect(kindsTab().textContent).toBe("Kinds5");
    const studio = server.tagKinds.find((item) => item.name === "Studio")!;
    // サーバーの側でタグにその種類を付けておく（タグの行の種類の操作は別の単位）。
    server.tags[0]!.kind = { id: studio.id, name: "Studio" };

    // 改名: 行はその場で書き換わり、名前の位置へ移る。
    await user.click(within(kindRow("Studio")).getByRole("button", { name: "Rename" }));
    const input = screen.getByRole("textbox", { name: 'New name for "Studio"' });
    await user.clear(input);
    await user.type(input, "Anthology{Enter}");
    await waitFor(() =>
      expect(kindNames(table)).toEqual([
        "Anthology",
        "Character",
        "Genre",
        "Place",
        "Work",
      ]),
    );
    expect(server.tags[0]!.kind).toEqual({ id: studio.id, name: "Anthology" });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(kindRow("Anthology")).getByRole("button", { name: "Rename" }),
      ),
    );

    // 削除: 確認の窓の文言は tagCount で決まり、204 で行が消え、トーストが出て、次の行へ。
    await waitFor(() =>
      expect(within(kindRow("Anthology")).getByText("1 tag")).toBeDefined(),
    );
    await user.click(
      within(kindRow("Anthology")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Delete the kind "Anthology"?',
    });
    expect(
      within(dialog).getByText(
        "The 1 tag of this kind stays, without a kind. This can't be undone.",
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByTitle("Anthology")).toBeNull());
    expect(await screen.findByText('Deleted the kind "Anthology"')).toBeDefined();
    expect(server.tags[0]!.kind).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(kindRow("Character")).getByRole("button", { name: "Rename" }),
      ),
    );
    // 改名・削除のたびに共有のタグの一覧を取り直す（タグはどれも種類の名前を持つ）。
    await waitFor(() => expect(server.fullGetCalls).toBeGreaterThanOrEqual(2));

    // 「Tags」へ戻ると先頭のページを読み直し、消した種類はタグから消えている。
    const before = server.getCalls;
    await user.click(tagsTab());
    await waitFor(() => expect(server.getCalls).toBe(before + 1));
    await screen.findByTitle("旅行");
  });

  it("改名も削除もしていなければ、「Tags」へ戻っても一覧を読み直さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");
    await openKinds(user);
    const before = server.getCalls;
    await user.click(tagsTab());
    await screen.findByTitle("Anime");
    expect(server.getCalls).toBe(before);
  });

  it("重複した名前は入力の下に出し、入力と文字を残し、種類は変わらない（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage({ url: "/tags?tab=kinds" });
    await screen.findByRole("table", { name: "Kinds" });

    await user.click(screen.getByRole("button", { name: "New kind" }));
    const create = screen.getByRole("textbox", { name: "New kind name" });
    await user.type(create, "Genre{Enter}");
    expect(await screen.findByText('A kind named "Genre" already exists.')).toBeDefined();
    expect((create as HTMLInputElement).value).toBe("Genre");
    expect(create.getAttribute("aria-invalid")).toBe("true");
    expect(server.tagKinds).toHaveLength(4);
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "New kind" }),
      ),
    );

    await user.click(within(kindRow("Work")).getByRole("button", { name: "Rename" }));
    const rename = screen.getByRole("textbox", { name: 'New name for "Work"' });
    await user.clear(rename);
    await user.type(rename, "Place{Enter}");
    expect(await screen.findByText('A kind named "Place" already exists.')).toBeDefined();
    expect((rename as HTMLInputElement).value).toBe("Place");
    expect(server.tagKinds.find((item) => item.id === 2)?.name).toBe("Work");
  });

  it("名前の変更・削除で種類がもう無ければ、閉じてトーストを出し、一覧を読み直す", async () => {
    const user = userEvent.setup();
    install();
    renderPage({ url: "/tags?tab=kinds" });
    const table = await screen.findByRole("table", { name: "Kinds" });

    await user.click(
      within(kindRow("Place")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
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
    await waitFor(() => expect(kindNames(table)).toEqual(["Character", "Genre", "Work"]));

    await user.click(within(kindRow("Work")).getByRole("button", { name: "Rename" }));
    server.tagKinds = server.tagKinds.filter((item) => item.name !== "Work");
    const rename = screen.getByRole("textbox", { name: 'New name for "Work"' });
    await user.type(rename, "s{Enter}");
    await waitFor(() => expect(kindNames(table)).toEqual(["Character", "Genre"]));
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("種類の名前を押すと、その種類で絞った「Tags」のタブを開く（ほかの条件は残す）", async () => {
    const user = userEvent.setup();
    install();
    renderPage({ url: "/tags?unused=1&sort=countDesc&tab=kinds" });
    await screen.findByRole("table", { name: "Kinds" });

    const link = screen.getByRole("link", { name: 'Show the tags of the kind "Genre"' });
    expect(link.getAttribute("href")).toBe("/tags?unused=1&kind=1&sort=countDesc");
    await user.click(link);
    await waitFor(() => expect(probe.search).toBe("?unused=1&kind=1&sort=countDesc"));
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
  });

  it("種類が 0 なら空の状態の「New kind」から作れる。読み込みの失敗は Retry で読み直す", async () => {
    const user = userEvent.setup();
    server.tagKinds = [];
    server.failTagKindGets = true;
    install();
    renderPage({ url: "/tags?tab=kinds" });

    expect(await screen.findByText("Couldn't load the kinds")).toBeDefined();
    server.failTagKindGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("No kinds yet")).toBeDefined();
    const buttons = screen.getAllByRole("button", { name: "New kind" });
    expect(buttons).toHaveLength(2);
    await user.click(buttons[1]!);
    await user.type(
      screen.getByRole("textbox", { name: "New kind name" }),
      "Genre{Enter}",
    );
    const table = await screen.findByRole("table", { name: "Kinds" });
    await waitFor(() => expect(kindNames(table)).toEqual(["Genre"]));
  });

  it("タグの数は開くたびに読み直す（タグの削除などで数が変わる）", async () => {
    const user = userEvent.setup();
    server.tags = [
      ...server.tags,
      tag({ id: 9, name: "Robot", kind: { id: 1, name: "Genre" } }),
    ];
    server.tagKinds = [kind({ id: 1, name: "Genre" })];
    install();
    renderPage();
    await screen.findByTitle("Robot");
    await openKinds(user);
    await waitFor(() =>
      expect(within(kindRow("Genre")).getByText("1 tag")).toBeDefined(),
    );
    await user.click(tagsTab());
    await screen.findByTitle("Robot");
    server.tags = server.tags.filter((item) => item.id !== 9);
    await openKinds(user);
    await waitFor(() =>
      expect(within(kindRow("Genre")).getByText("0 tags")).toBeDefined(),
    );
  });

  it("「Kinds」のタブで画面を開いたときも、前に読んだ種類があれば取り直す", async () => {
    install();
    // 前の画面で読んだ種類の一覧が共有の保持に残っている。
    await getTagKinds();
    server.tags = [
      ...server.tags,
      tag({ id: 9, name: "Robot", kind: { id: 1, name: "Genre" } }),
    ];
    const before = server.tagKindGets;
    renderPage({ url: "/tags?tab=kinds" });
    await screen.findByRole("table", { name: "Kinds" });
    await waitFor(() =>
      expect(within(kindRow("Genre")).getByText("1 tag")).toBeDefined(),
    );
    expect(server.tagKindGets).toBe(before + 1);
  });

  it("取り直しに失敗したら、前に読んだ並びではなく失敗と Retry を出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");
    await openKinds(user);
    await user.click(tagsTab());
    await screen.findByTitle("Anime");
    server.failTagKindGets = true;
    await user.click(kindsTab());

    expect(await screen.findByText("Couldn't load the kinds")).toBeDefined();
    expect(screen.queryByRole("table", { name: "Kinds" })).toBeNull();
    expect(
      (screen.getByRole("button", { name: "New kind" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    server.failTagKindGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("table", { name: "Kinds" })).toBeDefined();
  });

  it("削除の確認の窓を開いたまま戻るでタブを離れると窓を閉じ、「Kinds」へ戻っても出ない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");
    await openKinds(user);
    await user.click(
      within(kindRow("Genre")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await screen.findByRole("alertdialog", { name: 'Delete the kind "Genre"?' });

    await historyBack();
    await waitFor(() => expect(tagsTab().getAttribute("aria-selected")).toBe("true"));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    await openKinds(user);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("削除の送信中に戻るを押すと、削除が終わってから「Tags」へ移り、タグの一覧を読み直す", async () => {
    const user = userEvent.setup();
    server.tags = [
      ...server.tags,
      tag({ id: 9, name: "Robot", kind: { id: 1, name: "Genre" } }),
    ];
    install();
    renderPage();
    await screen.findByTitle("Robot");
    await openKinds(user);
    await user.click(
      within(kindRow("Genre")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Delete the kind "Genre"?',
    });
    hold.kindDeletes = true;
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(kindDeleteReleases).toHaveLength(1));

    await historyBack();
    // 送信中は「Kinds」のまま待つ。
    // 窓が開いている間はほかの要素が読み上げから隠れるので、hidden で探す。
    expect(
      screen
        .getByRole("tab", { name: /^Kinds/, hidden: true })
        .getAttribute("aria-selected"),
    ).toBe("true");
    expect(screen.getByRole("alertdialog")).toBeDefined();
    const before = server.getCalls;
    await act(async () => {
      kindDeleteReleases[0]!();
      await Promise.resolve();
    });

    await waitFor(() => expect(tagsTab().getAttribute("aria-selected")).toBe("true"));
    await waitFor(() => expect(server.getCalls).toBe(before + 1));
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});
