import { screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFakeEventSource } from "../api/fakeEventSource";
import { json, renderWithScan } from "../testing/autoImportServer";
import { ToastProvider } from "../ui/Toast";
import MediaFoldersSection from "./MediaFoldersSection";

const GB = 1024 ** 3;

describe("MediaFoldersSection", () => {
  beforeEach(() => {
    installFakeEventSource();
    vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/scans/current")
        return Promise.resolve(json({ code: "not_found", message: "none" }, 404));
      if (url === "/api/media-folders")
        return Promise.resolve(
          json([
            { id: 1, path: "/mnt/slow", version: 1, createdAt: "", updatedAt: "" },
            { id: 2, path: "/media/b", version: 1, createdAt: "", updatedAt: "" },
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
});
