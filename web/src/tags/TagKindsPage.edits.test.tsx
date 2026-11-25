import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import {
  historyBack,
  hold,
  install,
  kind,
  kindDeleteReleases,
  renderPage,
  server,
  setUpTagsPageServer,
  tag,
} from "../testing/tagsPage";

setUpTagsPageServer();

// Cover cancellation, failure and focus; TagKindsPage.test.tsx covers successful edits.

type User = ReturnType<typeof userEvent.setup>;

function kindRow(name: string): HTMLElement {
  return screen.getByTitle(name).closest<HTMLElement>("[data-kind-id]")!;
}

function menuButton(name: string): HTMLElement {
  return within(kindRow(name)).getByRole("button", {
    name: `More actions for the kind "${name}"`,
  });
}

async function openKindsPage() {
  install();
  renderPage({ url: "/tags/kinds" });
  await screen.findByTitle(server.tagKinds[0]!.name);
}

async function openDeleteDialog(user: User, name: string): Promise<HTMLElement> {
  await user.click(menuButton(name));
  await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
  return screen.findByRole("alertdialog", { name: `Delete the kind "${name}"?` });
}

describe("TagKindsPage 取り消しと失敗", () => {
  it("改名の入力は Esc で閉じ、何も送らず、その行の「More actions」へフォーカスを戻す", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    await user.click(menuButton("Work"));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
    await user.type(screen.getByRole("textbox", { name: 'New name for "Work"' }), "s");
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("textbox", { name: 'New name for "Work"' })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(menuButton("Work")));
    expect(server.tagKinds.find((item) => item.id === 2)?.name).toBe("Work");
  });

  it("削除の確認の窓の Cancel は何も消さず、その行の「More actions」へフォーカスを戻す", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    const dialog = await openDeleteDialog(user, "Place");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(menuButton("Place")));
    expect(server.tagKinds).toHaveLength(4);
  });

  it("削除の送信中は Esc で窓を閉じず、応答が届いてから閉じる", async () => {
    const user = userEvent.setup();
    await openKindsPage();

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
    expect(screen.queryByTitle("Place")).toBeNull();
  });

  it("削除の送信中にタグの一覧へ戻ると、削除が済んでから一覧を読み直し、種類なしで出す", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 9, name: "Robot", kind: { id: 1, name: "Genre", cardFirst: false } }),
    ];
    install();
    renderPage({ url: "/tags" });
    await screen.findByTitle("Robot");
    await user.click(screen.getByRole("link", { name: "Kinds" }));
    await screen.findByTitle("Genre");

    const dialog = await openDeleteDialog(user, "Genre");
    hold.kindDeletes = true;
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(kindDeleteReleases).toHaveLength(1));
    await historyBack();
    const row = (await screen.findByTitle("Robot")).closest<HTMLElement>(
      "[data-tag-id]",
    )!;
    expect(within(row).getByText("Genre")).toBeDefined();

    await act(async () => {
      kindDeleteReleases[0]!();
      await Promise.resolve();
    });
    await waitFor(() => {
      const current = screen.getByTitle("Robot").closest<HTMLElement>("[data-tag-id]")!;
      expect(within(current).queryByText("Genre")).toBeNull();
    });
  });

  it("削除が 404 以外で失敗したら窓の中に誤りを出し、窓と行を残す", async () => {
    const user = userEvent.setup();
    await openKindsPage();

    const dialog = await openDeleteDialog(user, "Place");
    server.failNextKindDelete = true;
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await within(dialog).findByText("Something went wrong on the server."),
    ).toBeDefined();
    expect(screen.getByRole("alertdialog")).toBe(dialog);
    // Dialogs hide background rows from role queries; use title instead.
    expect(screen.getByTitle("Place")).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(screen.queryByTitle("Place")).toBeNull();
  });

  it("最後の種類を消すと、フォーカスは「New kind」の欄へ移る", async () => {
    const user = userEvent.setup();
    server.tagKinds = [kind({ id: 1, name: "Genre" })];
    await openKindsPage();

    const dialog = await openDeleteDialog(user, "Genre");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("No kinds yet")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("textbox", { name: "New kind name" }),
      ),
    );
  });
});
