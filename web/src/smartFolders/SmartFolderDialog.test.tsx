import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import SmartFolderDialog, { type SmartFolderDialogTarget } from "./SmartFolderDialog";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function failure(status: number, code: string): Response {
  return json({ code, message: code }, status);
}

const folders = [
  { id: 1, name: "猫" },
  { id: 2, parentId: 1, name: "未視聴" },
  { id: 4, name: "犬" },
];

const fetchMock = vi.fn<typeof fetch>();
let writes: { method: string; path: string; body: unknown }[] = [];
let writeAnswer: () => Response = () => json({ id: 9, name: "保存" });
let listAnswer: () => Response = () => json({ smartFolders: folders });

function install() {
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/tags") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/tag-kinds") return Promise.resolve(json({ items: [] }));
    if (url.pathname === "/api/smart-folders" && method === "GET") {
      return Promise.resolve(listAnswer());
    }
    if (url.pathname.startsWith("/api/smart-folders")) {
      writes.push({ method, path: url.pathname, body: JSON.parse(String(init?.body)) });
      return Promise.resolve(writeAnswer());
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
}

function listRequests(): number {
  return fetchMock.mock.calls.filter(
    ([input, init]) =>
      new URL(String(input), "http://localhost").pathname === "/api/smart-folders" &&
      (init?.method ?? "GET") === "GET",
  ).length;
}

const onClose = vi.fn();
const onDone = vi.fn();
const onGone = vi.fn();

function renderDialog(target: SmartFolderDialogTarget, withGone = true) {
  render(
    <TooltipProvider>
      <SmartFolderDialog
        target={target}
        onClose={onClose}
        onDone={onDone}
        onGone={withGone ? onGone : undefined}
      />
    </TooltipProvider>,
  );
  return screen.getByRole("dialog");
}

function editTarget(rule: SmartFolderDialogTarget["rule"]): SmartFolderDialogTarget {
  return {
    kind: "edit",
    folder: { id: 2, name: "未視聴", parentId: 1 },
    rule,
    sort: "addedDesc",
    missing: [],
  };
}

const favorites: SmartFolderDialogTarget = {
  kind: "create",
  rule: { favorite: true },
  sort: "addedDesc",
};

function button(dialog: HTMLElement, name: string): HTMLButtonElement {
  return within(dialog).getByRole("button", { name });
}

describe("SmartFolderDialog", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    writes = [];
    writeAnswer = () => json({ id: 9, name: "保存" });
    listAnswer = () => json({ smartFolders: folders });
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

  describe("検索語は送る形に整えてから確かめる", () => {
    it("空白だけの検索語は条件にならず、作る窓は送らずに条件の誤りを出す", async () => {
      const user = userEvent.setup();
      const dialog = renderDialog({ kind: "create", rule: {}, sort: "addedDesc" });
      await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "空");
      await user.type(within(dialog).getByRole("searchbox", { name: "Search" }), "   ");
      await user.click(button(dialog, "Create"));

      expect(within(dialog).getByText("Add at least one condition.")).toBeDefined();
      expect(writes).toHaveLength(0);
    });

    it("直す窓で検索語に空白を足しただけでは Save を押せず、何も送らない", async () => {
      const user = userEvent.setup();
      const dialog = renderDialog(editTarget({ q: "cat" }));
      const search = within(dialog).getByRole("searchbox", { name: "Search" });

      await user.type(search, "  ");
      expect(button(dialog, "Save").disabled).toBe(true);
      await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "{Enter}");
      expect(writes).toHaveLength(0);

      await user.clear(search);
      await user.type(search, "  cats ");
      expect(button(dialog, "Save").disabled).toBe(false);
      await user.click(button(dialog, "Save"));
      await waitFor(() => expect(writes).toHaveLength(1));
      expect(writes[0]).toEqual({
        method: "PATCH",
        path: "/api/smart-folders/2",
        body: { rule: { q: "cats" } },
      });
    });

    it("直す窓で検索語を空白だけにすると条件が無くなり、送らずに条件の誤りを出す", async () => {
      const user = userEvent.setup();
      const dialog = renderDialog(editTarget({ q: "cat" }));
      const search = within(dialog).getByRole("searchbox", { name: "Search" });
      await user.clear(search);
      await user.type(search, " ");
      await user.click(button(dialog, "Save"));

      expect(within(dialog).getByText("Add at least one condition.")).toBeDefined();
      expect(writes).toHaveLength(0);
    });
  });

  it("元から条件の無いフォルダは、条件の無いまま名前を直せる", async () => {
    const user = userEvent.setup();
    const dialog = renderDialog(editTarget({}));
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "!");
    await user.click(button(dialog, "Save"));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.body).toEqual({ name: "未視聴!" });
    expect(onDone).toHaveBeenCalledWith({ id: 9, name: "保存" });
  });

  it("親を外すと parentId に null を送る", async () => {
    const user = userEvent.setup();
    const dialog = renderDialog(editTarget({ favorite: true }));
    await waitFor(() =>
      expect(within(dialog).getByRole("combobox", { name: "Parent" }).textContent).toBe(
        "猫",
      ),
    );
    await user.click(within(dialog).getByRole("combobox", { name: "Parent" }));
    await user.click(await screen.findByRole("option", { name: "None (top level)" }));
    await user.click(button(dialog, "Save"));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.body).toEqual({ parentId: null });
  });

  it("並べ順だけを変えると sort だけを送る", async () => {
    const user = userEvent.setup();
    const dialog = renderDialog(editTarget({ favorite: true }));
    await user.click(within(dialog).getByRole("combobox", { name: "Sort by" }));
    await user.click(await screen.findByRole("option", { name: "Title, ascending" }));
    await user.click(button(dialog, "Save"));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.body).toEqual({ sort: "titleAsc" });
  });

  describe("送った結果の失敗", () => {
    async function createWith(answer: () => Response) {
      writeAnswer = answer;
      const user = userEvent.setup();
      const dialog = renderDialog(favorites);
      await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "名前");
      await user.click(button(dialog, "Create"));
      await waitFor(() => expect(writes).toHaveLength(1));
      return dialog;
    }

    it("invalid_smart_rule は条件の欄の誤りにする", async () => {
      const dialog = await createWith(() => failure(400, "invalid_smart_rule"));
      expect(
        await within(dialog).findByText("A condition is out of range."),
      ).toBeDefined();
    });

    it("ほかの 400 は名前の誤りにし、名前の欄へフォーカスを移す", async () => {
      const dialog = await createWith(() => failure(400, "invalid_name"));
      expect(
        await within(dialog).findByText("Use a name of 1 to 100 characters."),
      ).toBeDefined();
      expect(document.activeElement).toBe(
        within(dialog).getByRole("textbox", { name: "Name" }),
      );
    });

    it("無い親（404 smart_folder_not_found）は親の欄の誤りにし、親の一覧を読み直す", async () => {
      const dialog = await createWith(() => failure(404, "smart_folder_not_found"));
      expect(
        await within(dialog).findByText("That smart folder no longer exists."),
      ).toBeDefined();
      await waitFor(() => expect(listRequests()).toBe(2));
    });

    it("欄の無い失敗は窓の中の Alert に出し、窓は開いたまま", async () => {
      const dialog = await createWith(() => failure(500, "internal"));
      expect((await within(dialog).findByRole("alert")).textContent).toContain(
        "Couldn't create the smart folder:",
      );
      expect(onClose).not.toHaveBeenCalled();
      expect(onDone).not.toHaveBeenCalled();
    });

    it("届かなかった（通信の失敗）ときも窓の中の Alert に出す", async () => {
      const dialog = await createWith(() => {
        throw new TypeError("Failed to fetch");
      });
      expect((await within(dialog).findByRole("alert")).textContent).toContain(
        "Couldn't create the smart folder:",
      );
    });

    it("直す窓の欄の無い失敗は保存の失敗として出す", async () => {
      writeAnswer = () => failure(500, "internal");
      const user = userEvent.setup();
      const dialog = renderDialog(editTarget({ favorite: true }));
      await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "!");
      await user.click(button(dialog, "Save"));
      expect((await within(dialog).findByRole("alert")).textContent).toContain(
        "Couldn't save the smart folder:",
      );
    });

    it("直す窓でフォルダ自身が無い（404 not_found）ときは onGone を呼ぶ", async () => {
      writeAnswer = () => failure(404, "not_found");
      const user = userEvent.setup();
      const dialog = renderDialog(editTarget({ favorite: true }));
      await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "!");
      await user.click(button(dialog, "Save"));
      await waitFor(() => expect(onGone).toHaveBeenCalled());
    });

    it("onGone の無い直す窓では、フォルダ自身が無い失敗も Alert に出す", async () => {
      writeAnswer = () => failure(404, "not_found");
      const user = userEvent.setup();
      const dialog = renderDialog(editTarget({ favorite: true }), false);
      await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "!");
      await user.click(button(dialog, "Save"));
      expect((await within(dialog).findByRole("alert")).textContent).toContain(
        "Couldn't save the smart folder:",
      );
    });
  });

  it("親の一覧を読めなければ親の欄にその理由を出す", async () => {
    listAnswer = () => failure(500, "internal");
    const dialog = renderDialog(favorites);
    expect(
      await within(dialog).findByText(/Couldn't load the smart folders:/),
    ).toBeDefined();
  });

  it("新しい子の窓は、親の一覧が届く前も親を選んだ形で出す", async () => {
    listAnswer = () => failure(500, "internal");
    const dialog = renderDialog({
      kind: "create",
      rule: {},
      sort: "addedDesc",
      parent: { id: 1, name: "猫" },
    });
    expect(within(dialog).getByRole("combobox", { name: "Parent" }).textContent).toBe(
      "猫",
    );
    expect(
      screen.getByRole("dialog", { name: 'New smart folder in "猫"' }),
    ).toBeDefined();
  });

  it("ランダムを置き換えたことは並べ順を変えるまで添える", async () => {
    const user = userEvent.setup();
    const dialog = renderDialog({ ...favorites, randomReplaced: true });
    const hint = "Random can't be saved; the folder is sorted by date added";
    expect(within(dialog).getByText(hint)).toBeDefined();

    await user.click(within(dialog).getByRole("combobox", { name: "Sort by" }));
    await user.click(await screen.findByRole("option", { name: "Title, ascending" }));
    expect(within(dialog).queryByText(hint)).toBeNull();
  });

  it("Cancel で閉じる", async () => {
    const user = userEvent.setup();
    const dialog = renderDialog(favorites);
    await user.click(button(dialog, "Cancel"));
    expect(onClose).toHaveBeenCalled();
  });
});
