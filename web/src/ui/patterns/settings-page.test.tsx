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
  });

  it("shows the list alone below sm on the list address and the page under a back link on a page address", () => {
    render(<SettingsPageExample />);

    const nav = screen.getByRole("navigation", { name: p.settings });
    expect(slot("settings-page").className).toContain("max-w-4xl");
    expect(slot("settings-page").dataset.view).toBe("list");
    // 一覧の住所: sm 未満は本文を隠し（sm から並べる）、一覧は隠さない。
    expect(slot("settings-page-main").className).toMatch(/(^| )hidden( |$)/);
    expect(slot("settings-page-nav").className).not.toMatch(/(^| )hidden( |$)/);
    // sm からは最初のページを今のページとして出す。
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
    // 各ページの住所: sm 未満は一覧を隠して本文を出し、戻るリンクは sm から隠す。
    expect(slot("settings-page-nav").className).toMatch(/(^| )hidden( |$)/);
    expect(slot("settings-page-main").className).not.toMatch(/(^| )hidden( |$)/);
    expect(slot("settings-page-main").className).toContain(
      "sm:**:data-[slot=page-header-leading]:hidden",
    );

    const header = slot("page-header");
    fireEvent.click(within(header).getByRole("link", { name: p.settings }));

    expect(slot("settings-page").dataset.view).toBe("list");
  });
});
