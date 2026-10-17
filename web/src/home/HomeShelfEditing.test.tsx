import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFakeEventSource } from "../api/fakeEventSource";
import type { Home, HomeShelf } from "../api/home";
import { type Audience, AudienceProvider } from "../auth/audience";
import { json, video } from "../testing/groupPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import HomePage from "./HomePage";

// 所有者のホームの棚の操作（docs/design-docs/home.md「Home」、research.md R-7）。棚を
// 動かす・外す・足すと、PUT /api/home/shelves にフォルダの棚の新しい並びを送る。

function folderShelf(id: number, name: string): HomeShelf {
  return { key: `folder:${String(id)}`, kind: "folder", name, folderId: id };
}

const travel = folderShelf(3, "Travel");
const food = folderShelf(4, "Food");
const recent: HomeShelf = { key: "recent", kind: "recent", name: "" };

const server = {
  home: { shelves: [] } as Home,
  puts: [] as number[][],
  folders: [] as { id: number; name: string }[],
  /** 置くと GET /api/home はこの約束が解けるまで答えない。 */
  holdHome: undefined as Promise<void> | undefined,
};

/** applyOrder は書いた並びをサーバーの棚の一覧に当てる。 */
function applyOrder(ids: readonly number[]) {
  const known = new Map(
    [...server.folders, { id: 3, name: "Travel" }, { id: 4, name: "Food" }].map(
      (folder) => [folder.id, folder.name],
    ),
  );
  const fixed = server.home.shelves.filter((shelf) => shelf.kind !== "folder");
  server.home = {
    ...server.home,
    shelves: [...ids.map((id) => folderShelf(id, known.get(id) ?? "")), ...fixed],
  };
}

beforeEach(() => {
  server.home = { shelves: [travel, food, recent], organizer: { running: false } };
  server.puts = [];
  server.holdHome = undefined;
  server.folders = [
    { id: 3, name: "Travel" },
    { id: 4, name: "Food" },
    { id: 7, name: "Cats" },
  ];
  installFakeEventSource();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/home/shelves" && init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as { folderIds: number[] };
        server.puts.push(body.folderIds);
        applyOrder(body.folderIds);
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (url.pathname === "/api/home") {
        const hold = server.holdHome ?? Promise.resolve();
        return hold.then(() => json(server.home));
      }
      if (url.pathname === "/api/home/shelves") {
        const keys = (url.searchParams.get("keys") ?? "").split(",");
        return Promise.resolve(
          json({
            shelves: keys.map((key, index) => ({
              key,
              items: [{ kind: "video", video: video(10 + index) }],
              total: 1,
            })),
          }),
        );
      }
      if (url.pathname === "/api/smart-folders") {
        return Promise.resolve(json({ smartFolders: server.folders }));
      }
      throw new Error(`unexpected request: ${url.toString()}`);
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderHome(audience: Audience = "owner") {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience={audience}>
            <Routes>
              <Route path="/" element={<HomePage />} />
            </Routes>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

/** headings は描いた棚の見出しを上から順に返す。 */
function headings(): string[] {
  return screen
    .getAllByRole("heading", { level: 2 })
    .map((heading) => heading.textContent ?? "");
}

describe("ホームの棚の操作（docs/design-docs/home.md「Home」）", () => {
  it("棚を動かすと新しい並びを送り、行がその並びになる", async () => {
    const user = userEvent.setup();
    renderHome();

    await user.click(
      await screen.findByRole("button", { name: "More actions for Food" }),
    );
    expect(screen.queryByRole("menuitem", { name: "Move down" })).toBeNull();
    await user.click(screen.getByRole("menuitem", { name: "Move up" }));

    expect(server.puts).toEqual([[4, 3]]);
    await waitFor(() => expect(headings()).toEqual(["Food", "Travel", "Recently added"]));
  });

  it("書いた後の読み直しが終わるまで、棚の操作を受け付けない", async () => {
    const user = userEvent.setup();
    renderHome();
    const travelMore = await screen.findByRole("button", {
      name: "More actions for Travel",
    });
    let release = () => {};
    server.holdHome = new Promise((resolve) => {
      release = resolve;
    });

    await user.click(travelMore);
    await user.click(screen.getByRole("menuitem", { name: "Remove from home" }));
    expect(server.puts).toEqual([[4]]);

    // 読み直す前の並び（[3, 4]）から次を作らせない。
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "More actions for Food" })
          .hasAttribute("disabled"),
      ).toBe(true),
    );
    expect(
      screen.getByRole("button", { name: "Add shelf" }).hasAttribute("disabled"),
    ).toBe(true);

    server.holdHome = undefined;
    release();
    await waitFor(() => expect(headings()).toEqual(["Food", "Recently added"]));
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "More actions for Food" })
          .hasAttribute("disabled"),
      ).toBe(false),
    );
  });

  it("Remove from home は棚を外し、トーストの Undo で元の位置に戻す", async () => {
    const user = userEvent.setup();
    renderHome();

    await user.click(
      await screen.findByRole("button", { name: "More actions for Travel" }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Remove from home" }));

    expect(server.puts).toEqual([[4]]);
    const toast = await screen.findByText('Removed "Travel" from your home.');
    await waitFor(() => expect(headings()).toEqual(["Food", "Recently added"]));

    const notification = toast.closest("li") as HTMLElement;
    // Sonner の通知は押すと pointer capture を使い、jsdom に無いので click だけを送る。
    fireEvent.click(within(notification).getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(server.puts.at(-1)).toEqual([3, 4]));
    await waitFor(() => expect(headings()).toEqual(["Travel", "Food", "Recently added"]));
  });

  it("Add shelf はホームに無いフォルダを、最後のフォルダの棚の後に足す", async () => {
    const user = userEvent.setup();
    renderHome();

    await user.click(await screen.findByRole("button", { name: "Add shelf" }));
    const option = await screen.findByRole("option", { name: "Cats" });
    expect(screen.queryByRole("option", { name: "Travel" })).toBeNull();
    await user.click(option);

    expect(server.puts).toEqual([[3, 4, 7]]);
    expect(await screen.findByText('Added "Cats" to your home.')).toBeTruthy();
    await waitFor(() =>
      expect(headings()).toEqual(["Travel", "Food", "Cats", "Recently added"]),
    );
  });

  it("どのフォルダももうホームにあれば、Add shelf はそう言う", async () => {
    const user = userEvent.setup();
    server.folders = [
      { id: 3, name: "Travel" },
      { id: 4, name: "Food" },
    ];
    renderHome();

    await user.click(await screen.findByRole("button", { name: "Add shelf" }));

    expect(
      await screen.findByText("Every smart folder is already on your home."),
    ).toBeTruthy();
  });

  it("整理役が走っている間、最後のフォルダの棚の下に行を出す", async () => {
    server.home = { ...server.home, organizer: { running: true } };
    renderHome();

    const food = await screen.findByRole("heading", { level: 2, name: "Food" });
    const line = screen.getByText("Organizing your shelves…");
    const recentHeading = screen.getByRole("heading", {
      level: 2,
      name: "Recently added",
    });
    expect(
      food.compareDocumentPosition(line) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      line.compareDocumentPosition(recentHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("閲覧者には Add shelf も棚の More も出さない", async () => {
    renderHome("guest");

    await screen.findByRole("heading", { level: 2, name: "Travel" });
    expect(screen.queryByRole("button", { name: "Add shelf" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^More actions for/ })).toBeNull();
  });
});
