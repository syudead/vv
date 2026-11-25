import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Organization } from "../../api/credits";
import { jsonResponse } from "../../testing/personPage";
import { video } from "../../testing/groupPage";
import OrganizationPage from "./OrganizationPage";

function organization(overrides: Partial<Organization> = {}): Organization {
  return {
    id: 5,
    name: "Studio",
    names: [{ name: "Studio" }],
    externalIds: [],
    profile: {},
    favorite: false,
    videoCount: 1,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const server = {
  organization: null as Organization | null,
  children: [] as Organization[],
  /** Videos per role; the read without a role answers every one of them. */
  videos: { maker: [] as number[], label: [] as number[] },
  /** Roles whose read fails. */
  failing: [] as string[],
  /** Holds the next read of this role until `release` answers it. */
  held: null as string | null,
  release: () => {},
  requests: [] as URL[],
};

function respond(url: URL): Response {
  if (url.pathname === "/api/videos") {
    const role = url.searchParams.get("organizationRole");
    if (role !== null && server.failing.includes(role)) {
      return jsonResponse({ code: "internal", message: "Down" }, 500);
    }
    const ids =
      role === "maker" || role === "label"
        ? server.videos[role]
        : [...server.videos.maker, ...server.videos.label];
    return jsonResponse({ items: ids.map((id) => video(id)), total: ids.length });
  }
  if (server.organization === null) {
    return jsonResponse({ code: "not_found", reason: "organization_not_found" }, 404);
  }
  if (url.pathname === "/api/organizations/5") return jsonResponse(server.organization);
  if (url.pathname === "/api/organizations/5/children") {
    return jsonResponse({
      items: server.children,
      total: server.children.length,
      totalAll: server.children.length,
    });
  }
  return jsonResponse({ code: "not_found" }, 404);
}

beforeEach(() => {
  server.organization = organization();
  server.children = [];
  server.videos = { maker: [1], label: [] };
  server.failing = [];
  server.held = null;
  server.requests = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input) => {
      const url = new URL(String(input), "http://localhost");
      server.requests.push(url);
      if (
        server.held !== null &&
        url.searchParams.get("organizationRole") === server.held
      ) {
        server.held = null;
        return new Promise((resolve) => {
          server.release = () => resolve(respond(url));
        });
      }
      return Promise.resolve(respond(url));
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

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/organizations/5"]}>
      <Routes>
        <Route path="/organizations/:id" element={<OrganizationPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

function href(name: string): string | null {
  return screen.getByRole("link", { name }).getAttribute("href");
}

describe("OrganizationPage", () => {
  it("shows the names, profile, external IDs and the logo placeholder, and links to the library", async () => {
    server.organization = organization({
      names: [{ name: "Studio" }, { name: "スタジオ" }],
      profile: {
        foundedDate: "1990-05",
        location: "Tokyo",
        bio: "Makes films.",
        links: ["https://studio.example/about"],
      },
      externalIds: [{ source: "fanza", value: "s-1" }],
    });
    renderPage();

    expect(await screen.findByRole("heading", { name: "Studio", level: 1 })).toBeTruthy();
    expect(screen.getByText("Also known as スタジオ")).toBeTruthy();
    expect(screen.getByText("Makes films.")).toBeTruthy();
    expect(screen.getByText("1990-05")).toBeTruthy();
    expect(screen.getByText("Tokyo")).toBeTruthy();
    expect(screen.getByText("studio.example")).toBeTruthy();
    expect(screen.getByText("fanza: s-1")).toBeTruthy();
    expect(document.querySelector('[data-slot="organization-logo"] img')).toBeNull();
    expect(href("Show in library")).toBe("/?organization=5");
    expect(href("Edit")).toBe("/organizations/5/edit");
    expect(document.title).toContain("Studio");
  });

  it("links the parent and every child to their pages, folding children past twelve", async () => {
    const user = userEvent.setup();
    server.organization = organization({
      parent: { id: 2, name: "Group", provisional: false },
    });
    server.children = Array.from({ length: 13 }, (_, index) =>
      organization({ id: 10 + index, name: `Child ${String(index + 1)}` }),
    );
    renderPage();

    const heading = await screen.findByRole("heading", { name: "Related organizations" });
    const family = heading.closest("section") as HTMLElement;
    expect(
      within(family).getByRole("link", { name: "Open Group" }).getAttribute("href"),
    ).toBe("/organizations/2");
    expect(
      await within(family).findByRole("link", { name: "Open Child 1" }),
    ).toBeTruthy();
    expect(within(family).queryByRole("link", { name: "Open Child 13" })).toBeNull();

    await user.click(within(family).getByRole("button", { name: /Show all 13/ }));

    expect(
      within(family).getByRole("link", { name: "Open Child 13" }).getAttribute("href"),
    ).toBe("/organizations/22");
  });

  it("leaves out the family shelf for an organization with neither parent nor children", async () => {
    renderPage();
    await screen.findByRole("link", { name: /Video 1/ });

    expect(screen.queryByRole("heading", { name: "Related organizations" })).toBeNull();
  });

  it("shows a shelf for each role with videos, read by organization role, and none for an empty role", async () => {
    server.videos = { maker: [1, 2], label: [] };
    renderPage();

    expect(await screen.findByRole("heading", { name: "Maker", level: 2 })).toBeTruthy();
    expect(await screen.findByRole("link", { name: /Video 2/ })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Label", level: 2 })).toBeNull();
    const reads = server.requests.filter((url) => url.pathname === "/api/videos");
    expect(reads.map((url) => url.searchParams.get("organization"))).not.toContain(null);
    expect(reads.map((url) => url.searchParams.get("organizationRole"))).toEqual(
      expect.arrayContaining([null, "maker", "label"]),
    );
  });

  it("keeps a role's shelf with Retry when its first read fails", async () => {
    server.videos = { maker: [1], label: [2] };
    server.failing = ["label"];
    renderPage();

    const failed = await screen.findByText(/Couldn't load videos/);
    expect(screen.getByRole("heading", { name: "Label", level: 2 })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Maker", level: 2 })).toBeTruthy();
    expect(failed.closest("section")?.querySelector("button")?.textContent).toBe("Retry");
  });

  it("keeps a failed role's heading while its retry is read, then shows the cards", async () => {
    server.videos = { maker: [1], label: [2] };
    server.failing = ["label"];
    renderPage();
    const failed = await screen.findByText(/Couldn't load videos/);

    server.failing = [];
    server.held = "label";
    await userEvent.click(
      within(failed.closest("section")!).getByRole("button", { name: "Retry" }),
    );

    const label = screen
      .getByRole("heading", { name: "Label", level: 2 })
      .closest("section")!;
    expect(within(label).getByRole("status")).toBeTruthy();
    server.release();
    expect(await within(label).findByText("Video 2")).toBeTruthy();
    expect(within(label).queryByRole("status")).toBeNull();
  });

  it("shows one empty state without shelves when the organization has no videos", async () => {
    server.videos = { maker: [], label: [] };
    renderPage();

    expect(await screen.findByText("No videos yet")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Studio", level: 1 })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Maker" })).toBeNull();
  });

  it("shows the gone state when the organization no longer exists", async () => {
    server.organization = null;
    renderPage();

    expect(await screen.findByText("This organization no longer exists")).toBeTruthy();
    expect(href("Back to organizations")).toBe("/organizations");
  });
});
