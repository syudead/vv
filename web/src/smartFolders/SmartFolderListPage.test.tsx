import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SmartFolderSummary } from "../api/client";
import { emitServerEvent, installFakeEventSource } from "../api/fakeEventSource";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";
import SmartFolderListPage from "./SmartFolderListPage";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** GET /api/smart-folders answers in name order, as the server does. */
let folders: SmartFolderSummary[] = [];
let folderReads = 0;
let favoriteAnswer: (() => void) | null = null;

const fetchMock = vi.fn<typeof fetch>((input, init) => {
  const url = new URL(String(input), "http://localhost");
  if (url.pathname === "/api/smart-folders" && init?.method === "POST") {
    const body = JSON.parse(String(init.body)) as { name: string; parentId?: number };
    const created: SmartFolderSummary = {
      id: 9,
      name: body.name,
      favorite: false,
      ...(body.parentId === undefined ? {} : { parentId: body.parentId }),
    };
    // The server places the new folder by its own name order.
    folders = [folders[0] as SmartFolderSummary, created, ...folders.slice(1)];
    return Promise.resolve(json(created, 201));
  }
  if (url.pathname === "/api/smart-folders") {
    folderReads += 1;
    return Promise.resolve(json({ smartFolders: folders }));
  }
  if (url.pathname === "/api/favorites") {
    return new Promise((resolve) => {
      favoriteAnswer = () =>
        resolve(json({ appliedVideos: 0, appliedGroups: 0, appliedSmartFolders: 1 }));
    });
  }
  throw new Error(`unexpected request: ${url.toString()}`);
});

function FolderProbe() {
  const location = useLocation();
  const state = location.state as { from?: string } | null;
  return <p>{`folder page ${location.pathname} from ${state?.from ?? ""}`}</p>;
}

function renderPage() {
  render(
    <MemoryRouter initialEntries={["/smart-folders"]}>
      <TooltipProvider>
        <ToastProvider>
          <Routes>
            <Route path="/smart-folders" element={<SmartFolderListPage />} />
            <Route path="/smart-folders/:id" element={<FolderProbe />} />
          </Routes>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function table(): HTMLElement {
  return screen.getByRole("table", { name: "Smart folders" });
}

function rowNames(): string[] {
  return within(table())
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("link")[0]?.textContent ?? "");
}

beforeEach(() => {
  folders = [
    { id: 1, name: "A", favorite: false },
    { id: 2, name: "B", favorite: true },
    { id: 3, name: "B-1", parentId: 2, favorite: true },
    { id: 4, name: "C", favorite: false },
  ];
  folderReads = 0;
  favoriteAnswer = null;
  vi.stubGlobal("fetch", fetchMock);
  // The dialog's Radix Select and the toast's swipe use these, which jsdom lacks.
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.releasePointerCapture ??= () => undefined;
  Element.prototype.setPointerCapture ??= () => undefined;
  // Row values stay unread; the page's own list read is the contract here.
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  installFakeEventSource();
});

afterEach(() => {
  cleanup();
  fetchMock.mockClear();
  vi.unstubAllGlobals();
});

describe("SmartFolderListPage", () => {
  it("lists only the root folders in the server's order and opens a folder's page from its row", async () => {
    const user = userEvent.setup();
    renderPage();

    await waitFor(() => expect(rowNames()).toEqual(["A", "B", "C"]));
    expect(screen.getByText("3 smart folders")).toBeTruthy();
    expect(document.title).toBe("Smart folders");

    await user.click(within(table()).getByRole("link", { name: "B, smart folder" }));
    expect(
      await screen.findByText("folder page /smart-folders/2 from /smart-folders"),
    ).toBeTruthy();
  });

  it("creates a folder from the page, shows it where the server puts it and focuses it", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(rowNames()).toEqual(["A", "B", "C"]));

    await user.click(screen.getByRole("button", { name: "New smart folder" }));
    const dialog = await screen.findByRole("dialog", { name: "New smart folder" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "AA");
    await user.type(within(dialog).getByRole("searchbox", { name: "Search" }), "cat");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    await waitFor(() => expect(rowNames()).toEqual(["A", "AA", "B", "C"]));
    await waitFor(() => expect(document.activeElement?.textContent).toBe("AA"));
    const post = fetchMock.mock.calls.find(([, init]) => init?.method === "POST");
    expect(JSON.parse(String(post?.[1]?.body))).not.toHaveProperty("parentId");
  });

  it("offers the page of a created child folder, which has no row here, from the toast", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(rowNames()).toEqual(["A", "B", "C"]));

    await user.click(screen.getByRole("button", { name: "New smart folder" }));
    const dialog = await screen.findByRole("dialog", { name: "New smart folder" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "AA");
    await user.click(within(dialog).getByRole("combobox", { name: "Parent" }));
    await user.click(await screen.findByRole("option", { name: "B" }));
    await user.type(within(dialog).getByRole("searchbox", { name: "Search" }), "cat");
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    // The page's read, the dialog's parent choices, then the page's read after the creation.
    await waitFor(() => expect(folderReads).toBe(3));
    expect(rowNames()).toEqual(["A", "B", "C"]);
    await user.click(await screen.findByRole("button", { name: "Open" }));
    expect(
      await screen.findByText("folder page /smart-folders/9 from /smart-folders"),
    ).toBeTruthy();
  });

  it("sends the row heart as smartFolderIds and fills it only after the answer", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(rowNames()).toEqual(["A", "B", "C"]));

    const heart = screen.getByRole("button", { name: 'Favorite smart folder "A"' });
    expect(heart.getAttribute("aria-pressed")).toBe("false");
    await user.click(heart);
    const put = fetchMock.mock.calls.find(
      ([input]) =>
        new URL(String(input), "http://localhost").pathname === "/api/favorites",
    );
    expect(JSON.parse(String(put?.[1]?.body))).toMatchObject({
      smartFolderIds: [1],
      favorite: true,
    });
    expect(heart.getAttribute("aria-pressed")).toBe("false");

    favoriteAnswer?.();
    await waitFor(() => expect(heart.getAttribute("aria-pressed")).toBe("true"));
  });

  it("reads the list again on a smartFolder event, so changes from another tab appear", async () => {
    renderPage();
    await waitFor(() => expect(rowNames()).toEqual(["A", "B", "C"]));

    folders = [{ id: 4, name: "C", favorite: false }];
    await emitServerEvent("smartFolder", { ids: [1, 2, 3] });

    await waitFor(() => expect(rowNames()).toEqual(["C"]));
    expect(folderReads).toBe(2);
    expect(screen.getByText("1 smart folder")).toBeTruthy();
  });

  it("offers the creation in the empty state, with no count and no header action", async () => {
    folders = [];
    renderPage();

    expect(await screen.findByText("No smart folders yet")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "New smart folder" })).toHaveLength(1);
    expect(screen.queryByText(/^\d+ smart folders?$/)).toBeNull();
  });
});
