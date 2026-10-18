import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Person } from "../../api/credits";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import PeoplePage from "./PeoplePage";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function person(id: number, overrides: Partial<Person> = {}): Person {
  return {
    id,
    name: `Person ${String(id)}`,
    aliases: [],
    externalIds: [],
    profile: {},
    favorite: false,
    videoCount: 0,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const server = {
  /** Pages keyed by the cursor they answer; the first page has none. */
  pages: new Map<string | null, { items: Person[]; nextCursor?: string }>(),
  totalAll: 0,
  sent: [] as { method: string; url: URL; body: unknown }[],
  /** A list request asking for this sort answers 500. */
  failSort: null as string | null,
};

let reveal = () => {};

class FakeIntersectionObserver {
  private readonly callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
  }
  observe() {
    reveal = () =>
      this.callback(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      );
  }
  disconnect() {
    reveal = () => {};
  }
}

function respond(method: string, url: URL, body: unknown): Response {
  if (
    url.pathname === "/api/people" &&
    method === "GET" &&
    url.searchParams.get("sort") === server.failSort
  ) {
    return jsonResponse({ code: "internal", message: "boom" }, 500);
  }
  if (url.pathname === "/api/people" && method === "GET") {
    const page = server.pages.get(url.searchParams.get("cursor")) ?? { items: [] };
    const items =
      url.searchParams.get("favorite") === "true"
        ? page.items.filter((item) => item.favorite)
        : page.items;
    return jsonResponse({
      ...page,
      items,
      total: items.length,
      totalAll: server.totalAll,
    });
  }
  if (url.pathname === "/api/people" && method === "POST") {
    return jsonResponse(person(50, { name: (body as { name: string }).name }), 201);
  }
  return jsonResponse({ code: "not_found" }, 404);
}

beforeEach(() => {
  window.localStorage.clear();
  server.pages = new Map([[null, { items: [person(1), person(2)] }]]);
  server.totalAll = 2;
  server.sent = [];
  server.failSort = null;
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
  vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function renderPage(url = "/people") {
  render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={[url]}>
          <Routes>
            <Route path="/people" element={<PeoplePage />} />
            <Route path="/people/:id" element={<p>Person page</p>} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </TooltipProvider>,
  );
}

function listRequests(): URLSearchParams[] {
  return server.sent
    .filter(
      (request) => request.method === "GET" && request.url.pathname === "/api/people",
    )
    .map((request) => request.url.searchParams);
}

async function chooseSort(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
  await user.click(await screen.findByRole("menuitemradio", { name }));
}

describe("PeoplePage", () => {
  it("links a card to the person page with the age and count, and gives it only the heart", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 18, 12, 0, 0));
    server.pages.set(null, {
      items: [
        person(7, { name: "Aoi", profile: { birthDate: "1992-05-01" }, videoCount: 12 }),
        person(8, { name: "Ren", profile: { birthDate: "1992" }, videoCount: 1 }),
      ],
    });
    renderPage();

    const link = await screen.findByRole("link", { name: /Aoi/ });
    expect(link.getAttribute("href")).toBe("/people/7");
    expect(within(link).getByText("34 · 12 videos")).toBeTruthy();
    expect(screen.getByText("33–34 · 1 video")).toBeTruthy();
    const card = link.closest("article")!;
    expect(
      within(card)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["Add to favorites"]);
    expect(
      screen.queryByRole("button", { name: /Rename|More actions|Delete/ }),
    ).toBeNull();
  });

  it("asks for the next page with nextCursor when the end of the grid comes into view", async () => {
    server.pages.set(null, { items: [person(1)], nextCursor: "c1" });
    server.pages.set("c1", { items: [person(2)] });
    renderPage();
    await screen.findByRole("link", { name: /Person 1/ });

    act(() => reveal());

    expect(await screen.findByRole("link", { name: /Person 2/ })).toBeTruthy();
    expect(listRequests().map((params) => params.toString())).toEqual([
      "sort=name&limit=100",
      "sort=name&limit=100&cursor=c1",
    ]);
  });

  it("asks for the youngest first when that sort is chosen", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: /Person 1/ });

    await chooseSort(user, "Youngest first");

    await waitFor(() => expect(listRequests().at(-1)?.get("sort")).toBe("ageAsc"));
    expect(screen.getByRole("button", { name: "Sort by: Youngest first" })).toBeTruthy();
  });

  it("keeps the loaded cards under a notice with Retry when a refresh fails", async () => {
    const user = userEvent.setup();
    server.failSort = "ageAsc";
    renderPage();
    await screen.findByRole("link", { name: /Person 1/ });

    await chooseSort(user, "Youngest first");

    expect(
      await screen.findByText(/The list below may not match the current search and sort/),
    ).toBeTruthy();
    expect(screen.getByRole("link", { name: /Person 1/ })).toBeTruthy();
    expect(screen.getByRole("link", { name: /Person 2/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });

  it("keeps only favorites with the filter and says when there are none", async () => {
    renderPage("/people?favorite=true");

    expect(await screen.findByText("No favorite people yet")).toBeTruthy();
    expect(listRequests().at(-1)?.get("favorite")).toBe("true");
  });

  it("creates a person from the dialog and opens the new person's page", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: /Person 1/ });

    await user.click(screen.getByRole("button", { name: "New person" }));
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Mio{Enter}");

    expect(await screen.findByText("Person page")).toBeTruthy();
    const created = server.sent.find((request) => request.method === "POST");
    expect(created?.body).toEqual({ name: "Mio" });
  });
});
