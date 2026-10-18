import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { installFakeEventSource } from "../api/fakeEventSource";
import { clearListSnapshot } from "../api/listSnapshot";
import { AudienceProvider } from "../auth/audience";
import LibraryPage from "../library/LibraryPage";
import { ScanProvider } from "../shell/ScanProvider";
import { json, video } from "../testing/groupPage";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";

function Location() {
  const location = useLocation();
  return (
    <output aria-label="Current URL">
      {location.pathname}
      {location.search}
    </output>
  );
}

beforeEach(() => {
  clearListSnapshot();
  installFakeEventSource();
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      private readonly callback: IntersectionObserverCallback;
      constructor(callback: IntersectionObserverCallback) {
        this.callback = callback;
      }
      observe(element: HTMLElement) {
        if (element.dataset.slot === "shelf-row") {
          this.callback(
            [
              {
                isIntersecting: true,
                target: element,
              } as unknown as IntersectionObserverEntry,
            ],
            this as unknown as IntersectionObserver,
          );
        }
      }
      disconnect() {}
    },
  );
});
afterEach(() => vi.unstubAllGlobals());

it("keeps shelves above the library while search updates the same page", async () => {
  const requests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>((input) => {
      const url = new URL(String(input), "http://localhost");
      requests.push(url.pathname + url.search);
      if (url.pathname === "/api/home")
        return Promise.resolve(
          json({ shelves: [{ key: "folder:3", name: "Travel", folderId: 3 }] }),
        );
      if (url.pathname === "/api/home/shelves")
        return Promise.resolve(
          json({
            shelves: [
              {
                key: "folder:3",
                items: [{ kind: "video", video: video(1, { title: "Harbour" }) }],
                total: 1,
              },
            ],
          }),
        );
      if (url.pathname === "/api/library") {
        const filtered = url.searchParams.get("query") === "Ridge";
        return Promise.resolve(
          json({
            items: (filtered
              ? [video(2, { title: "Ridge" })]
              : [video(1, { title: "Harbour" }), video(2, { title: "Ridge" })]
            ).map((item) => ({ kind: "video", video: item })),
            total: filtered ? 1 : 2,
          }),
        );
      }
      if (url.pathname === "/api/scans/current") return Promise.resolve(json({}, 404));
      if (url.pathname === "/api/smart-folders")
        return Promise.resolve(json({ items: [] }));
      throw new Error(`unexpected request: ${url.toString()}`);
    }),
  );
  render(
    <MemoryRouter>
      <TooltipProvider>
        <ToastProvider>
          <AudienceProvider audience="owner">
            <ScanProvider>
              <LibraryPage />
              <Location />
            </ScanProvider>
          </AudienceProvider>
        </ToastProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
  const shelves = await screen.findByRole("region", { name: "Shelves" });
  const library = screen.getByRole("region", { name: "Library" });
  expect(await within(shelves).findByRole("link", { name: "Harbour" })).toBeTruthy();
  expect(await within(library).findByRole("link", { name: "Harbour" })).toBeTruthy();
  expect(
    shelves.compareDocumentPosition(library) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0);
  expect(screen.getAllByRole("searchbox")).toHaveLength(1);
  expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);

  const user = userEvent.setup();
  await user.type(
    screen.getByRole("searchbox", { name: "Search videos" }),
    "Ridge{Enter}",
  );
  await waitFor(() =>
    expect(within(library).queryByRole("link", { name: "Harbour" })).toBeNull(),
  );
  expect(await within(library).findByRole("link", { name: "Ridge" })).toBeTruthy();
  expect(within(shelves).getByRole("link", { name: "Harbour" })).toBeTruthy();
  expect(screen.getByRole("status", { name: "Current URL" }).textContent).toBe(
    "/?q=Ridge&sort=addedDesc",
  );
  expect(requests.filter((path) => path === "/api/home")).toHaveLength(1);
});
