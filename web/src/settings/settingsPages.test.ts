import { describe, expect, it } from "vitest";

import { t } from "../i18n";
import { pageForAnchor, settingsPages, settingsRoute } from "./settingsPages";

describe("settingsRoute", () => {
  it("shows the list, with the Scan page, at /settings", () => {
    const route = settingsRoute("/settings", "");
    expect(route.kind).toBe("list");
    expect(route.kind === "list" && route.page.slug).toBe("scan");
  });

  it("opens the page a known slug names, ignoring case and a trailing slash", () => {
    const route = settingsRoute("/Settings/Connections/", "");
    expect(route.kind === "page" && route.page.slug).toBe("connections");
  });

  it("replaces an unknown slug with the Scan page", () => {
    expect(settingsRoute("/settings/nowhere", "")).toEqual({
      kind: "replace",
      to: "/settings/scan",
    });
    expect(settingsRoute("/settings/scan/extra", "#comfyui")).toEqual({
      kind: "replace",
      to: "/settings/scan",
    });
  });

  it("forwards a known anchor on /settings to its page, keeping the anchor", () => {
    expect(settingsRoute("/settings", "#comfyui")).toEqual({
      kind: "replace",
      to: "/settings/connections#comfyui",
    });
    expect(settingsRoute("/settings", "#scan-status")).toEqual({
      kind: "replace",
      to: "/settings/scan#scan-status",
    });
    expect(settingsRoute("/settings", "#work-info-plugins")).toEqual({
      kind: "replace",
      to: "/settings/plugins#work-info-plugins",
    });
  });

  it("treats /settings/ as the list", () => {
    expect(settingsRoute("/settings/", "").kind).toBe("list");
  });

  it("keeps an unknown anchor on /settings as the list", () => {
    expect(settingsRoute("/settings", "#nowhere").kind).toBe("list");
  });
});

describe("pageForAnchor", () => {
  it("maps #comfyui to the Connections page", () => {
    expect(pageForAnchor("#comfyui")?.slug).toBe("connections");
    expect(pageForAnchor("")).toBeUndefined();
  });
});

describe("settingsPages", () => {
  it("lists the pages in order, each title in the catalog, no section twice", () => {
    expect(settingsPages.map((page) => page.slug)).toEqual([
      "scan",
      "library",
      "auto-tagging",
      "face-recognition",
      "connections",
      "plugins",
      "conversion",
      "network",
      "pairing",
      "account",
      "api",
      "danger-zone",
    ]);
    expect(Object.keys(t.settings.pages)).toEqual(settingsPages.map((page) => page.slug));
    const sections = settingsPages.flatMap((page) => page.sections);
    expect(new Set(sections).size).toBe(sections.length);
  });
});
