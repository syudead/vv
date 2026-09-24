import { render, screen, waitFor, within } from "@testing-library/react";
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
};

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    const method = init?.method ?? "GET";

    if (path === "/api/tags" && method === "GET") {
      return Promise.resolve(jsonResponse({ items: server.tags }));
    }

    if (path === "/api/tags" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { name: string };
      const conflict = server.tags.find(
        (t) => t.name === body.name || t.synonyms.includes(body.name),
      );
      if (conflict !== undefined) {
        return Promise.resolve(
          jsonResponse(
            { code: "tag_name_taken", message: `「${body.name}」は既に使われています` },
            409,
          ),
        );
      }
      const created = tag({ id: server.nextId, name: body.name });
      server.nextId += 1;
      server.tags.push(created);
      return Promise.resolve(jsonResponse(created, 201));
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
      const body = JSON.parse(String(init?.body)) as { name: string };
      const conflict = server.tags.find(
        (t) => t.id !== id && (t.name === body.name || t.synonyms.includes(body.name)),
      );
      if (conflict !== undefined) {
        return Promise.resolve(
          jsonResponse(
            {
              code: "tag_name_taken",
              message: `「${body.name}」は「${conflict.name}」のシノニムです`,
            },
            409,
          ),
        );
      }
      found.name = body.name;
      return Promise.resolve(jsonResponse(found));
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

  it("改名中にEscを押すと、何も変えずに戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    await user.clear(input);
    await user.type(input, "捨てる名前");
    await user.keyboard("{Escape}");

    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect(screen.queryByTitle("捨てる名前")).toBeNull();
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

  it("削除の確認でEscを押すと何も変わらない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "削除…" }));
    await screen.findByRole("dialog", { name: "「Drama」を削除" });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTitle("Drama")).toBeDefined();
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
});
