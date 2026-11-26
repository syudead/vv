import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Organization } from "../../api/credits";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import OrganizationsPage from "./OrganizationsPage";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function organization(id: number, overrides: Partial<Organization> = {}): Organization {
  return {
    id,
    name: `Studio ${String(id)}`,
    names: [{ name: `Studio ${String(id)}` }],
    externalIds: [],
    profile: {},
    favorite: false,
    videoCount: 0,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const server = {
  items: [] as Organization[],
  totalAll: 0,
  sent: [] as { method: string; url: URL; body: unknown }[],
};

function respond(method: string, url: URL, body: unknown): Response {
  if (url.pathname === "/api/organizations" && method === "GET") {
    const q = url.searchParams.get("q");
    const items = server.items.filter(
      (item) =>
        (url.searchParams.get("favorite") !== "true" || item.favorite) &&
        (q === null || item.name.includes(q)),
    );
    return jsonResponse({ items, total: items.length, totalAll: server.totalAll });
  }
  if (url.pathname === "/api/organizations" && method === "POST") {
    return jsonResponse(
      organization(50, { name: (body as { names: { name: string }[] }).names[0]!.name }),
      201,
    );
  }
  if (url.pathname === "/api/favorites" && method === "PUT") {
    return jsonResponse({ appliedVideos: 0, appliedGroups: 0, appliedSmartFolders: 0 });
  }
  return jsonResponse({ code: "not_found" }, 404);
}

beforeEach(() => {
  window.localStorage.clear();
  server.items = [organization(1), organization(2)];
  server.totalAll = 2;
  server.sent = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      const method = init?.method ?? "GET";
      const body =
        typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : null;
      server.sent.push({ method, url, body });
      return Promise.resolve(respond(method, url, body));
    }),
  );
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderPage(url = "/organizations") {
  render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={[url]}>
          <Routes>
            <Route path="/organizations" element={<OrganizationsPage />} />
            <Route path="/organizations/:id" element={<p>Organization page</p>} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

function listRequests(): URLSearchParams[] {
  return server.sent
    .filter(
      (request) =>
        request.method === "GET" && request.url.pathname === "/api/organizations",
    )
    .map((request) => request.url.searchParams);
}

describe("OrganizationsPage", () => {
  it("restores the search, sort and favorites filter from the URL", async () => {
    server.items = [organization(3, { name: "Kumo Works", favorite: true })];
    renderPage("/organizations?q=Kumo&sort=countDesc&favorite=true");

    await screen.findByRole("link", { name: /Kumo Works/ });
    expect(listRequests().map((params) => params.toString())).toEqual([
      "q=Kumo&sort=countDesc&limit=100&favorite=true",
    ]);
    expect(
      (
        screen.getByRole("searchbox", {
          name: "Search organizations",
        }) as HTMLInputElement
      ).value,
    ).toBe("Kumo");
    expect(screen.getByRole("button", { name: /^Sort by: Video count/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Filter (1 applied)" })).toBeTruthy();
  });

  it("draws a card's logo, or its name in the frame without one, and gives it only the heart", async () => {
    server.items = [
      organization(7, { name: "Kumo", imageRevision: 4, videoCount: 12 }),
      organization(8, { name: "Blue Harbor", videoCount: 1 }),
    ];
    renderPage();

    const withLogo = await screen.findByRole("link", { name: /Kumo/ });
    expect(withLogo.getAttribute("href")).toBe("/organizations/7");
    expect(
      within(withLogo).getByRole("img", { name: "Logo of Kumo" }).getAttribute("src"),
    ).toBe("/api/organizations/7/image?v=4");
    expect(within(withLogo).getByText("12 videos")).toBeTruthy();

    const withoutLogo = screen.getByRole("link", { name: /Blue Harbor/ });
    expect(within(withoutLogo).queryByRole("img")).toBeNull();
    const frame = withoutLogo.querySelector('[data-slot="organization-logo"]')!;
    expect(frame.textContent).toBe("Blue Harbor");

    const card = withLogo.closest("article")!;
    expect(
      within(card)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["Add to favorites"]);
  });

  it("sends a card's heart as the organization's favorite and shows it pressed after the response", async () => {
    const user = userEvent.setup();
    renderPage();
    const card = (await screen.findByRole("link", { name: /Studio 2/ })).closest(
      "article",
    )!;

    await user.click(within(card).getByRole("button", { name: "Add to favorites" }));

    const pressed = await within(card).findByRole("button", {
      name: "Remove from favorites",
    });
    expect(pressed.getAttribute("aria-pressed")).toBe("true");
    const favorite = server.sent.find(
      (request) => request.url.pathname === "/api/favorites",
    );
    expect(favorite?.body).toEqual({
      videoIds: [],
      groupIds: [],
      organizationIds: [2],
      favorite: true,
    });
  });

  it("creates an organization from the dialog and opens its page", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: /Studio 1/ });

    await user.click(screen.getByRole("button", { name: "New organization" }));
    expect(screen.getByRole("dialog", { name: "New organization" })).toBeTruthy();
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Kumo{Enter}");

    expect(await screen.findByText("Organization page")).toBeTruthy();
    const created = server.sent.find((request) => request.method === "POST");
    expect(created?.body).toEqual({ names: [{ name: "Kumo" }] });
  });

  it("offers New organization when there are none yet", async () => {
    server.items = [];
    server.totalAll = 0;
    renderPage();

    expect(await screen.findByText("No organizations yet")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "New organization" })).toHaveLength(2);
  });

  it("clears a search without a match and says when no organization is a favorite", async () => {
    const user = userEvent.setup();
    renderPage("/organizations?q=Nothing");

    expect(await screen.findByText('No organizations match "Nothing"')).toBeTruthy();
    await user.click(screen.getByText("Clear search", { selector: "button" }));
    await screen.findByRole("link", { name: /Studio 1/ });
    expect(listRequests().at(-1)?.has("q")).toBe(false);

    await user.click(screen.getByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("checkbox", { name: "Favorites only" }));

    expect(await screen.findByText("No favorite organizations yet")).toBeTruthy();
    expect(listRequests().at(-1)?.get("favorite")).toBe("true");
  });
});
