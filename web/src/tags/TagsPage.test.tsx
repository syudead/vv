import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Tag } from "../api/tags";
import { __resetTagsForTest } from "../api/tags";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";
import TagsPage from "./TagsPage";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return { synonyms: [], videoCount: 0, ...overrides };
}

/** server はタグの管理経路（GET・POST・PATCH・DELETE /api/tags*）を扱う偽のサーバーである。 */
const server = {
  tags: [] as Tag[],
  nextId: 100,
  getCalls: 0,
  /** true にすると、次の GET /api/tags を 500 で失敗させる（N6: 取り直しの失敗）。 */
  failNextGet: false,
  /** true にすると、次の PATCH /api/tags/{id} を tag_name_taken 以外の理由で失敗させる（N2）。 */
  failNextPatch: false,
};

/** holdNextMutation を true にした次の POST・PATCH は、release() を呼ぶまで応答しない（B3）。 */
let holdNextMutation = false;
let release: (() => void) | null = null;

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    const method = init?.method ?? "GET";

    if (path === "/api/tags" && method === "GET") {
      server.getCalls += 1;
      if (server.failNextGet) {
        server.failNextGet = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "失敗しました" }, 500),
        );
      }
      return Promise.resolve(jsonResponse({ items: server.tags }));
    }

    function maybeHold(respond: () => Response): Promise<Response> {
      if (!holdNextMutation) return Promise.resolve(respond());
      holdNextMutation = false;
      return new Promise((resolve) => {
        release = () => resolve(respond());
      });
    }

    if (path === "/api/tags" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { name: string };
      const conflict = server.tags.find(
        (t) => t.name === body.name || t.synonyms.includes(body.name),
      );
      if (conflict !== undefined) {
        return maybeHold(() =>
          jsonResponse(
            { code: "tag_name_taken", message: `「${body.name}」は既に使われています` },
            409,
          ),
        );
      }
      const created = tag({ id: server.nextId, name: body.name });
      server.nextId += 1;
      return maybeHold(() => {
        server.tags.push(created);
        return jsonResponse(created, 201);
      });
    }

    const patchMatch = /^\/api\/tags\/(\d+)$/.exec(path);
    if (patchMatch && method === "PATCH") {
      const id = Number(patchMatch[1]);
      const found = server.tags.find((t) => t.id === id);
      if (found === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "タグはもうありません" }, 404),
        );
      }
      if (server.failNextPatch) {
        server.failNextPatch = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "改名できませんでした" }, 500),
        );
      }
      const body = JSON.parse(String(init?.body)) as { name: string };
      const conflict = server.tags.find(
        (t) => t.id !== id && (t.name === body.name || t.synonyms.includes(body.name)),
      );
      if (conflict !== undefined) {
        return maybeHold(() =>
          jsonResponse(
            {
              code: "tag_name_taken",
              message: `「${body.name}」は「${conflict.name}」のシノニムです`,
            },
            409,
          ),
        );
      }
      return maybeHold(() => {
        found.name = body.name;
        return jsonResponse(found);
      });
    }

    if (patchMatch && method === "DELETE") {
      const id = Number(patchMatch[1]);
      const index = server.tags.findIndex((t) => t.id === id);
      if (index === -1) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "タグはもうありません" }, 404),
        );
      }
      server.tags.splice(index, 1);
      return Promise.resolve(jsonResponse(null, 204));
    }

    throw new Error(`想定しない要求: ${method} ${path}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <TooltipProvider>
        <ToastProvider>
          <TagsPage />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  __resetTagsForTest();
  server.tags = [
    tag({ id: 1, name: "旅行" }),
    tag({ id: 2, name: "Anime", synonyms: ["アニメ"], videoCount: 3 }),
    tag({ id: 3, name: "Drama" }),
  ];
  server.nextId = 100;
  server.getCalls = 0;
  server.failNextGet = false;
  server.failNextPatch = false;
  holdNextMutation = false;
  release = null;
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("TagsPage", () => {
  it("一覧を本数つきで出し、件数の行に総数が出る", async () => {
    install();
    renderPage();

    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByText("シノニム: アニメ")).toBeDefined();
    expect(screen.getByText("3 個のタグ")).toBeDefined();
    // 本数 0 のタグも出る。
    const dramaRow = screen.getByTitle("Drama").closest("div")!.parentElement!;
    expect(dramaRow.textContent).toContain("0 本");
  });

  it("検索は名前とシノニムのどちらでも、大文字小文字を区別せず絞り込む（受け入れ条件18）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "タグを検索" });
    await user.type(search, "アニ");
    expect(await screen.findByText("1 / 3 個のタグ")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.queryByTitle("Drama")).toBeNull();

    await user.clear(search);
    await user.type(search, "ani");
    expect(await screen.findByText("1 / 3 個のタグ")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();

    await user.clear(search);
    expect(await screen.findByText("3 個のタグ")).toBeDefined();
    expect(screen.getByTitle("旅行")).toBeDefined();
  });

  it("検索で一致が無いときは、タグが無い状態と別の表示になり、そこから入力を消せる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "タグを検索" });
    await user.type(search, "存在しない語");

    expect(
      await screen.findByText("「存在しない語」に一致するタグはありません"),
    ).toBeDefined();
    expect(screen.queryByText("タグはまだありません")).toBeNull();

    await user.click(screen.getByRole("button", { name: "検索をクリア" }));
    expect(document.activeElement).toBe(search);
    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  it("タグが1つも無いときは空の状態を出し、検索の入力はdisabledのまま", async () => {
    server.tags = [];
    install();
    renderPage();

    expect(await screen.findByText("タグはまだありません")).toBeDefined();
    expect(
      screen.getByRole("searchbox", { name: "タグを検索" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("新しいタグを作ると本数0で名前順の位置に入り、名前へフォーカスが移る（受け入れ条件9）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "新しいタグ" }));
    const input = screen.getByRole("textbox", { name: "新しいタグの名前" });
    expect(document.activeElement).toBe(input);
    await user.type(input, "Banana");
    await user.keyboard("{Enter}");

    const created = await screen.findByRole("link", {
      name: "Bananaで絞り込んだライブラリを開く",
    });
    await waitFor(() => expect(document.activeElement).toBe(created));
    expect(screen.getByText("4 個のタグ")).toBeDefined();
  });

  it("既存の名前と重なる作成は理由を出し、入力を残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "新しいタグ" }));
    const input = screen.getByRole("textbox", { name: "新しいタグの名前" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("「旅行」は既に使われています")).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("旅行");
  });

  it("改名すると新しい名前で表示され、既存名との重なりは理由を出す（受け入れ条件10）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));

    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    expect(document.activeElement).toBe(input);

    // 既存のシノニムと重なる改名は拒否され、理由が画面に出る。
    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");
    expect(await screen.findByText("「アニメ」は「Anime」のシノニムです")).toBeDefined();

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(await screen.findByTitle("旅行2024")).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
  });

  it("改名中にEscを押すと、何も変えずに戻り、フォーカスが「改名」へ戻る（N3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    const renameButton = within(row).getByRole("button", { name: "改名" });
    await user.click(renameButton);
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    await user.clear(input);
    await user.type(input, "捨てる名前");
    await user.keyboard("{Escape}");

    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect(screen.queryByTitle("捨てる名前")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(renameButton));
  });

  it("作成中にEscを押すと、何も作らずフォーカスが「新しいタグ」へ戻る（N3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const createButton = screen.getByRole("button", { name: "新しいタグ" });
    await user.click(createButton);
    const input = screen.getByRole("textbox", { name: "新しいタグの名前" });
    await user.type(input, "捨てる名前");
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("textbox", { name: "新しいタグの名前" })).toBeNull();
    expect(document.activeElement).toBe(createButton);
  });

  it("別のタブで消したタグを改名しようとすると、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });

    // 改名を試みる前に、別のタブで削除されたことにする。
    server.tags = server.tags.filter((t) => t.name !== "旅行");

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("このタグはもう無いため、一覧を取り直しました"),
    ).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.getByText("2 個のタグ")).toBeDefined();
  });

  it("削除の確認は外れる本数を示し、確定すると消えて次の行の改名へフォーカスが移る（受け入れ条件11）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "削除…" }));

    const dialog = await screen.findByRole("dialog", { name: "「Anime」を削除" });
    expect(
      within(dialog).getByText(
        "3 本の動画からこのタグが外れます。この操作は取り消せません。",
      ),
    ).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "削除する" }));

    await waitFor(() => expect(screen.queryByTitle("Anime")).toBeNull());
    expect(await screen.findByText("削除しました")).toBeDefined();
    // Anime の次は Drama（自然順で 旅行 < Anime < Drama）。
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(screen.getByTitle("Drama").closest("div")!.parentElement!).getByRole(
          "button",
          { name: "改名" },
        ),
      ),
    );
  });

  it("0本のタグの削除確認は「どの動画にも付いていません」と出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "削除…" }));

    const dialog = await screen.findByRole("dialog", { name: "「Drama」を削除" });
    expect(
      within(dialog).getByText("このタグはどの動画にも付いていません。"),
    ).toBeDefined();
  });

  it("削除の確認でEscを押すと何も変わらず、フォーカスがその行の「その他の操作」へ戻る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    const menuButton = within(row).getByRole("button", { name: "その他の操作" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "削除…" }));
    await screen.findByRole("dialog", { name: "「Drama」を削除" });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTitle("Drama")).toBeDefined();
    // その他の操作のメニュー項目（削除…）はメニューが閉じるともう無いので、
    // ModalFrame の「前のフォーカスへ戻す」だけには頼らず、その行の
    // 「その他の操作」へ明示的に戻す。
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("削除の確認の「キャンセル」を押しても、フォーカスがその行の「その他の操作」へ戻る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    const menuButton = within(row).getByRole("button", { name: "その他の操作" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "削除…" }));
    const dialog = await screen.findByRole("dialog", { name: "「Drama」を削除" });

    await user.click(within(dialog).getByRole("button", { name: "キャンセル" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("別のタブで消したタグの削除を試みると、もう無いことが伝わり次の行の改名へフォーカスが移る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "削除…" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」を削除" });

    // 確認を開いたあと、別のタブで先に削除されたことにする。
    server.tags = server.tags.filter((t) => t.name !== "Anime");

    await user.click(within(dialog).getByRole("button", { name: "削除する" }));

    expect(
      await screen.findByText("このタグはもう無いため、一覧を取り直しました"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    // Anime の次は Drama（自然順で 旅行 < Anime < Drama）。
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(screen.getByTitle("Drama").closest("div")!.parentElement!).getByRole(
          "button",
          { name: "改名" },
        ),
      ),
    );
  });

  it("キーボードだけで検索の入力に届く（本文で最初のTab）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("searchbox", { name: "タグを検索" }),
    );
  });

  it("読み込みに失敗すると再試行でき、成功すると一覧が出る", async () => {
    const fetchMock = vi.fn<typeof fetch>();
    let first = true;
    fetchMock.mockImplementation((input) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/tags" && first) {
        first = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "失敗しました" }, 500),
        );
      }
      return Promise.resolve(jsonResponse({ items: server.tags }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    expect(await screen.findByText("タグを取得できません")).toBeDefined();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "再試行" }));
    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  it("IME変換中のEnterとEscは、作成・改名・検索のどの入力でも無視される（B1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    // 作成の入力。
    await user.click(screen.getByRole("button", { name: "新しいタグ" }));
    const createInput = screen.getByRole("textbox", { name: "新しいタグの名前" });
    await user.type(createInput, "IME作成中");
    fireEvent.keyDown(createInput, { key: "Enter", keyCode: 229 });
    expect(screen.queryByTitle("IME作成中")).toBeNull();
    fireEvent.keyDown(createInput, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("textbox", { name: "新しいタグの名前" })).toBeDefined();
    fireEvent.keyDown(createInput, { key: "Enter" });
    expect(await screen.findByTitle("IME作成中")).toBeDefined();

    // 改名の入力。
    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const renameInput = await screen.findByRole("textbox", {
      name: "「旅行」の新しい名前",
    });
    await user.clear(renameInput);
    await user.type(renameInput, "旅行2024");
    fireEvent.keyDown(renameInput, { key: "Enter", keyCode: 229 });
    expect(screen.queryByTitle("旅行2024")).toBeNull();
    fireEvent.keyDown(renameInput, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("textbox", { name: "「旅行」の新しい名前" })).toBeDefined();
    fireEvent.keyDown(renameInput, { key: "Enter" });
    expect(await screen.findByTitle("旅行2024")).toBeDefined();

    // 検索の入力。
    const search = screen.getByRole("searchbox", { name: "タグを検索" });
    await user.type(search, "存在しない語");
    fireEvent.keyDown(search, { key: "Escape", keyCode: 229 });
    expect((search as HTMLInputElement).value).toBe("存在しない語");
    fireEvent.keyDown(search, { key: "Escape" });
    expect((search as HTMLInputElement).value).toBe("");
  });

  it("作成中はaria-busyだけを付けて入力をdisabledにせず、tag_name_taken後もフォーカスと値が残る（B3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "新しいタグ" }));
    const input = screen.getByRole("textbox", { name: "新しいタグの名前" });
    await user.type(input, "旅行");

    holdNextMutation = true;
    await user.keyboard("{Enter}");

    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));
    expect(input.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(input);

    release?.();

    expect(await screen.findByText("「旅行」は既に使われています")).toBeDefined();
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).value).toBe("旅行");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBeNull());
  });

  it("改名中はaria-busyだけを付けて入力をdisabledにせず、tag_name_taken後もフォーカスと値が残る（B3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    await user.clear(input);
    await user.type(input, "アニメ");

    holdNextMutation = true;
    await user.keyboard("{Enter}");

    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));
    expect(input.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(input);

    release?.();

    expect(await screen.findByText("「アニメ」は「Anime」のシノニムです")).toBeDefined();
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).value).toBe("アニメ");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBeNull());
  });

  it("改名中も本数・改名・その他の操作は出したままで、同じ行の要素のままになる（B4）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));

    const input = await screen.findByRole("textbox", { name: "「Anime」の新しい名前" });
    expect(input.closest("[data-tag-id]")).toBe(row);
    expect(row.textContent).toContain("3 本");
    expect(within(row).getByRole("button", { name: "改名" })).toBeDefined();
    expect(within(row).getByRole("button", { name: "その他の操作" })).toBeDefined();
  });

  it("tag_name_taken以外の改名の失敗はtext-sm・role=alertで、tag_name_takenはrole=alert無しで出す（N2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });

    server.failNextPatch = true;
    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("改名できませんでした");
    expect(alert.className).toContain("text-sm");

    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    const taken = await screen.findByText("「アニメ」は「Anime」のシノニムです");
    expect(taken.getAttribute("role")).toBeNull();
    expect(taken.className).toContain("text-xs");
  });

  it("空白だけの検索は絞り込み中として数えない（N5）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "タグを検索" });
    await user.type(search, "   ");

    expect(screen.getByText("3 個のタグ")).toBeDefined();
    expect(screen.queryByText(/\/ 3 個のタグ/)).toBeNull();
  });

  it("作成の直後に一覧を2回取り直さない（N6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    expect(server.getCalls).toBe(1);

    await user.click(screen.getByRole("button", { name: "新しいタグ" }));
    const input = screen.getByRole("textbox", { name: "新しいタグの名前" });
    await user.type(input, "Banana");
    await user.keyboard("{Enter}");
    await screen.findByTitle("Banana");

    // api/tags.ts の afterTagCreated によるバックグラウンドの1回だけが増える
    // （TagsPage 自身は、その結果を待たずに作った1件をその場で重ねるので、
    // もう1回 GET /api/tags を送らない）。
    await waitFor(() => expect(server.getCalls).toBe(2));
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(server.getCalls).toBe(2);
  });

  it("tag_not_foundの取り直しに失敗しても、一覧を空白にせず今の一覧を残す（N6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });

    server.tags = server.tags.filter((t) => t.name !== "旅行");
    server.failNextGet = true;

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("このタグはもう無いため、一覧を取り直しました"),
    ).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByTitle("Drama")).toBeDefined();
    expect(screen.queryByText("タグを取得できません")).toBeNull();
  });
});
