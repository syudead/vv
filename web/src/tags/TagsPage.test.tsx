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
  /**
   * true にすると、以後の GET /api/tags をすべて 500 で失敗させ続ける
   * （自分では戻らない）。バックグラウンドの取り直し（`afterTagChanged` の
   * `refreshTags`）が、確かめたいその場の反映を上書きしてしまわないように
   * するためのもの。
   */
  failAllGets: false,
  /** true にすると、次の PATCH /api/tags/{id} を tag_name_taken 以外の理由で失敗させる（N2）。 */
  failNextPatch: false,
  /** true にすると、次の POST /api/tags/{id}/merge を一般の理由で失敗させる（N4）。 */
  failNextMerge: false,
  /** true にすると、次の POST /api/tags/{id}/synonyms を一般の理由で失敗させる（N4）。 */
  failNextSynonymsPost: false,
  /**
   * true にすると、次の POST /api/tags/{id}/synonyms を、実際の所有者に
   * かかわらず tag_merge_required で1回だけ失敗させる（自分で消費して false
   * に戻る）。「確認をとる時点ではもう誰も持っていない名前」を、タイミングを
   * 作らずに再現するための仕掛け（N6b）。
   */
  forceMergeRequiredOnce: false,
};

/** holdNextMutation を true にした次の POST・PATCH は、release() を呼ぶまで応答しない（B3）。 */
let holdNextMutation = false;
let release: (() => void) | null = null;

/**
 * holdSynonymDeletes を true にした間の DELETE /api/tags/{id}/synonyms は、
 * それぞれの release を呼ぶまで応答しない。並行する複数の解除を、任意の
 * 順で・同じタイミングで解決させて確かめる（N5・並行する解除）。
 */
let holdSynonymDeletes = false;
const synonymDeleteReleases: (() => void)[] = [];

function install() {
  const fetchMock = vi.fn<typeof fetch>();
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    const method = init?.method ?? "GET";

    if (path === "/api/tags" && method === "GET") {
      server.getCalls += 1;
      if (server.failAllGets || server.failNextGet) {
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

    const mergeMatch = /^\/api\/tags\/(\d+)\/merge$/.exec(path);
    if (mergeMatch && method === "POST") {
      const id = Number(mergeMatch[1]);
      const body = JSON.parse(String(init?.body)) as { sourceId: number };
      const target = server.tags.find((t) => t.id === id);
      const source = server.tags.find((t) => t.id === body.sourceId);
      if (target === undefined || source === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "タグはもうありません" }, 404),
        );
      }
      if (server.failNextMerge) {
        server.failNextMerge = false;
        return maybeHold(() =>
          jsonResponse({ code: "internal", message: "統合できませんでした" }, 500),
        );
      }
      return maybeHold(() => {
        target.synonyms = [...target.synonyms, source.name, ...source.synonyms];
        target.videoCount += source.videoCount;
        server.tags = server.tags.filter((t) => t.id !== source.id);
        return jsonResponse(target);
      });
    }

    const synonymsMatch = /^\/api\/tags\/(\d+)\/synonyms$/.exec(path);
    if (synonymsMatch && method === "POST") {
      const id = Number(synonymsMatch[1]);
      const target = server.tags.find((t) => t.id === id);
      if (target === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "タグはもうありません" }, 404),
        );
      }
      if (server.failNextSynonymsPost) {
        server.failNextSynonymsPost = false;
        return maybeHold(() =>
          jsonResponse({ code: "internal", message: "追加できませんでした" }, 500),
        );
      }
      if (server.forceMergeRequiredOnce) {
        server.forceMergeRequiredOnce = false;
        return maybeHold(() =>
          jsonResponse(
            { code: "tag_merge_required", message: "統合の確認が必要です" },
            409,
          ),
        );
      }
      const body = JSON.parse(String(init?.body)) as {
        name: string;
        mergeTagId?: number;
      };
      if (target.synonyms.includes(body.name)) {
        // 既にこのタグのシノニムである名前の登録は、何も変えずに 200 を返す。
        return maybeHold(() => jsonResponse(target));
      }
      if (target.name === body.name) {
        return maybeHold(() =>
          jsonResponse(
            {
              code: "tag_name_taken",
              message: `「${body.name}」は既にこのタグの名前です`,
            },
            409,
          ),
        );
      }
      const ownedBy = server.tags.find(
        (t) => t.id !== id && (t.name === body.name || t.synonyms.includes(body.name)),
      );
      if (ownedBy !== undefined) {
        if (ownedBy.name === body.name) {
          // 別のタグの元の名前。承諾（mergeTagId が一致）していなければ
          // tag_merge_required。
          if (body.mergeTagId !== ownedBy.id) {
            return maybeHold(() =>
              jsonResponse(
                { code: "tag_merge_required", message: "統合の確認が必要です" },
                409,
              ),
            );
          }
          return maybeHold(() => {
            target.synonyms = [...target.synonyms, ownedBy.name, ...ownedBy.synonyms];
            server.tags = server.tags.filter((t) => t.id !== ownedBy.id);
            target.synonyms = [...target.synonyms, body.name].filter(
              (name, index, all) => all.indexOf(name) === index,
            );
            return jsonResponse(target);
          });
        }
        // 別のタグの既存のシノニムとの衝突。
        return maybeHold(() =>
          jsonResponse(
            {
              code: "tag_name_taken",
              message: `「${body.name}」は「${ownedBy.name}」のシノニムです`,
            },
            409,
          ),
        );
      }
      return maybeHold(() => {
        target.synonyms = [...target.synonyms, body.name];
        return jsonResponse(target);
      });
    }

    if (synonymsMatch && method === "DELETE") {
      const id = Number(synonymsMatch[1]);
      const target = server.tags.find((t) => t.id === id);
      if (target === undefined) {
        return Promise.resolve(
          jsonResponse({ code: "tag_not_found", message: "タグはもうありません" }, 404),
        );
      }
      const name = url.searchParams.get("name") ?? "";
      const respond = () => {
        target.synonyms = target.synonyms.filter((s) => s !== name);
        return jsonResponse(null, 204);
      };
      if (!holdSynonymDeletes) return Promise.resolve(respond());
      return new Promise((resolve) => {
        synonymDeleteReleases.push(() => resolve(respond()));
      });
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
  server.failAllGets = false;
  server.failNextPatch = false;
  server.failNextMerge = false;
  server.failNextSynonymsPost = false;
  server.forceMergeRequiredOnce = false;
  holdNextMutation = false;
  release = null;
  holdSynonymDeletes = false;
  synonymDeleteReleases.length = 0;
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

  it("改名中に検索でその行が一致しなくなっても行は消えず、打っている途中の名前が残る（Devinの指摘1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    await user.clear(input);
    await user.type(input, "捨てない下書き");

    // 「旅行」はもう検索に一致しない条件へ変える。
    const search = screen.getByRole("searchbox", { name: "タグを検索" });
    await user.type(search, "Anime");

    // 行は消えず、打っている途中の名前もそのまま残る。
    const stillInput = screen.getByRole("textbox", { name: "「旅行」の新しい名前" });
    expect(stillInput).toBe(input);
    expect((stillInput as HTMLInputElement).value).toBe("捨てない下書き");
    // 件数の行は実際の一致件数（Anime の1件）のままで、ピン留めした分は数えない。
    expect(screen.getByText("1 / 3 個のタグ")).toBeDefined();
  });

  it("改名の送信中はEscで閉じない。閉じたあとに応答が届いて状態を書き換えることも無い（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    await user.clear(input);
    await user.type(input, "旅行2024");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    // 送信中の Esc は無視される。
    await user.keyboard("{Escape}");
    expect(screen.getByRole("textbox", { name: "「旅行」の新しい名前" })).toBeDefined();

    release?.();
    expect(await screen.findByTitle("旅行2024")).toBeDefined();
  });

  it("作成の送信中はEscで閉じず、キャンセルのボタンもdisabledのまま（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "新しいタグ" }));
    const input = screen.getByRole("textbox", { name: "新しいタグの名前" });
    await user.type(input, "Banana");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    expect(
      screen.getByRole("button", { name: "キャンセル" }).hasAttribute("disabled"),
    ).toBe(true);
    await user.keyboard("{Escape}");
    expect(screen.getByRole("textbox", { name: "新しいタグの名前" })).toBeDefined();

    release?.();
    expect(await screen.findByTitle("Banana")).toBeDefined();
  });

  it("改名の送信中はほかの行の改名も「新しいタグ」も始められない。応答が届けば再び始められる（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    await user.clear(input);
    await user.type(input, "旅行2024");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    const animeRow = screen.getByTitle("Anime").closest("div")!.parentElement!;
    const animeRenameButton = within(animeRow).getByRole("button", { name: "改名" });
    expect(animeRenameButton.hasAttribute("disabled")).toBe(true);
    await user.click(animeRenameButton);
    expect(screen.queryByRole("textbox", { name: "「Anime」の新しい名前" })).toBeNull();

    expect(
      screen.getByRole("button", { name: "新しいタグ" }).hasAttribute("disabled"),
    ).toBe(true);

    release?.();
    // 「旅行」の改名が終わって初めて、Anime の改名を始められる。応答が
    // すでに終わった「旅行」の改名を巻き戻すことは無い。
    expect(await screen.findByTitle("旅行2024")).toBeDefined();
    await user.click(within(animeRow).getByRole("button", { name: "改名" }));
    expect(
      await screen.findByRole("textbox", { name: "「Anime」の新しい名前" }),
    ).toBeDefined();
  });

  it("作成の失敗の表示は、入力を打ち直すと消える（Devinの指摘3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "新しいタグ" }));
    const input = screen.getByRole("textbox", { name: "新しいタグの名前" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("「旅行」は既に使われています")).toBeDefined();

    await user.type(input, "2024");
    expect(screen.queryByText("「旅行」は既に使われています")).toBeNull();
  });

  it("改名の失敗の表示は、入力を打ち直すと消える（Devinの指摘3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "改名" }));
    const input = await screen.findByRole("textbox", { name: "「旅行」の新しい名前" });
    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("「アニメ」は「Anime」のシノニムです")).toBeDefined();

    await user.type(input, "2024");
    expect(screen.queryByText("「アニメ」は「Anime」のシノニムです")).toBeNull();
  });
});

describe("TagsPage 統合", () => {
  it("「その他の操作」の先頭に「別のタグへ統合…」、区切り線を挟んで「削除…」が並ぶ（統合は作成・改名より目立たない）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    const menu = await screen.findByRole("menu");
    const items = within(menu).getAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual(["別のタグへ統合…", "削除…"]);
  });

  it("統合すると統合元が一覧から消え統合先のシノニムに並び、フォーカスが統合先の名前へ移る（受け入れ条件12）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "旅行", videoCount: 5 }),
      tag({ id: 2, name: "Anime", synonyms: ["アニメ"], videoCount: 3 }),
    ];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    const dialog = await screen.findByRole("dialog", { name: "「旅行」を統合" });

    const combo = within(dialog).getByRole("combobox", { name: "統合先のタグ" });
    const mergeButton = within(dialog).getByRole("button", { name: "統合する" });
    expect(mergeButton.hasAttribute("disabled")).toBe(true);

    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    expect(
      within(dialog).getByText(
        "「旅行」が付いた 5 本の動画に「Anime」が付きます。「旅行」とそのシノニムは「Anime」のシノニムになり、「旅行」はタグの一覧から消えます。この操作は取り消せません。",
      ),
    ).toBeDefined();
    expect(mergeButton.hasAttribute("disabled")).toBe(false);

    await user.click(mergeButton);

    expect(await screen.findByText("統合しました")).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.getByText("シノニム: アニメ · 旅行")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Animeで絞り込んだライブラリを開く" }),
      ),
    );
  });

  it("統合先の候補は統合元を除き、作成の行を持たない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    const dialog = await screen.findByRole("dialog", { name: "「旅行」を統合" });
    const combo = within(dialog).getByRole("combobox", { name: "統合先のタグ" });
    await user.click(combo);

    expect(within(dialog).queryByRole("option", { name: /旅行/ })).toBeNull();
    await user.type(combo, "存在しない語");
    expect(within(dialog).queryByText(/を作成/)).toBeNull();
  });

  it("統合の確認でEscを押すと何も変わらず、フォーカスがその行の「その他の操作」へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    const menuButton = within(row).getByRole("button", { name: "その他の操作" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    await screen.findByRole("dialog", { name: "「旅行」を統合" });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTitle("旅行")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("統合先が別のタブで消えていると、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    const dialog = await screen.findByRole("dialog", { name: "「旅行」を統合" });
    const combo = within(dialog).getByRole("combobox", { name: "統合先のタグ" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.tags = server.tags.filter((t) => t.name !== "Anime");

    await user.click(within(dialog).getByRole("button", { name: "統合する" }));

    expect(
      await screen.findByText("このタグはもう無いため、一覧を取り直しました"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合先の候補が開いているときのEscは一覧だけを閉じ、窓は閉じない（B1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    const dialog = await screen.findByRole("dialog", { name: "「旅行」を統合" });

    const combo = within(dialog).getByRole("combobox", { name: "統合先のタグ" });
    await user.click(combo);
    await user.type(combo, "A");
    expect(combo.getAttribute("aria-expanded")).toBe("true");

    // 一覧が開いているときの1回目のEscは、一覧だけを閉じる。窓は開いたまま。
    await user.keyboard("{Escape}");
    expect(combo.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByRole("dialog", { name: "「旅行」を統合" })).toBeDefined();

    // 一覧が閉じている状態での2回目のEscで、窓が閉じる。
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("IME変換中のEscは窓を閉じない（B2）", async () => {
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const user = userEvent.setup();
    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    const dialog = await screen.findByRole("dialog", { name: "「旅行」を統合" });
    const cancelButton = within(dialog).getByRole("button", { name: "キャンセル" });
    cancelButton.focus();

    fireEvent.keyDown(cancelButton, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("dialog", { name: "「旅行」を統合" })).toBeDefined();

    fireEvent.keyDown(cancelButton, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合の要求が届く間はEscや×で閉じない（N3）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    const dialog = await screen.findByRole("dialog", { name: "「旅行」を統合" });
    const combo = within(dialog).getByRole("combobox", { name: "統合先のタグ" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    holdNextMutation = true;
    await user.click(within(dialog).getByRole("button", { name: "統合する" }));
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "統合する" }).hasAttribute("disabled"),
      ).toBe(true),
    );

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "「旅行」を統合" })).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "閉じる" }));
    expect(screen.getByRole("dialog", { name: "「旅行」を統合" })).toBeDefined();

    release?.();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合が失敗すると、フォーカスが「統合する」へ戻る（N4）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "その他の操作" }));
    await user.click(await screen.findByRole("menuitem", { name: "別のタグへ統合…" }));
    const dialog = await screen.findByRole("dialog", { name: "「旅行」を統合" });
    const combo = within(dialog).getByRole("combobox", { name: "統合先のタグ" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.failNextMerge = true;
    const mergeButton = within(dialog).getByRole("button", { name: "統合する" });
    await user.click(mergeButton);

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));
  });
});

describe("TagsPage シノニム", () => {
  it("今のシノニムをChipに出し、×で確認なしにその場で解除できる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });

    expect(within(dialog).getByText("アニメ")).toBeDefined();
    await user.click(
      within(dialog).getByRole("button", { name: "シノニム「アニメ」を解除" }),
    );

    await waitFor(() => expect(within(dialog).queryByText("アニメ")).toBeNull());
    await waitFor(() => expect(screen.queryByText("シノニム: アニメ")).toBeNull());
  });

  it("シノニムを追加すると窓の中の一覧に並び、入力が空に戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Drama」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "ドラマ");
    await user.keyboard("{Enter}");

    expect(await within(dialog).findByText("ドラマ")).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("");
  });

  it("別のタグのシノニムと同じ名前の登録で、そのタグの名前が画面に出る（シノニム名の衝突）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Drama」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText("「アニメ」は「Anime」のシノニムです"),
    ).toBeDefined();
  });

  it("既存のタグの名前をシノニムとして登録しようとすると統合の確認が出て、承諾すると統合される（受け入れ条件17）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        "「anime」は 10 本の動画に付いているタグです。「Anime」に統合すると、その 10 本に「Anime」が付き、「anime」は「Anime」のシノニムになります。「anime」はタグの一覧から消えます。",
      ),
    ).toBeDefined();
    const backButton = within(dialog).getByRole("button", { name: "戻る" });
    expect(document.activeElement).toBe(backButton);

    await user.click(within(dialog).getByRole("button", { name: "統合する" }));

    await waitFor(() =>
      expect(within(dialog).queryByText(/本の動画に付いているタグです/)).toBeNull(),
    );
    expect(within(dialog).getByText("anime")).toBeDefined();
    expect(server.tags.some((t) => t.name === "anime")).toBe(false);
  });

  it("シノニムが無いタグとの統合の確認では「とそのシノニム」を省く", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", synonyms: ["アニメーション"], videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        "「anime」は 10 本の動画に付いているタグです。「Anime」に統合すると、その 10 本に「Anime」が付き、「anime」とそのシノニムは「Anime」のシノニムになります。「anime」はタグの一覧から消えます。",
      ),
    ).toBeDefined();
  });

  it("シノニム登録に伴う統合の確認で「戻る」を押すと何も変えず入力へ戻り、文字が残る（取り消すと何も変わらない）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/本の動画に付いているタグです/);

    await user.click(within(dialog).getByRole("button", { name: "戻る" }));

    expect(within(dialog).queryByText(/本の動画に付いているタグです/)).toBeNull();
    // 「戻る」で確認のビューから入力のビューに戻ると、入力は作り直される
    // （確認のビューには入力が無い）ので、あらためて取得する。
    const inputAfterBack = within(dialog).getByRole("textbox", {
      name: "シノニムを追加",
    }) as HTMLInputElement;
    expect(inputAfterBack.value).toBe("anime");
    expect(document.activeElement).toBe(inputAfterBack);
    expect(server.tags.some((t) => t.id === 2 && t.name === "anime")).toBe(true);
    expect(server.tags.find((t) => t.id === 1)?.synonyms).toEqual([]);
  });

  it("確認の後に別のタブでその名前が別のタグへ移っていると、統合されずに確認がやり直しになる", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/10 本の動画に付いているタグです/);

    // 確認を出したあと、別のタブで id2 を改名し、新しく「anime」（id3）を作る。
    server.tags[1]!.name = "anime-old";
    server.tags.push(tag({ id: 3, name: "anime", videoCount: 20 }));

    await user.click(within(dialog).getByRole("button", { name: "統合する" }));

    expect(
      await within(dialog).findByText(/20 本の動画に付いているタグです/),
    ).toBeDefined();
    expect(server.tags.some((t) => t.id === 2 && t.name === "anime-old")).toBe(true);
  });

  it("シノニムの窓でEscを押すと閉じ、フォーカスがその行の「シノニム」へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    const synonymsButton = within(row).getByRole("button", { name: "シノニム" });
    await user.click(synonymsButton);
    await screen.findByRole("dialog", { name: "「Anime」のシノニム" });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(synonymsButton));
  });

  it("シノニムの追加の失敗は、入力を打ち直すと消える（N1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Drama」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");
    await within(dialog).findByText("「アニメ」は「Anime」のシノニムです");

    await user.type(input, "2");

    expect(within(dialog).queryByText("「アニメ」は「Anime」のシノニムです")).toBeNull();
  });

  it("シノニムを解除すると、次のチップの×、無ければ前、1つも無ければ入力へフォーカスが移る（N2）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", synonyms: ["A", "B", "C"] })];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });

    // 中間（B）を解除すると、次（C）の×へ移る。
    await user.click(within(dialog).getByRole("button", { name: "シノニム「B」を解除" }));
    await waitFor(() => expect(within(dialog).queryByText("B")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("button", { name: "シノニム「C」を解除" }),
      ),
    );

    // 残りの最後（C）を解除すると、前（A）の×へ移る。
    await user.click(within(dialog).getByRole("button", { name: "シノニム「C」を解除" }));
    await waitFor(() => expect(within(dialog).queryByText("C")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("button", { name: "シノニム「A」を解除" }),
      ),
    );

    // 最後の1つ（A）を解除すると、入力へ移る。
    await user.click(within(dialog).getByRole("button", { name: "シノニム「A」を解除" }));
    await waitFor(() => expect(within(dialog).queryByText("A")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(input));
  });

  it("シノニム登録に伴う統合が失敗すると、フォーカスが「統合する」へ戻る（N4）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/本の動画に付いているタグです/);

    server.failNextSynonymsPost = true;
    const confirmButton = within(dialog).getByRole("button", { name: "統合する" });
    await user.click(confirmButton);

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(confirmButton));
  });

  it("シノニム登録に伴う統合が成功すると、統合元がその場で一覧から消える（バックグラウンドの取り直しを待たない）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");
    await screen.findByTitle("anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/本の動画に付いているタグです/);

    // バックグラウンドの取り直し（afterTagChanged の refreshTags）が失敗しても、
    // その場での反映（統合元を一覧から消す）は変わらないことを確かめる。
    server.failNextGet = true;

    await user.click(within(dialog).getByRole("button", { name: "統合する" }));
    await within(dialog).findByText("anime");
    await user.click(within(dialog).getByRole("button", { name: "閉じる" }));

    expect(screen.queryByTitle("anime")).toBeNull();
    expect(screen.getByTitle("Anime")).toBeDefined();
  });

  it("素のシノニムの登録・解除では、ほかのタグを一覧から消さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Drama」のシノニム" });
    const input = within(dialog).getByRole("textbox", { name: "シノニムを追加" });
    await user.type(input, "ドラマ");
    await user.keyboard("{Enter}");
    await within(dialog).findByText("ドラマ");
    await user.click(within(dialog).getByRole("button", { name: "閉じる" }));

    expect(screen.getByTitle("旅行")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByTitle("Drama")).toBeDefined();
  });

  it("確認をとる時点で名前の持ち主がもう無ければ、1回だけ送り直して素のまま登録する", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", videoCount: 2 })];
    server.forceMergeRequiredOnce = true;
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });
    const input = within(dialog).getByRole("textbox", {
      name: "シノニムを追加",
    }) as HTMLInputElement;
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    // 確認には切り替わらず、素の登録として直接付く（送り直しが1回だけ
    // 許される。N6b）。
    expect(await within(dialog).findByText("anime")).toBeDefined();
    expect(within(dialog).queryByText(/本の動画に付いているタグです/)).toBeNull();
    expect(input.value).toBe("");
  });

  it("シノニムの追加が成功する応答の間に打ち直していたら、入力の文字を消さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Drama」のシノニム" });
    const input = within(dialog).getByRole("textbox", {
      name: "シノニムを追加",
    }) as HTMLInputElement;
    await user.type(input, "ドラマ");

    holdNextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    // 応答を待つ間に、別の名前へ打ち直す。
    await user.clear(input);
    await user.type(input, "別の下書き");

    release?.();

    await within(dialog).findByText("ドラマ");
    // 追加した「ドラマ」は付いたが、その後に打ち直した文字は消えない。
    expect(input.value).toBe("別の下書き");
  });

  it("並行して複数のシノニムを解除しても、互いの結果を巻き戻さない", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", synonyms: ["A", "B"] })];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest("div")!.parentElement!;
    await user.click(within(row).getByRole("button", { name: "シノニム" }));
    const dialog = await screen.findByRole("dialog", { name: "「Anime」のシノニム" });

    // バックグラウンドの取り直し（各解除の afterTagChanged による
    // refreshTags）がサーバーの正しい状態で上書きして、クライアント側の
    // 巻き戻りを覆い隠してしまわないようにする。ここで確かめたいのは、
    // その場（サーバーの応答を待たない側）の反映が正しいことである。
    server.failAllGets = true;
    holdSynonymDeletes = true;
    await user.click(within(dialog).getByRole("button", { name: "シノニム「A」を解除" }));
    await user.click(within(dialog).getByRole("button", { name: "シノニム「B」を解除" }));
    expect(synonymDeleteReleases).toHaveLength(2);

    // 両方の応答を、同じタイミングで（間に描画を挟まず）解決させる。片方の
    // 結果がもう片方の閉じ込めた古い一覧で上書きされると、どちらか一方が
    // 生き残ってしまう。
    synonymDeleteReleases[0]!();
    synonymDeleteReleases[1]!();

    await waitFor(() => {
      expect(within(dialog).queryByText("A")).toBeNull();
      expect(within(dialog).queryByText("B")).toBeNull();
    });
  });
});
