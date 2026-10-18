import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import {
  autoImportSettings as settings,
  autoImportURL,
  installAutoImportServer,
  renderWithScan,
} from "../testing/autoImportServer";
import { AutoImportStateSection, AutoImportSwitchSection } from "./AutoImportSection";

// State and saving contracts have dedicated tests; this file checks Scan and Library section composition.

describe("AutoImportStateSection", () => {
  const server = installAutoImportServer();

  it("shows the watch state under the Auto-import heading, with no switch", async () => {
    server.current = settings(false);
    renderWithScan(<AutoImportStateSection />);

    const heading = await screen.findByRole("heading", { level: 2, name: "Auto-import" });
    const link = await screen.findByRole("link", { name: "Library" });
    expect(link.getAttribute("href")).toBe("/settings/library");
    expect(link.closest("p")?.textContent).toBe(
      "Off. Changes are picked up by the next scan. Turn it on in Library.",
    );
    expect(heading.closest("section")?.contains(link)).toBe(true);
    expect(screen.queryByRole("switch")).toBeNull();
  });
});

describe("AutoImportSwitchSection", () => {
  const server = installAutoImportServer();

  it("describes the switch with a link to the Scan page and saves it", async () => {
    server.current = settings(false);
    const user = userEvent.setup();
    renderWithScan(<AutoImportSwitchSection />);

    const toggle = await screen.findByRole("switch", {
      name: "Pick up changes as they happen",
    });
    const link = screen.getByRole("link", { name: "Scan" });
    expect(link.getAttribute("href")).toBe("/settings/scan");
    expect(link.closest("[data-slot='form-row']")?.contains(toggle)).toBe(true);

    await user.click(toggle);
    expect(server.fetchMock).toHaveBeenCalledWith(
      autoImportURL,
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ enabled: true }) }),
    );
    expect(screen.getByRole("link", { name: "Scan" })).toBeDefined();
  });
});
