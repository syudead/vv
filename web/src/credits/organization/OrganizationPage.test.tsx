import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Organization } from "../../api/credits";
import { jsonResponse } from "../../testing/personPage";
import { video } from "../../testing/groupPage";
import { ToastProvider } from "../../ui/Toast";
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
  /** Requests other than reads, with their bodies. */
  writes: [] as { method: string; url: URL; body: BodyInit | null | undefined }[],
  /** Other organizations, offered by the merge dialog's search. */
  others: [] as Organization[],
  /** The reason a logo upload fails with, or null to accept it. */
  logoFailure: null as string | null,
  /** Who is gone when the merge answers: the target alone, or both organizations. */
  mergeMissing: null as "target" | "both" | null,
};

/** Answers the page's writes as the server does; null for a read. */
function respondToWrite(method: string, url: URL): Response | null {
  if (method === "PUT" && url.pathname === "/api/favorites") {
    return jsonResponse({ videoIds: [], groupIds: [] });
  }
  if (method === "PUT" && url.pathname === "/api/organizations/5/image") {
    if (server.logoFailure !== null) {
      return jsonResponse({ code: "invalid", message: server.logoFailure }, 400);
    }
    return jsonResponse({ ...server.organization, imageRevision: 3 });
  }
  if (method === "POST" && url.pathname === "/api/organizations/8/merge") {
    if (server.mergeMissing !== null) {
      if (server.mergeMissing === "both") server.organization = null;
      return jsonResponse({ code: "not_found", reason: "organization_not_found" }, 404);
    }
    return jsonResponse({ organization: server.others[0], notFoundIds: [] });
  }
  if (method === "DELETE" && url.pathname === "/api/organizations/5/image") {
    return jsonResponse({ code: "invalid", message: "Disk full" }, 400);
  }
  if (method === "DELETE" && url.pathname === "/api/organizations/5") {
    return new Response(null, { status: 204 });
  }
  return null;
}

function respond(url: URL): Response {
  if (url.pathname === "/api/organizations") {
    return jsonResponse({
      items: server.others,
      total: server.others.length,
      totalAll: server.others.length,
    });
  }
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
  const other = server.others.find(
    (item) => url.pathname === `/api/organizations/${item.id}`,
  );
  if (other !== undefined) return jsonResponse(other);
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
  server.writes = [];
  server.others = [];
  server.logoFailure = null;
  server.mergeMissing = null;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      const method = init?.method ?? "GET";
      if (method !== "GET") {
        server.writes.push({ method, url, body: init?.body });
        const answer = respondToWrite(method, url);
        if (answer !== null) return Promise.resolve(answer);
      }
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

function Location() {
  const location = useLocation();
  return <output aria-label="location">{location.pathname}</output>;
}

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/organizations/5"]}>
        <Routes>
          <Route path="/organizations/:id" element={<OrganizationPage />} />
          <Route path="/organizations" element={<p>Organizations list</p>} />
        </Routes>
        <Location />
      </MemoryRouter>
    </ToastProvider>,
  );
}

function location(): string | null {
  return screen.getByRole("status", { name: "location" }).textContent;
}

/** Opens `More actions` and picks the item. */
async function chooseMore(user: ReturnType<typeof userEvent.setup>, item: string) {
  await user.click(await screen.findByRole("button", { name: "More actions" }));
  await user.click(await screen.findByRole("menuitem", { name: item }));
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

  it("sends the organization's favorite and shows it pressed after the response", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Add to favorites" }));

    const pressed = await screen.findByRole("button", { name: "Remove from favorites" });
    expect(pressed.getAttribute("aria-pressed")).toBe("true");
    const favorite = server.writes.find(
      (write) => write.url.pathname === "/api/favorites",
    );
    expect(JSON.parse(String(favorite?.body))).toEqual({
      videoIds: [],
      groupIds: [],
      organizationIds: [5],
      favorite: true,
    });
  });

  it("saves an uploaded logo at once and shows it in place of the placeholder", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Add logo" }));
    const dialog = await screen.findByRole("dialog", { name: "Upload a logo" });
    const file = new File(["png"], "logo.png", { type: "image/png" });
    await user.upload(within(dialog).getByTestId("image-upload-input"), file);

    const logo = await screen.findByRole("img", { name: "Logo of Studio" });
    expect(logo.getAttribute("src")).toBe("/api/organizations/5/image?v=3");
    const upload = server.writes.find((write) => write.method === "PUT");
    expect(upload?.url.pathname).toBe("/api/organizations/5/image");
    expect(((upload?.body as FormData).get("image") as File).name).toBe("logo.png");
    expect(screen.queryByRole("button", { name: "Add logo" })).toBeNull();
  });

  it("keeps the previous logo and says why under it when a logo change fails", async () => {
    const user = userEvent.setup();
    server.organization = organization({ imageRevision: 1 });
    server.logoFailure = "Bad image";
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Change logo" }));
    await user.click(await screen.findByRole("menuitem", { name: "Upload an image…" }));
    const dialog = await screen.findByRole("dialog", { name: "Upload a logo" });
    await user.upload(
      within(dialog).getByTestId("image-upload-input"),
      new File(["png"], "logo.png", { type: "image/png" }),
    );

    expect(await screen.findByText("Couldn't change the logo: Bad image")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Logo of Studio" }).getAttribute("src")).toBe(
      "/api/organizations/5/image?v=1",
    );
  });

  it("closes the removal confirmation and says why under the logo when removing fails", async () => {
    const user = userEvent.setup();
    server.organization = organization({ imageRevision: 1 });
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Change logo" }));
    await user.click(await screen.findByRole("menuitem", { name: "Remove logo" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: 'Remove the logo of "Studio"?',
    });
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));

    expect(await screen.findByText("Couldn't change the logo: Disk full")).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByRole("img", { name: "Logo of Studio" }).getAttribute("src")).toBe(
      "/api/organizations/5/image?v=1",
    );
  });

  it("opens the target's page after merging into it", async () => {
    const user = userEvent.setup();
    server.others = [organization({ id: 8, name: "Agency" })];
    renderPage();
    await chooseMore(user, "Merge into another organization…");
    const dialog = await screen.findByRole("dialog", { name: 'Merge "Studio"' });
    await user.click(await within(dialog).findByRole("option", { name: /Agency/ }));
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    await waitFor(() => expect(location()).toBe("/organizations/8"));
    expect(await screen.findByText('Merged into "Agency"')).toBeTruthy();
    expect(await screen.findByRole("heading", { name: "Agency", level: 1 })).toBeTruthy();
    const merge = server.writes.find((write) => write.method === "POST");
    expect(JSON.parse(String(merge?.body))).toEqual({ sourceIds: [5] });
  });

  /** Picks Agency in the merge dialog and merges into it. */
  async function mergeIntoAgency(user: ReturnType<typeof userEvent.setup>) {
    server.others = [organization({ id: 8, name: "Agency" })];
    renderPage();
    await chooseMore(user, "Merge into another organization…");
    const dialog = await screen.findByRole("dialog", { name: 'Merge "Studio"' });
    await user.click(await within(dialog).findByRole("option", { name: /Agency/ }));
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    return dialog;
  }

  it("keeps the page and says why in the dialog when the merge target is gone", async () => {
    const user = userEvent.setup();
    server.mergeMissing = "target";
    const dialog = await mergeIntoAgency(user);

    expect(
      await within(dialog).findByText("This organization no longer exists."),
    ).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Merge" })).toHaveProperty(
      "disabled",
      true,
    );
    // The open dialog hides the page from the accessibility tree.
    expect(
      screen.getByRole("heading", { name: "Studio", level: 1, hidden: true }),
    ).toBeTruthy();
  });

  it("turns into the gone state when the merge finds the organization itself gone", async () => {
    const user = userEvent.setup();
    server.mergeMissing = "both";
    await mergeIntoAgency(user);

    expect(await screen.findByText("This organization no longer exists")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens the organizations list after deleting", async () => {
    const user = userEvent.setup();
    renderPage();
    await chooseMore(user, "Delete");
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Studio"?' });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Organizations list")).toBeTruthy();
    expect(location()).toBe("/organizations");
    expect(screen.getByText('Deleted "Studio"')).toBeTruthy();
    expect(server.writes.map((write) => write.method)).toEqual(["DELETE"]);
  });
});
