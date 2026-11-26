import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ListPage } from "./list-page";
import { PageHeader } from "./page-header";

function filterRow(): HTMLElement {
  const element = document.querySelector<HTMLElement>('[data-slot="page-header-filter"]');
  if (element === null) throw new Error("no filter row");
  return element;
}

/** jsdom lays nothing out, so every element reports these widths. */
function stubWidths(scrollWidth: number, clientWidth: number) {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(scrollWidth);
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(clientWidth);
}

function scrollTo(element: HTMLElement, left: number) {
  Object.defineProperty(element, "scrollLeft", { configurable: true, value: left });
  fireEvent.scroll(element);
}

function renderListPage() {
  render(
    <ListPage
      header={
        <PageHeader title="Tags" actions={<button type="button">New tag</button>} />
      }
      filter={<div>chips</div>}
    >
      <p>body</p>
    </ListPage>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("PageHeader filter", () => {
  it("draws ListPage's filter on its own row after the actions", () => {
    stubWidths(300, 300);
    renderListPage();

    const actions = screen.getByRole("button", { name: "New tag" });
    expect(
      actions.compareDocumentPosition(filterRow()) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(filterRow().contains(actions)).toBe(false);
  });

  it("marks no side when every choice fits", () => {
    stubWidths(300, 300);
    renderListPage();

    expect(filterRow().hasAttribute("data-scroll-start")).toBe(false);
    expect(filterRow().hasAttribute("data-scroll-end")).toBe(false);
  });

  it("marks each side that hides choices as the row scrolls", () => {
    stubWidths(500, 300);
    renderListPage();
    const row = filterRow();

    expect(row.hasAttribute("data-scroll-start")).toBe(false);
    expect(row.hasAttribute("data-scroll-end")).toBe(true);

    scrollTo(row, 100);
    expect(row.hasAttribute("data-scroll-start")).toBe(true);
    expect(row.hasAttribute("data-scroll-end")).toBe(true);

    scrollTo(row, 200);
    expect(row.hasAttribute("data-scroll-start")).toBe(true);
    expect(row.hasAttribute("data-scroll-end")).toBe(false);
  });
});
