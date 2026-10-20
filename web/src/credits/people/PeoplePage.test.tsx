import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Person } from "../../api/credits";
import { ToastProvider } from "../../ui/Toast";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import PeoplePage from "./PeoplePage";
import { clearPeopleSnapshot } from "./peopleSnapshot";

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
    provisional: false,
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
/** The sentinel observes in a passive effect, which can run after the cards are on screen. */
let observing = false;

class FakeIntersectionObserver {
  private readonly callback: IntersectionObserverCallback;
  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
  }
  observe() {
    observing = true;
    reveal = () =>
      this.callback(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      );
  }
  disconnect() {
    observing = false;
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
    const provisional = url.searchParams.get("provisional");
    const items = page.items.filter(
      (item) =>
        (url.searchParams.get("favorite") !== "true" || item.favorite) &&
        (provisional === null || String(item.provisional) === provisional),
    );
    return jsonResponse({
      ...page,
      items,
      total: items.length,
      totalAll: server.totalAll,
    });
  }
  if (url.pathname.startsWith("/api/people/") && method === "PATCH") {
    const id = Number(url.pathname.split("/").at(-1));
    return jsonResponse(person(id, { name: (body as { name: string }).name }));
  }
  if (url.pathname === "/api/people" && method === "POST") {
    return jsonResponse(person(50, { name: (body as { name: string }).name }), 201);
  }
  return jsonResponse({ code: "not_found" }, 404);
}

beforeEach(() => {
  window.localStorage.clear();
  clearPeopleSnapshot();
  server.pages = new Map([[null, { items: [person(1), person(2)] }]]);
  server.totalAll = 2;
  server.sent = [];
  server.failSort = null;
  reveal = () => {};
  observing = false;
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

function PersonStub() {
  const navigate = useNavigate();
  return (
    <>
      <p>Person page</p>
      <button type="button" onClick={() => void navigate(-1)}>
        Back
      </button>
    </>
  );
}

function renderPage(url = "/people") {
  render(
    <TooltipProvider>
      <ToastProvider>
        <MemoryRouter initialEntries={[url]}>
          <Routes>
            <Route path="/people" element={<PeoplePage />} />
            <Route path="/people/:id" element={<PersonStub />} />
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
    // Without a face image the placeholder shows the name's initial.
    expect(within(link).getByText("A")).toBeTruthy();
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
    await waitFor(() => expect(observing).toBe(true));

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

  it("names a provisional person's card Unnamed person in place of the empty name", async () => {
    server.pages.set(null, {
      items: [person(3, { name: "", provisional: true, videoCount: 2 })],
    });
    renderPage();

    const link = await screen.findByRole("link", { name: /Unnamed person/ });
    expect(link.getAttribute("href")).toBe("/people/3");
    expect(within(link).getByText("2 videos")).toBeTruthy();
  });

  it("shows the applied Name filter above the grid and drops it with All", async () => {
    const user = userEvent.setup();
    renderPage("/people?named=0");

    expect(await screen.findByText("No unnamed people")).toBeTruthy();
    expect(listRequests().at(-1)?.get("provisional")).toBe("true");
    const names = screen.getByRole("radiogroup", { name: "Name" });
    expect(
      within(names).getByRole("radio", { name: "Unnamed" }).getAttribute("aria-checked"),
    ).toBe("true");

    await user.click(within(names).getByRole("radio", { name: "All" }));

    await screen.findByRole("link", { name: /Person 1/ });
    expect(listRequests().at(-1)?.has("provisional")).toBe(false);
  });

  it("keeps only favorites with the Favorites toggle and says when there are none", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("link", { name: /Person 1/ });

    await user.click(screen.getByRole("button", { name: "Favorites" }));

    expect(await screen.findByText("No favorite people yet")).toBeTruthy();
    expect(listRequests().at(-1)?.get("favorite")).toBe("true");
  });

  it("names an unnamed person from the card and shows the name in place", async () => {
    const user = userEvent.setup();
    server.pages.set(null, { items: [person(3, { name: "", provisional: true })] });
    renderPage();
    await screen.findByRole("link", { name: /Unnamed person/ });

    await user.click(screen.getByRole("button", { name: "Name…" }));
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Mio{Enter}");

    expect(await screen.findByRole("link", { name: /Mio/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Name…" })).toBeNull();
    const patch = server.sent.find((request) => request.method === "PATCH");
    expect(patch?.url.pathname).toBe("/api/people/3");
    expect(patch?.body).toEqual({ name: "Mio" });
    expect(listRequests()).toHaveLength(1);
  });

  it("keeps the chosen card size for the next visit", async () => {
    const user = userEvent.setup();
    window.localStorage.setItem("vv.people.zoom", "0");
    renderPage();
    const link = await screen.findByRole("link", { name: /Person 1/ });
    const grid = link.closest<HTMLElement>('[data-slot="card-grid"]')!;
    expect(grid.style.gridTemplateColumns).toContain("--spacing-card-face-sm");

    screen.getByRole("slider", { name: "Card size" }).focus();
    await user.keyboard("{ArrowRight}");

    expect(grid.style.gridTemplateColumns).toContain("--spacing-card-face)");
    expect(window.localStorage.getItem("vv.people.zoom")).toBe("1");
  });

  it("returns from a person page to the loaded pages without asking again", async () => {
    const user = userEvent.setup();
    server.pages.set(null, { items: [person(1)], nextCursor: "c1" });
    server.pages.set("c1", { items: [person(2)] });
    renderPage();
    await screen.findByRole("link", { name: /Person 1/ });
    await waitFor(() => expect(observing).toBe(true));
    act(() => reveal());
    await screen.findByRole("link", { name: /Person 2/ });

    await user.click(screen.getByRole("link", { name: /Person 2/ }));
    await user.click(await screen.findByRole("button", { name: "Back" }));

    expect(await screen.findByRole("link", { name: /Person 2/ })).toBeTruthy();
    expect(screen.getByRole("link", { name: /Person 1/ })).toBeTruthy();
    expect(listRequests()).toHaveLength(2);
  });

  it("asks for named people only when the URL keeps named=1", async () => {
    renderPage("/people?named=1");

    await screen.findByRole("link", { name: /Person 1/ });
    expect(listRequests().at(-1)?.get("provisional")).toBe("false");
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
