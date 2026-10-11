import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PlaylistSummary } from "../api/playlists";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import PlaylistsPage from "./PlaylistsPage";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function playlist(id: number, name: string, itemCount: number): PlaylistSummary {
  return { id, name, itemCount, updatedAt: "2026-09-01T00:00:00Z" };
}

const server = {
  /** 一覧の応答（最後に変えた順）。作るとその先頭に入る。 */
  items: [] as PlaylistSummary[],
  listStatus: 200,
  listCalls: 0,
  /** 作る要求の本文。 */
  creates: [] as unknown[],
  createStatus: 201,
  nextId: 100,
};

function install() {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/playlists" && method === "GET") {
      server.listCalls += 1;
      return Promise.resolve(
        server.listStatus === 200
          ? json({ items: server.items })
          : json({ code: "internal", message: "x" }, server.listStatus),
      );
    }
    if (url === "/api/playlists" && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { name: string };
      server.creates.push(body);
      if (server.createStatus === 400) {
        return Promise.resolve(
          json({ code: "invalid_request", message: "bad name" }, 400),
        );
      }
      if (server.createStatus !== 201) {
        return Promise.resolve(
          json({ code: "internal", message: "x" }, server.createStatus),
        );
      }
      const created = {
        ...playlist(server.nextId, body.name, 0),
        updatedAt: "2026-09-02T00:00:00Z",
      };
      server.nextId += 1;
      server.items = [created, ...server.items];
      return Promise.resolve(json(created, 201));
    }
    throw new Error(`unexpected request: ${method} ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
}

function renderPage() {
  render(
    <MemoryRouter initialEntries={["/playlists"]}>
      <TooltipProvider>
        <ToastProvider>
          <Routes>
            <Route path="/playlists" element={<PlaylistsPage />} />
            <Route path="/playlists/:id" element={<p>Playlist screen</p>} />
          </Routes>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

/** rowNames は表の行の名前のリンクの読み上げ名を上から返す。 */
function rowNames(): string[] {
  const table = screen.getByRole("table", { name: "Playlists" });
  return within(table)
    .getAllByRole("link")
    .map((link) => link.getAttribute("aria-label") ?? "");
}

beforeEach(() => {
  server.items = [
    playlist(3, "Morning", 12),
    playlist(1, "Road trip", 1),
    playlist(2, "Empty one", 0),
  ];
  server.listStatus = 200;
  server.listCalls = 0;
  server.creates = [];
  server.createStatus = 201;
  server.nextId = 100;
  install();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("PlaylistsPage（specs/044-playlists/ui-design.md「Playlists screen」）", () => {
  it("プレイリストを API の順（最後に変えた順）に、項目数とともに並べる", async () => {
    renderPage();
    expect(screen.getByRole("heading", { level: 1, name: "Playlists" })).toBeDefined();
    expect(document.title).toBe("Playlists");

    await screen.findByRole("table", { name: "Playlists" });
    expect(rowNames()).toEqual([
      "Morning, 12 items",
      "Road trip, 1 item",
      "Empty one, 0 items",
    ]);
    expect(screen.getByText("3 playlists")).toBeDefined();
    expect(screen.getByText("0 items")).toBeDefined();
    expect(
      screen.getByRole("link", { name: "Morning, 12 items" }).getAttribute("href"),
    ).toBe("/playlists/3");
  });

  it("同じ名前の 2 つのプレイリストをまとめずにどちらも並べる", async () => {
    server.items = [playlist(5, "Same", 2), playlist(4, "Same", 7)];
    renderPage();

    await screen.findByRole("table", { name: "Playlists" });
    expect(rowNames()).toEqual(["Same, 2 items", "Same, 7 items"]);
    expect(screen.getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/playlists/5",
      "/playlists/4",
    ]);
  });

  it("名前を入れて作ると、トリムした名前を送り、先頭の行の名前へフォーカスを移す", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Playlists" });

    await user.click(screen.getByRole("button", { name: "New playlist" }));
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    expect(document.activeElement).toBe(field);
    await user.type(field, "  Evening  ");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.creates).toEqual([{ name: "Evening" }]);
    expect(rowNames()[0]).toBe("Evening, 0 items");
    expect(screen.getByText("4 playlists")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Evening, 0 items" }),
      ),
    );
    // 作った後に一覧を読み直す（ui-design.md「States」）。
    await waitFor(() => expect(server.listCalls).toBe(2));
    expect(rowNames()).toHaveLength(4);
  });

  it("空の名前と 100 文字を超える名前は送らず、名前の規則を欄の誤りとして出す", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Playlists" });

    await user.click(screen.getByRole("button", { name: "New playlist" }));
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });
    await user.type(field, "   ");
    // 入れている間は誤りを出さない。
    expect(within(dialog).queryByText("Use a name of 1 to 100 characters.")).toBeNull();
    await user.keyboard("{Enter}");

    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(field.getAttribute("aria-invalid")).toBe("true");
    await user.clear(field);
    expect(field.getAttribute("aria-invalid")).toBeNull();

    await user.click(field);
    await user.paste("a".repeat(101));
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    expect(within(dialog).getByText("Use a name of 1 to 100 characters.")).toBeDefined();
    expect(server.creates).toEqual([]);
  });

  it("400 は名前の規則を、ほかの失敗は理由を添えた 1 行を窓の中に出し、窓を開いたままにする", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Playlists" });
    await user.click(screen.getByRole("button", { name: "New playlist" }));
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    const field = within(dialog).getByRole("textbox", { name: "Name" });

    server.createStatus = 400;
    await user.type(field, "Night{Enter}");
    expect(
      await within(dialog).findByText("Use a name of 1 to 100 characters."),
    ).toBeDefined();

    server.createStatus = 500;
    await user.type(field, "s{Enter}");
    expect(
      await within(dialog).findByText(/^Couldn't create the playlist: /),
    ).toBeDefined();
    expect(screen.getByRole("dialog", { name: "New playlist" })).toBeDefined();
    expect(server.items).toHaveLength(3);
  });

  it("Cancel で閉じると入れた名前を捨てる", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table", { name: "Playlists" });
    await user.click(screen.getByRole("button", { name: "New playlist" }));
    let dialog = await screen.findByRole("dialog", { name: "New playlist" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Draft");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getByRole("button", { name: "New playlist" }));
    dialog = await screen.findByRole("dialog", { name: "New playlist" });
    expect(
      (within(dialog).getByRole("textbox", { name: "Name" }) as HTMLInputElement).value,
    ).toBe("");
    expect(server.creates).toEqual([]);
  });

  it("空なら本体の「New playlist」だけを出し、作ると表に変わる", async () => {
    server.items = [];
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("No playlists yet")).toBeDefined();
    const buttons = screen.getAllByRole("button", { name: "New playlist" });
    expect(buttons).toHaveLength(1);
    await user.click(buttons[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "New playlist" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Name" }),
      "First{Enter}",
    );

    expect(await screen.findByRole("table", { name: "Playlists" })).toBeDefined();
    expect(rowNames()).toEqual(["First, 0 items"]);
    expect(screen.getByText("1 playlist")).toBeDefined();
  });

  it("読み込みに失敗したら Retry で読み直す", async () => {
    server.listStatus = 500;
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("Couldn't load the playlists")).toBeDefined();
    server.listStatus = 200;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("table", { name: "Playlists" })).toBeDefined();
    expect(rowNames()).toHaveLength(3);
  });

  it("疑似ロケールではカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    renderPage();
    await screen.findByRole("table", { name: /Playlists/ });
    expectCatalogTextOnly(document.body, ["Morning", "Road trip", "Empty one"]);
  });
});
