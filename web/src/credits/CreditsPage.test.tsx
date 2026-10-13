import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Credit, CreditKind } from "../api/credits";
import { foldForMatch } from "../lib/foldForMatch";
import { compareTagsForSort } from "../tags/tagPageRows";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import CreditsPage from "./CreditsPage";

// 人物・団体の管理画面のページの試験（specs/047-video-work-info/ui-design.md「People and
// organizations screens」、Issue #1028 の完了の条件）。偽のサーバーは `/api/people` と
// `/api/organizations` の 6 つの経路を持つ。

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function credit(overrides: Partial<Credit> & { id: number; name: string }): Credit {
  return {
    aliases: [],
    externalIds: [],
    videoCount: 0,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

interface Sent {
  method: string;
  path: string;
  params: URLSearchParams;
  body: unknown;
}

const server = {
  people: [] as Credit[],
  organizations: [] as Credit[],
  sent: [] as Sent[],
  nextId: 100,
  /** 設定すると、次の変更の要求（POST・PATCH・DELETE）をこの応答で 1 回だけ失敗させる。 */
  nextError: null as { status: number; body: Record<string, unknown> } | null,
};

function listOf(kind: CreditKind): Credit[] {
  return kind === "person" ? server.people : server.organizations;
}

/** pageOf は一覧の 1 ページを作る。カーソルは読み終えた数である。 */
function pageOf(items: readonly Credit[], params: URLSearchParams) {
  const q = foldForMatch(params.get("q") ?? "").trim();
  const sort = (params.get("sort") ?? "name") as Parameters<typeof compareTagsForSort>[2];
  const matched = items
    .filter(
      (item) =>
        q === "" ||
        foldForMatch(item.name).includes(q) ||
        item.aliases.some((alias) => foldForMatch(alias.name).includes(q)),
    )
    .sort((a, b) => compareTagsForSort(a, b, sort));
  const limit = params.get("limit");
  const offset = Number(params.get("cursor") ?? "0");
  const end = limit === null ? matched.length : offset + Number(limit);
  return {
    items: matched.slice(offset, end).map((item) => structuredClone(item)),
    total: matched.length,
    totalAll: items.length,
    ...(end < matched.length ? { nextCursor: String(end) } : {}),
  };
}

function handle(kind: CreditKind, rest: string, method: string, url: URL, body: unknown) {
  const items = listOf(kind);
  const notFound = kind === "person" ? "person_not_found" : "organization_not_found";
  if (rest === "" && method === "GET")
    return jsonResponse(pageOf(items, url.searchParams));
  if (rest === "" && method === "POST") {
    const created = credit({
      id: server.nextId++,
      name: (body as { name: string }).name,
    });
    items.push(created);
    return jsonResponse(created, 201);
  }
  const match = /^\/(\d+)(\/merge)?$/.exec(rest);
  const target = items.find((item) => item.id === Number(match?.[1]));
  if (target === undefined) {
    return jsonResponse({ code: "not_found", reason: notFound, message: "gone" }, 404);
  }
  if (match?.[2] !== undefined) {
    const { sourceIds } = body as { sourceIds: number[] };
    const sources = items.filter((item) => sourceIds.includes(item.id));
    for (const source of sources) {
      target.videoCount += source.videoCount;
      target.aliases.push({ name: source.name });
      items.splice(items.indexOf(source), 1);
    }
    const merged = structuredClone(target);
    return jsonResponse({
      [kind]: merged,
      notFoundIds: sourceIds.filter((id) => !sources.some((item) => item.id === id)),
    });
  }
  if (method === "PATCH") {
    const patch = body as Partial<Credit> & { parentId?: number | null };
    if (patch.name !== undefined) target.name = patch.name;
    if (patch.aliases !== undefined) target.aliases = patch.aliases ?? [];
    if (patch.externalIds !== undefined) target.externalIds = patch.externalIds ?? [];
    if (patch.parentId !== undefined) {
      const parent = items.find((item) => item.id === patch.parentId);
      Object.assign(target, {
        parent: parent === undefined ? undefined : { id: parent.id, name: parent.name },
      });
    }
    return jsonResponse(structuredClone(target));
  }
  if (method === "DELETE") {
    items.splice(items.indexOf(target), 1);
    return jsonResponse(null, 204);
  }
  return jsonResponse(structuredClone(target));
}

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    const body =
      typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : null;
    server.sent.push({ method, path: url.pathname, params: url.searchParams, body });
    if (method !== "GET" && server.nextError !== null) {
      const { status, body: error } = server.nextError;
      server.nextError = null;
      return Promise.resolve(jsonResponse(error, status));
    }
    const people = /^\/api\/people(.*)$/.exec(url.pathname);
    if (people !== null) {
      return Promise.resolve(handle("person", people[1]!, method, url, body));
    }
    const organizations = /^\/api\/organizations(.*)$/.exec(url.pathname);
    if (organizations !== null) {
      return Promise.resolve(
        handle("organization", organizations[1]!, method, url, body),
      );
    }
    return Promise.resolve(jsonResponse({ code: "not_found", message: "no" }, 404));
  });
  vi.stubGlobal("fetch", fetchMock);
}

function renderPage(
  kind: CreditKind,
  url = kind === "person" ? "/people" : "/organizations",
) {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <TooltipProvider>
        <ToastProvider>
          <header>
            <div id="topbar-library-tools" />
          </header>
          <CreditsPage kind={kind} />
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function requests(method: string, path: RegExp): Sent[] {
  return server.sent.filter((item) => item.method === method && path.test(item.path));
}

function rowOf(name: string): HTMLElement {
  return screen.getByTitle(name).closest("[data-credit-id]")!;
}

function count(): string {
  return screen
    .getAllByRole("status")
    .find((node) => node.closest('[data-slot="page-header-count"]') !== null)!
    .textContent!;
}

beforeEach(() => {
  window.localStorage.clear();
  server.people = [
    credit({
      id: 1,
      name: "Alice",
      aliases: [{ name: "アリス", language: "ja" }, { name: "Ali" }],
      videoCount: 3,
    }),
    credit({ id: 2, name: "Bob", videoCount: 2 }),
    credit({ id: 3, name: "Carol" }),
  ];
  server.organizations = [];
  server.sent = [];
  server.nextId = 100;
  server.nextError = null;
  install();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("CreditsPage 人物の一覧", () => {
  it("1 ページを名前・別名・動画の数とともに並べ、limit=100 と sort を送る", async () => {
    renderPage("person");
    await screen.findByTitle("Alice");

    expect(
      requests("GET", /^\/api\/people$/).map((item) => item.params.toString()),
    ).toEqual(["sort=name&limit=100"]);
    expect(screen.getByRole("heading", { level: 1, name: "People" })).toBeDefined();
    expect(count()).toBe("3 people");
    const alice = rowOf("Alice");
    expect(within(alice).getByText("Aliases: アリス (ja) · Ali")).toBeDefined();
    expect(within(alice).getByText("3 videos")).toBeDefined();
    expect(
      within(alice)
        .getByRole("link", { name: "Open the library filtered by Alice" })
        .getAttribute("href"),
    ).toBe("/?person=1");
    // 別名の無い行は 2 行目を持たない。
    expect(rowOf("Carol").querySelector("p")).toBeNull();
  });

  it("検索は q をサーバーへ送り、応答の行と件数を出す", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Alice");

    await user.type(screen.getByRole("searchbox", { name: "Search people" }), "アリ");

    await waitFor(() =>
      expect(
        requests("GET", /^\/api\/people$/)
          .at(-1)
          ?.params.get("q"),
      ).toBe("アリ"),
    );
    await waitFor(() => expect(screen.queryByTitle("Bob")).toBeNull());
    expect(screen.getByTitle("Alice")).toBeDefined();
    expect(count()).toBe("1 of 3 people");
  });

  it("スクロールで次の 100 件を nextCursor で読む", async () => {
    server.people = Array.from({ length: 150 }, (_, index) =>
      credit({ id: index + 1, name: `Person ${String(index).padStart(3, "0")}` }),
    );
    // jsdom には表示域と要素の高さが無いので、試験用の高さを置く（タグの画面の試験と同じ）。
    const originalInnerHeight = window.innerHeight;
    const originalOffsetHeight = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "offsetHeight",
    );
    window.innerHeight = 200;
    Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
      configurable: true,
      get(this: HTMLElement) {
        return this.hasAttribute("data-index") ? 40 : 0;
      },
    });
    try {
      renderPage("person");
      await screen.findByTitle("Person 000");
      expect(screen.queryByTitle("Person 120")).toBeNull();

      Object.defineProperty(document.documentElement, "scrollHeight", {
        configurable: true,
        value: 1_000_000,
      });
      Object.defineProperty(window, "scrollY", { configurable: true, value: 100 * 60 });
      window.dispatchEvent(new Event("scroll"));

      await waitFor(() =>
        expect(
          requests("GET", /^\/api\/people$/).some(
            (item) => item.params.get("cursor") === "100",
          ),
        ).toBe(true),
      );
      expect(await screen.findByTitle("Person 120")).toBeDefined();
    } finally {
      window.innerHeight = originalInnerHeight;
      if (originalOffsetHeight !== undefined) {
        Object.defineProperty(
          HTMLElement.prototype,
          "offsetHeight",
          originalOffsetHeight,
        );
      }
      Object.defineProperty(window, "scrollY", { configurable: true, value: 0 });
      Reflect.deleteProperty(document.documentElement, "scrollHeight");
    }
  });

  it("作成は POST { name } を送り、同じ名前でも警告せずに作る", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Alice");

    await user.click(screen.getByRole("button", { name: "New person" }));
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Alice{Enter}");

    await waitFor(() => expect(screen.getAllByTitle("Alice")).toHaveLength(2));
    expect(requests("POST", /^\/api\/people$/).map((item) => item.body)).toEqual([
      { name: "Alice" },
    ]);
    expect(count()).toBe("4 people");
  });

  it("改名は name だけの PATCH を送る", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Bob");

    await user.click(within(rowOf("Bob")).getByRole("button", { name: "Rename" }));
    const input = screen.getByRole("textbox", { name: 'New name for "Bob"' });
    await user.clear(input);
    await user.type(input, "Robert{Enter}");

    await screen.findByTitle("Robert");
    expect(requests("PATCH", /^\/api\/people\/2$/).map((item) => item.body)).toEqual([
      { name: "Robert" },
    ]);
  });

  it("詳細の窓は言語つきの別名と外部 ID の一覧を丸ごと送る", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Alice");

    await user.click(
      within(rowOf("Alice")).getByRole("button", { name: "Edit details" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Edit "Alice"' });
    // 2 つ目の別名を消し、言語つきの別名を 1 つ足す。
    await user.click(within(dialog).getAllByRole("button", { name: "Remove alias" })[1]!);
    await user.click(within(dialog).getByRole("button", { name: "Add alias" }));
    const names = within(dialog).getAllByRole("textbox", { name: "Alias" });
    const languages = within(dialog).getAllByRole("textbox", { name: "Language" });
    await user.type(names.at(-1)!, "Alicia");
    await user.type(languages.at(-1)!, "es");
    await user.click(within(dialog).getByRole("button", { name: "Add ID" }));
    await user.type(within(dialog).getByRole("textbox", { name: "Source" }), "imdb");
    await user.type(within(dialog).getByRole("textbox", { name: "Value" }), "nm1");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(requests("PATCH", /^\/api\/people\/1$/).map((item) => item.body)).toEqual([
      {
        aliases: [
          { name: "アリス", language: "ja" },
          { name: "Alicia", language: "es" },
        ],
        externalIds: [{ source: "imdb", value: "nm1" }],
      },
    ]);
    expect(
      within(rowOf("Alice")).getByText("Aliases: アリス (ja) · Alicia (es)"),
    ).toBeDefined();
  });

  it("詳細の窓は field の示す行の下に理由を出し、開いたままにする", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Alice");
    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "work_info_invalid_language",
        field: "aliases[0].language",
        message: "bad",
      },
    };

    await user.click(
      within(rowOf("Alice")).getByRole("button", { name: "Edit details" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Edit "Alice"' });
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await within(dialog).findByText("Enter a language tag such as ja or en-US."),
    ).toBeDefined();
    expect(
      within(dialog)
        .getAllByRole("textbox", { name: "Language" })[0]!
        .getAttribute("aria-invalid"),
    ).toBe("true");
  });

  it("統合は合計の videoCount で確かめてから sourceIds を送る", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Bob");

    await user.click(within(rowOf("Bob")).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another person…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "Bob"' });
    const merge = within(dialog).getByRole("button", { name: "Merge" });
    expect((merge as HTMLButtonElement).disabled).toBe(true);
    // 候補は統合元を除く。
    await within(dialog).findByRole("option", { name: /Alice/ });
    expect(within(dialog).queryByRole("option", { name: /Bob/ })).toBeNull();

    await user.click(within(dialog).getByRole("option", { name: /Alice/ }));
    expect(
      within(dialog).getByText(
        'The 2 videos with "Bob" get "Alice" instead. "Bob"\'s name, aliases and IDs become aliases and IDs of "Alice", and "Bob" leaves the list. This can\'t be undone.',
      ),
    ).toBeDefined();
    await user.click(merge);

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      requests("POST", /^\/api\/people\/1\/merge$/).map((item) => item.body),
    ).toEqual([{ sourceIds: [2] }]);
    expect(screen.queryByTitle("Bob")).toBeNull();
    expect(within(rowOf("Alice")).getByText("5 videos")).toBeDefined();
    expect(await screen.findByText('Merged "Bob" into "Alice"')).toBeDefined();
  });

  it("削除は件数で確かめてから DELETE を送る", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Alice");

    await user.click(
      within(rowOf("Alice")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Alice"?' });
    expect(
      within(dialog).getByText(
        '"Alice" will be removed from 3 videos. This can\'t be undone.',
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(requests("DELETE", /^\/api\/people\/1$/)).toHaveLength(1);
    expect(screen.queryByTitle("Alice")).toBeNull();
    expect(count()).toBe("2 people");
  });

  it("対象がもう無ければトーストを出し、一覧を先頭から読み直す", async () => {
    const user = userEvent.setup();
    renderPage("person");
    await screen.findByTitle("Carol");
    server.people = server.people.filter((item) => item.id !== 3);

    await user.click(
      within(rowOf("Carol")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByText("This person no longer exists, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByTitle("Carol")).toBeNull());
    expect(requests("GET", /^\/api\/people$/)).toHaveLength(2);
  });

  it("1 件も無ければ空の状態と作成のボタンを出す", async () => {
    server.people = [];
    renderPage("person");

    expect(await screen.findByRole("heading", { name: "No people yet" })).toBeDefined();
    expect(screen.getAllByRole("button", { name: "New person" }).length).toBeGreaterThan(
      0,
    );
  });
});

describe("CreditsPage 団体の一覧", () => {
  beforeEach(() => {
    server.organizations = [
      credit({ id: 10, name: "Studio" }),
      credit({
        id: 11,
        name: "Label",
        parent: { id: 10, name: "Studio" },
        aliases: [{ name: "Lbl" }],
      } as Partial<Credit> & { id: number; name: string }),
    ];
  });

  it("親を名前の下の 1 行の先頭に出し、名前は organization の絞り込みへのリンク", async () => {
    renderPage("organization");
    await screen.findByTitle("Label");

    expect(
      screen.getByRole("heading", { level: 1, name: "Organizations" }),
    ).toBeDefined();
    expect(
      within(rowOf("Label")).getByText("Part of Studio · Aliases: Lbl"),
    ).toBeDefined();
    expect(
      within(rowOf("Label"))
        .getByRole("link", { name: "Open the library filtered by Label" })
        .getAttribute("href"),
    ).toBe("/?organization=11");
  });

  it("親の輪の誤りを親の欄の下に出し、窓を開いたままにする", async () => {
    const user = userEvent.setup();
    renderPage("organization");
    await screen.findByTitle("Studio");
    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "organization_parent_cycle",
        message: "cycle",
      },
    };

    await user.click(
      within(rowOf("Studio")).getByRole("button", { name: "Edit details" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Edit "Studio"' });
    const parent = within(dialog).getByRole("combobox", { name: "Parent organization" });
    await waitFor(() => expect((parent as HTMLButtonElement).disabled).toBe(false));
    await user.click(parent);
    await user.click(await screen.findByRole("option", { name: "Label" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    expect(
      await within(dialog).findByText(
        "The parent can't be the organization itself or one inside it.",
      ),
    ).toBeDefined();
    expect(
      requests("PATCH", /^\/api\/organizations\/10$/).map((item) => item.body),
    ).toEqual([{ aliases: [], externalIds: [], parentId: 11 }]);
    // 親の候補は自分を除き、「None」が先頭にある。
    expect(
      requests("GET", /^\/api\/organizations$/)
        .at(-1)
        ?.params.has("limit"),
    ).toBe(false);
  });

  it("削除の確認は、中の団体が親を失うことも書く", async () => {
    const user = userEvent.setup();
    renderPage("organization");
    await screen.findByTitle("Studio");

    await user.click(
      within(rowOf("Studio")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Studio"?' });
    expect(dialog.textContent).toContain(
      "\"Studio\" isn't on any videos. This can't be undone. Organizations inside it stay and lose their parent.",
    );
  });
});
