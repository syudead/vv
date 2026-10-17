import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SmartFolderSettings } from "../api/smartFolders";
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import SmartFolderMenu from "./SmartFolderMenu";

// スマートフォルダの「More」の Edit・New child・Delete と、条件の編集の窓
//

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** 猫 ─ 未視聴 ─ 最近、犬。 */
const folders = [
  { id: 1, name: "猫" },
  { id: 2, parentId: 1, name: "未視聴" },
  { id: 3, parentId: 2, name: "最近" },
  { id: 4, name: "犬" },
];

const fetchMock = vi.fn<typeof fetch>();
/** 書き込みの要求（メソッド、パス、本文）。 */
let writes: { method: string; path: string; body: unknown }[] = [];
/** PATCH の答え。 */
let patchAnswer: () => Response = () => json({ id: 2, name: "未視聴" });

function install() {
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (method !== "GET") {
      writes.push({
        method,
        path: url.pathname,
        body: init?.body === undefined ? undefined : JSON.parse(String(init.body)),
      });
    }
    if (url.pathname === "/api/tags") {
      return Promise.resolve(
        json({
          items: [
            { id: 1, name: "猫", synonyms: [], videoCount: 2 },
            { id: 2, name: "犬", synonyms: [], videoCount: 1 },
          ],
        }),
      );
    }
    if (url.pathname === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/smart-folders" && method === "GET") {
      return Promise.resolve(json({ smartFolders: folders }));
    }
    if (url.pathname === "/api/smart-folders" && method === "POST") {
      return Promise.resolve(json({ id: 5, parentId: 1, name: "新しい子" }, 201));
    }
    if (url.pathname.startsWith("/api/smart-folders/") && method === "PATCH") {
      return Promise.resolve(patchAnswer());
    }
    if (url.pathname.startsWith("/api/smart-folders/") && method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
}

const callbacks = {
  onSaved: vi.fn(),
  onCreatedChild: vi.fn(),
  onDeleted: vi.fn(),
  onGone: vi.fn(),
};

function renderMenu(
  folder: { id: number; name: string; parentId?: number },
  settings: SmartFolderSettings,
) {
  return render(
    <MemoryRouter>
      <TooltipProvider>
        <ToastProvider>
          <SmartFolderMenu folder={folder} settings={settings} {...callbacks} />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

const unwatchedCats: SmartFolderSettings = {
  rule: { watch: "unwatched", tags: [1] },
  sort: "addedDesc",
  missing: [],
};

async function openItem(name: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "More" }));
  await user.click(await screen.findByRole("menuitem", { name }));
  return user;
}

describe("SmartFolderMenu", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    writes = [];
    patchAnswer = () => json({ id: 2, name: "未視聴" });
    vi.stubGlobal("fetch", fetchMock);
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
    Element.prototype.scrollIntoView ??= () => undefined;
    install();
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("Edit はフォルダの条件で開き、変えた条件だけを送る", async () => {
    renderMenu({ id: 2, name: "未視聴", parentId: 1 }, unwatchedCats);
    const user = await openItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });

    expect(
      (within(dialog).getByRole("textbox", { name: "Name" }) as HTMLInputElement).value,
    ).toBe("未視聴");
    expect(
      within(dialog)
        .getByRole("radio", { name: "Unwatched" })
        .getAttribute("aria-checked"),
    ).toBe("true");
    expect(
      await within(dialog).findByRole("button", { name: "Remove the condition 猫" }),
    ).toBeDefined();
    await waitFor(() =>
      expect(within(dialog).getByRole("combobox", { name: "Parent" }).textContent).toBe(
        "猫",
      ),
    );
    // 何も変えていない間は送れない。
    const save = within(dialog).getByRole("button", { name: "Save" });
    expect((save as HTMLButtonElement).disabled).toBe(true);

    await user.click(within(dialog).getByRole("radio", { name: "Watched" }));
    await user.click(save);
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toEqual({
      method: "PATCH",
      path: "/api/smart-folders/2",
      body: { rule: { watch: "watched", tags: [1] } },
    });
    expect(await screen.findByText('Saved "未視聴"')).toBeDefined();
    expect(callbacks.onSaved).toHaveBeenCalledWith({ id: 2, name: "未視聴" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("直す窓でフォルダがもう無ければ（404 not_found）、窓を閉じて onGone を呼ぶ", async () => {
    patchAnswer = () => json({ code: "not_found", message: "gone" }, 404);
    renderMenu({ id: 2, name: "未視聴", parentId: 1 }, unwatchedCats);
    const user = await openItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });
    await user.click(within(dialog).getByRole("radio", { name: "Watched" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(callbacks.onGone).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(callbacks.onSaved).not.toHaveBeenCalled();
  });

  it("条件をすべて外すと送らずに条件の欄の誤りを出す", async () => {
    renderMenu({ id: 2, name: "未視聴", parentId: 1 }, unwatchedCats);
    const user = await openItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });

    await user.click(within(dialog).getByRole("radio", { name: "All" }));
    await user.click(
      await within(dialog).findByRole("button", { name: "Remove the condition 猫" }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByText("Add at least one condition.")).toBeDefined();
    expect(writes).toHaveLength(0);
  });

  it("範囲の外の条件は送れない", async () => {
    renderMenu(
      { id: 2, name: "未視聴", parentId: 1 },
      { rule: { minDuration: 600, maxDuration: 60 }, sort: "addedDesc", missing: [] },
    );
    const user = await openItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "2");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByText("A condition is out of range.")).toBeDefined();
    expect(writes).toHaveLength(0);
  });

  it("親の選択肢に自分と子孫は無く、自分の子の下へ移すと移せないと言う", async () => {
    renderMenu({ id: 1, name: "猫" }, unwatchedCats);
    const user = await openItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });

    await user.click(within(dialog).getByRole("combobox", { name: "Parent" }));
    await waitFor(() =>
      expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
        "None (top level)",
        "犬",
      ]),
    );
    await user.keyboard("{Escape}");

    // 別のタブで木が変わり、選んだ親が実は子孫だった（Edge Case）。
    patchAnswer = () =>
      json(
        {
          code: "smart_folder_cycle",
          message: "A smart folder can't be put inside itself or one of its children.",
        },
        409,
      );
    await user.click(within(dialog).getByRole("combobox", { name: "Parent" }));
    await user.click(await screen.findByRole("option", { name: "犬" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(
      await within(dialog).findByText(
        "A smart folder can't be put inside itself or inside one of its children.",
      ),
    ).toBeDefined();
    expect(writes[0]).toEqual({
      method: "PATCH",
      path: "/api/smart-folders/1",
      body: { parentId: 4 },
    });
    expect(screen.getByRole("dialog", { name: "Edit smart folder" })).toBeDefined();
  });

  it("もう無いタグの条件は警告のチップで出す", async () => {
    renderMenu(
      { id: 2, name: "未視聴" },
      { rule: { tags: [7] }, sort: "addedDesc", missing: [{ kind: "tag", id: 7 }] },
    );
    await openItem("Edit…");
    const dialog = await screen.findByRole("dialog", { name: "Edit smart folder" });
    expect(within(dialog).getByText("Tag: deleted")).toBeDefined();
    expect(
      within(dialog).getByRole("button", { name: "Remove the deleted condition" }),
    ).toBeDefined();
  });

  it("New child は親をこのフォルダにして作る", async () => {
    renderMenu({ id: 1, name: "猫" }, unwatchedCats);
    const user = await openItem("New child smart folder…");
    const dialog = await screen.findByRole("dialog", {
      name: 'New smart folder in "猫"',
    });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "新しい子");
    await user.click(within(dialog).getByRole("checkbox", { name: "Playable only" }));
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toEqual({
      method: "POST",
      path: "/api/smart-folders",
      body: {
        name: "新しい子",
        rule: { playable: true },
        sort: "addedDesc",
        parentId: 1,
      },
    });
    expect(await screen.findByText('Created "新しい子"')).toBeDefined();
    expect(callbacks.onCreatedChild).toHaveBeenCalled();
  });

  it("Delete は子孫の数を言い、確かめてから消す", async () => {
    renderMenu({ id: 1, name: "猫" }, unwatchedCats);
    const user = await openItem("Delete…");
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "猫"?' });
    expect(
      await within(dialog).findByText(
        /The 2 smart folders inside it are deleted with it\./,
      ),
    ).toBeDefined();
    expect(writes).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(callbacks.onDeleted).toHaveBeenCalledWith(true));
    expect(writes).toEqual([
      { method: "DELETE", path: "/api/smart-folders/1", body: undefined },
    ]);
  });
});
