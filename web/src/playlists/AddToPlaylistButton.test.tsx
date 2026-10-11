import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PlaylistSummary } from "../api/playlists";
import { t } from "../i18n";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";
import AddToPlaylistButton from "./AddToPlaylistButton";

/**
 * 「Add to playlist」のピッカー（specs/044-playlists/ui-design.md「Add to playlist」）。
 * 動画ページの形の引き金で開き、偽のサーバーへ送る。
 */

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function playlist(id: number, name: string): PlaylistSummary {
  return { id, name, itemCount: 0, updatedAt: "2026-10-01T00:00:00Z" };
}

interface Server {
  playlists: PlaylistSummary[];
  /** 受け取った POST の経路と本文。 */
  posts: { path: string; body: unknown }[];
  /** 次の GET /api/playlists を失敗にする。 */
  failList?: boolean;
  /** POST …/items の応答を差し替える。 */
  addAnswer?: () => Response;
}

const fetchMock = vi.fn<typeof fetch>();

function install(playlists: PlaylistSummary[]): Server {
  const server: Server = { playlists, posts: [] };
  fetchMock.mockImplementation((input, init) => {
    const path = String(input);
    const method = init?.method ?? "GET";
    if (path === "/api/playlists" && method === "GET") {
      if (server.failList === true) {
        server.failList = false;
        return Promise.resolve(json({ code: "internal", message: "boom" }, 500));
      }
      return Promise.resolve(json({ items: server.playlists }));
    }
    if (method === "POST") {
      const body = JSON.parse(String(init?.body)) as unknown;
      server.posts.push({ path, body });
      if (path === "/api/playlists") {
        const created = playlist(90, (body as { name: string }).name.trim());
        server.playlists = [created, ...server.playlists];
        return Promise.resolve(json(created, 201));
      }
      if (server.addAnswer !== undefined) return Promise.resolve(server.addAnswer());
      return Promise.resolve(json({ added: 1, existing: 0 }));
    }
    throw new Error(`unexpected request: ${method} ${path}`);
  });
  return server;
}

function renderButton() {
  return render(
    <TooltipProvider>
      <ToastProvider>
        <AddToPlaylistButton
          variant="page"
          label={t.playlists.add.label}
          videoIds={[7]}
        />
      </ToastProvider>
    </TooltipProvider>,
  );
}

function field() {
  return screen.getByRole("combobox", { name: "Search or create a playlist" });
}

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Add to playlist" }));
  return screen.findByRole("dialog", { name: "Add to playlist" });
}

function optionNames(): string[] {
  return within(screen.getByRole("listbox", { name: "Playlists" }))
    .getAllByRole("option")
    .map((option) => option.textContent ?? "");
}

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  fetchMock.mockReset();
  vi.unstubAllGlobals();
});

describe("AddToPlaylistButton", () => {
  it("開くと入力にフォーカスし、API の順に出し、選ぶと videoIds を送ってトーストを出して閉じる", async () => {
    const server = install([playlist(2, "Tonight"), playlist(1, "Anime")]);
    const user = userEvent.setup();
    renderButton();
    await openPicker(user);
    await waitFor(() => expect(optionNames()).toEqual(["Tonight", "Anime"]));
    expect(document.activeElement).toBe(field());

    await user.click(screen.getByRole("option", { name: "Anime" }));
    expect(await screen.findByText('Added 1 item to "Anime"')).toBeDefined();
    expect(server.posts).toEqual([
      { path: "/api/playlists/1/items", body: { videoIds: [7], folders: [] } },
    ]);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("打つと絞り込み、同じ名前が無ければ作成の行を先頭に出し、Enter で作ってから足す", async () => {
    const server = install([playlist(2, "Tonight"), playlist(1, "Anime")]);
    const user = userEvent.setup();
    renderButton();
    await openPicker(user);
    await waitFor(() => expect(optionNames()).toHaveLength(2));

    await user.type(field(), "night");
    expect(optionNames()).toEqual(['Create "night"', "Tonight"]);
    await user.clear(field());
    // 同じ名前があれば作成の行は出さない（R-11）。
    await user.type(field(), "Tonight");
    expect(optionNames()).toEqual(["Tonight"]);
    await user.clear(field());

    await user.type(field(), "  Later  ");
    expect(optionNames()).toEqual(['Create "Later"']);
    await user.keyboard("{Enter}");
    expect(await screen.findByText('Added 1 item to "Later"')).toBeDefined();
    expect(server.posts).toEqual([
      { path: "/api/playlists", body: { name: "Later" } },
      { path: "/api/playlists/90/items", body: { videoIds: [7], folders: [] } },
    ]);
  });

  it("既に入っていたら「Already in」と伝える", async () => {
    const server = install([playlist(2, "Tonight")]);
    server.addAnswer = () => json({ added: 0, existing: 1 });
    const user = userEvent.setup();
    renderButton();
    await openPicker(user);
    await user.click(await screen.findByRole("option", { name: "Tonight" }));
    expect(await screen.findByText('Already in "Tonight"')).toBeDefined();
  });

  it("100 文字を超える名前は理由を出し、Enter でも押しても何もしない", async () => {
    const server = install([]);
    const user = userEvent.setup();
    renderButton();
    await openPicker(user);
    expect(
      await screen.findByText("No playlists yet. Type a name to create one."),
    ).toBeDefined();
    await user.click(field());
    await user.paste("a".repeat(101));
    expect(screen.getByText("Use a name of up to 100 characters")).toBeDefined();
    await user.keyboard("{Enter}");
    await user.click(screen.getByRole("option", { name: /^Create/ }));
    expect(server.posts).toEqual([]);
  });

  it("消えていたプレイリスト（404）では開いたまま理由を出し、一覧を読み直す", async () => {
    const server = install([playlist(2, "Tonight"), playlist(1, "Anime")]);
    server.addAnswer = () => json({ code: "not_found", message: "not found" }, 404);
    const user = userEvent.setup();
    renderButton();
    const dialog = await openPicker(user);
    await user.click(await screen.findByRole("option", { name: "Tonight" }));
    expect(
      await within(dialog).findByText(/^Couldn't add to the playlist: /),
    ).toBeDefined();
    expect(screen.getByRole("dialog", { name: "Add to playlist" })).toBe(dialog);
    const reads = () =>
      fetchMock.mock.calls.filter(
        ([input, init]) =>
          String(input) === "/api/playlists" && (init?.method ?? "GET") === "GET",
      ).length;
    await waitFor(() => expect(reads()).toBe(2));
  });

  it("一覧を読めなければ Retry を出し、押すと読み直す", async () => {
    const server = install([playlist(2, "Tonight")]);
    server.failList = true;
    const user = userEvent.setup();
    renderButton();
    await openPicker(user);
    expect(await screen.findByText("Couldn't load the playlists")).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(optionNames()).toEqual(["Tonight"]));
  });
});
