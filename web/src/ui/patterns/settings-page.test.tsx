import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SettingsPageExample } from "../../designSystem/blocks/settings-page-example";
import { t } from "../../i18n";
import { PageHeader } from "./page-header";
import { SettingsPage } from "./settings-page";

const p = t.designSystem.pattern;

function slot(name: string): HTMLElement {
  const element = document.querySelector<HTMLElement>(`[data-slot="${name}"]`);
  if (element === null) throw new Error(`no ${name}`);
  return element;
}

describe("SettingsPage", () => {
  it("keeps one column at max-w-3xl without a nav", () => {
    render(
      <SettingsPage header={<PageHeader title="Settings" />}>
        <p>rows</p>
      </SettingsPage>,
    );

    expect(slot("settings-page").className).toContain("max-w-3xl");
    expect(document.querySelector('[data-slot="settings-page-nav"]')).toBeNull();
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(document.querySelector('[data-slot="settings-page-notice"]')).toBeNull();
  });

  it("shows the list alone below sm on the list address and the page under a back link on a page address", () => {
    render(<SettingsPageExample />);

    const nav = screen.getByRole("navigation", { name: p.settings });
    expect(slot("settings-page").className).toContain("max-w-4xl");
    expect(slot("settings-page").dataset.view).toBe("list");
    expect(slot("settings-page-main").className).toMatch(/(^| )hidden( |$)/);
    expect(slot("settings-page-nav").className).not.toMatch(/(^| )hidden( |$)/);
    // Scroll tall sticky navigation internally so every entry remains reachable.
    expect(slot("settings-page-nav").className).toContain("sm:max-h-settings-nav-max");
    expect(slot("settings-page-nav").className).toContain("sm:overflow-y-auto");
    const library = within(nav).getByRole("link", { name: p.settingsPages.library });
    expect(library.getAttribute("aria-current")).toBe("page");
    expect(library.className).toContain("sm:data-current:bg-secondary");
    expect(library.className).not.toMatch(/(^| )bg-secondary/);

    fireEvent.click(within(nav).getByRole("link", { name: p.settingsPages.server }));

    expect(slot("settings-page").dataset.view).toBe("page");
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(
      p.settingsPages.server,
    );
    expect(
      within(nav)
        .getByRole("link", { name: p.settingsPages.server })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(
      within(nav)
        .getByRole("link", { name: p.settingsPages.library })
        .hasAttribute("aria-current"),
    ).toBe(false);
    expect(slot("settings-page-nav").className).toMatch(/(^| )hidden( |$)/);
    expect(slot("settings-page-main").className).not.toMatch(/(^| )hidden( |$)/);
    expect(slot("settings-page-main").className).toContain(
      "sm:**:data-[slot=page-header-leading]:hidden",
    );

    const notice = slot("settings-page-notice");
    expect(notice.textContent).toBe(p.serverNotice);
    expect(
      notice.compareDocumentPosition(slot("settings-page-body")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      slot("page-header").compareDocumentPosition(notice) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    const header = slot("page-header");
    fireEvent.click(within(header).getByRole("link", { name: p.settings }));

    expect(slot("settings-page").dataset.view).toBe("list");
    expect(document.querySelector('[data-slot="settings-page-notice"]')).toBeNull();
  });
});
