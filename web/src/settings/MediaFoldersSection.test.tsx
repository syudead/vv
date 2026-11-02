import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFakeEventSource } from "../api/fakeEventSource";
import { json, renderWithScan } from "../testing/autoImportServer";
import { ToastProvider } from "../ui/Toast";
import MediaFoldersSection from "./MediaFoldersSection";

const GB = 1024 ** 3;

describe("MediaFoldersSection", () => {
  const switched: { url: string; body: unknown }[] = [];
  beforeEach(() => {
    switched.length = 0;
    installFakeEventSource();
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/enabled") && init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as { enabled: boolean };
        switched.push({ url, body });
        return Promise.resolve(
          json({
            id: 2,
            path: "/media/b",
            version: 1,
            createdAt: "",
            updatedAt: "",
            ...body,
          }),
        );
      }
      if (url === "/api/scans/current")
        return Promise.resolve(json({ code: "not_found", message: "none" }, 404));
      if (url === "/api/media-folders")
        return Promise.resolve(
          json([
            {
              id: 1,
              path: "/mnt/slow",
              version: 1,
              enabled: true,
              createdAt: "",
              updatedAt: "",
            },
            {
              id: 2,
              path: "/media/b",
              version: 1,
              enabled: true,
              createdAt: "",
              updatedAt: "",
            },
          ]),
        );
      // The first folder's volume never answers.
      if (url === "/api/media-folders/1/usage") return new Promise<Response>(() => {});
      if (url === "/api/media-folders/2/usage")
        return Promise.resolve(
          json({ videoBytes: GB, disk: { freeBytes: 3 * GB, totalBytes: 8 * GB } }),
        );
      return Promise.reject(new Error(`unexpected request: ${url}`));
    });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists every folder while each row's usage arrives on its own", async () => {
    renderWithScan(
      <ToastProvider>
        <MediaFoldersSection />
      </ToastProvider>,
    );

    expect(await screen.findByText("3.0 GB free")).toBeDefined();
    const slow = screen.getByText("/mnt/slow").parentElement!;
    expect(
      within(slow).getByRole("status", { name: "Checking disk space" }),
    ).toBeDefined();
    const changes = screen.getAllByRole("button", { name: "Change folder" });
    expect(changes.map((button) => (button as HTMLButtonElement).disabled)).toEqual([
      false,
      false,
    ]);
  });

  it("a row's switch saves that folder off and dims its path", async () => {
    renderWithScan(
      <ToastProvider>
        <MediaFoldersSection />
      </ToastProvider>,
    );
    const toggle = await screen.findByRole("switch", {
      name: "Show videos from /media/b",
    });
    expect(toggle.getAttribute("aria-checked")).toBe("true");

    await userEvent.setup().click(toggle);

    expect(switched).toEqual([
      { url: "/api/media-folders/2/enabled", body: { enabled: false } },
    ]);
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(toggle.closest("label")?.textContent).toBe("Off");
    expect(screen.getByText("/media/b").className).toContain("text-muted-foreground");
  });
});
