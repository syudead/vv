import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import {
  historyBack,
  historyForward,
  hold,
  install,
  kind,
  kindDeleteReleases,
  probe,
  renderPage,
  server,
  setUpTagsPageServer,
  tagsTab,
} from "../testing/tagsPage";

setUpTagsPageServer();

// 「Kinds」のタブの取り消し・失敗・フォーカスの行き先と、狭い幅の「Actions」
// （specs/051-tag-kinds/ui-design.md「Kinds tab」）。作成・改名・削除の通る道は
// TagsPage.kinds.test.tsx にある。

type User = ReturnType<typeof userEvent.setup>;

function kindRow(name: string): HTMLElement {
  return screen.getByTitle(name).closest<HTMLElement>("[data-kind-id]")!;
}

function kindNames(table: HTMLElement): string[] {
  return within(table)
    .getAllByRole("link")
    .map((link) => link.textContent ?? "");
}

async function openKindsPage(): Promise<HTMLElement> {
  install();
  renderPage({ url: "/tags?tab=kinds" });
  return screen.findByRole("table", { name: "Kinds" });
}

/** openDeleteDialog は行の「More actions」から削除の確認の窓を開く。 */
async function openDeleteDialog(user: User, name: string): Promise<HTMLElement> {
  await user.click(within(kindRow(name)).getByRole("button", { name: "More actions" }));
  await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
  return screen.findByRole("alertdialog", { name: `Delete the kind "${name}"?` });
}

describe("TagsPage「Kinds」のタブの取り消しと失敗", () => {
  it("改名の入力は Esc で閉じ、何も送らず、その行の「Rename」へフォーカスを戻す", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    await user.click(within(kindRow("Work")).getByRole("button", { name: "Rename" }));
    await user.type(screen.getByRole("textbox", { name: 'New name for "Work"' }), "s");
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("textbox")).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(kindRow("Work")).getByRole("button", { name: "Rename" }),
      ),
    );
    expect(server.tagKinds.find((item) => item.id === 2)?.name).toBe("Work");
  });

  it("削除の確認の窓の Cancel は何も消さず、その行の「More actions」へフォーカスを戻す", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    const dialog = await openDeleteDialog(user, "Place");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(kindRow("Place")).getByRole("button", { name: "More actions" }),
      ),
    );
    expect(server.tagKinds).toHaveLength(4);
  });

  it("削除の送信中は Esc で窓を閉じず、応答が届いてから閉じる", async () => {
    const user = userEvent.setup();
    const table = await openKindsPage();

    const dialog = await openDeleteDialog(user, "Place");
    hold.kindDeletes = true;
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(kindDeleteReleases).toHaveLength(1));

    await user.keyboard("{Escape}");
    expect(screen.getByRole("alertdialog")).toBe(dialog);

    await act(async () => {
      kindDeleteReleases[0]!();
      await Promise.resolve();
    });
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(kindNames(table)).not.toContain("Place");
  });

  it("削除が 404 以外で失敗したら窓の中に誤りを出し、窓と行を残す", async () => {
    const user = userEvent.setup();
    const table = await openKindsPage();

    const dialog = await openDeleteDialog(user, "Place");
    server.failNextKindDelete = true;
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await within(dialog).findByText("Something went wrong on the server."),
    ).toBeDefined();
    expect(screen.getByRole("alertdialog")).toBe(dialog);
    // 窓の背面は読み上げから隠れているので、行は title で探す。
    expect(screen.getByTitle("Place")).toBeDefined();

    // もう一度押せば消せ、誤りは消える。
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(kindNames(table)).not.toContain("Place");
  });

  it("最後の種類を消すと、フォーカスは見出しの行の「New kind」へ移る", async () => {
    const user = userEvent.setup();
    server.tagKinds = [kind({ id: 1, name: "Genre" })];
    await openKindsPage();

    const dialog = await openDeleteDialog(user, "Genre");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("No kinds yet")).toBeDefined();
    const [header] = screen.getAllByRole("button", { name: "New kind" });
    await waitFor(() => expect(document.activeElement).toBe(header));
  });
});

describe("TagsPage「Kinds」のタブの行の操作", () => {
  it("タッチと狭い幅の「Actions」から名前の変更と削除の窓を開ける", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    await user.click(within(kindRow("Work")).getByRole("button", { name: "Actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
    expect(screen.getByRole("textbox", { name: 'New name for "Work"' })).toBeDefined();
    await user.keyboard("{Escape}");

    await user.click(within(kindRow("Place")).getByRole("button", { name: "Actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    expect(
      await screen.findByRole("alertdialog", { name: 'Delete the kind "Place"?' }),
    ).toBeDefined();
  });

  it("修飾キー付きで種類の名前を押したときはブラウザに任せ、画面の URL を変えない", async () => {
    await openKindsPage();
    const link = screen.getByRole("link", { name: 'Show the tags of the kind "Genre"' });
    let prevented: boolean | undefined;
    // React の処理の後で既定の動きを見て、jsdom の移動は止める。
    const record = (event: Event) => {
      prevented = event.defaultPrevented;
      event.preventDefault();
    };
    document.addEventListener("click", record);
    try {
      act(() => {
        link.dispatchEvent(
          new MouseEvent("click", { bubbles: true, cancelable: true, ctrlKey: true }),
        );
      });
    } finally {
      document.removeEventListener("click", record);
    }

    expect(prevented).toBe(false);
    expect(new URLSearchParams(probe.search).get("tab")).toBe("kinds");
  });

  it("選択したまま戻るで「Tags」を離れると選択を外し、進むで戻っても選択は無い", async () => {
    const user = userEvent.setup();
    await openKindsPage();
    await user.click(tagsTab());
    const row = (await screen.findByTitle("Anime")).closest<HTMLElement>(
      "[data-tag-id]",
    )!;
    await user.click(within(row).getByRole("checkbox"));
    expect(screen.getByRole("region", { name: "Selected tags" })).toBeDefined();

    await historyBack();
    await screen.findByRole("table", { name: "Kinds" });
    await historyForward();
    await waitFor(() => expect(tagsTab().getAttribute("aria-selected")).toBe("true"));
    await screen.findByTitle("Anime");
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
  });
});
