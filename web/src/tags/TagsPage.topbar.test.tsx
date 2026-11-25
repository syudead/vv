import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, onTestFinished } from "vitest";

import {
  tag,
  server,
  install,
  probe,
  renderPage,
  filterButton,
  toggleFilter,
  kindsLink,
  rejectedLink,
  setUpTagsPageServer,
} from "../testing/tagsPage";
import { stubRowHeights } from "../testing/virtualRows";

setUpTagsPageServer();

describe("TagsPage トップバーと見出し", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  async function chooseSortKind(user: ReturnType<typeof userEvent.setup>, kind: string) {
    await user.click(screen.getByRole("button", { name: /^Sort by: / }));
    await user.click(await screen.findByRole("menuitemradio", { name: kind }));
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
      tag({ id: 3, name: "Cat", videoCount: 4 }),
      tag({ id: 4, name: "Gamma" }),
    ];
    probe.search = "";
  });

  it("検索・Filter・並び順は共通トップバーに、ライブラリと同じ並び（検索 → Filter → 並び順）で入る", async () => {
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Alpha");

    const topBar = screen.getByTestId("topbar");
    const search = within(topBar).getByRole("searchbox", { name: "Search tags" });
    expect(search.getAttribute("placeholder")).toBe("Search tags");
    const filter = within(topBar).getByRole("button", { name: "Filter" });
    const sort = within(topBar).getByRole("button", { name: "Sort by: Name" });
    expect(
      search.compareDocumentPosition(filter) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      filter.compareDocumentPosition(sort) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(within(topBar).queryByRole("button", { name: /Press for/ })).toBeNull();
    expect(screen.getAllByRole("searchbox")).toHaveLength(1);

    const user = userEvent.setup();
    await user.keyboard("/");
    expect(document.activeElement).toBe(search);
  });

  it("Filter の吹き出しは「Tentative only」「Unused only」のチェックを持ち、効いている数をボタンに添え、チップと「Clear filters」で外せる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(filterButton());
    const tentative = await screen.findByRole("checkbox", { name: /^Tentative only/ });
    const unused = screen.getByRole("checkbox", { name: /^Unused only/ });
    expect(tentative.getAttribute("aria-checked")).toBe("false");
    expect(unused.getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();

    await user.click(tentative);
    await user.click(unused);
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (2 applied)");
    expect(filterButton().querySelector('[data-slot="badge"]')?.className).toContain(
      "bg-primary-soft",
    );
    expect(filterButton().textContent).toContain("2");
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
    await user.keyboard("{Escape}");

    const chips = screen.getByRole("list", { name: "Active filters" });
    expect(
      within(chips)
        .getAllByRole("button")
        .map((chip) => chip.textContent),
    ).toEqual(["Tentative only", "Unused only"]);

    await user.click(
      within(chips).getByRole("button", { name: 'Remove the filter "Tentative only"' }),
    );
    expect(await screen.findByText("2 of 4 tags")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: 'Remove the filter "Unused only"' }),
      ),
    );
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (1 applied)");

    await user.click(filterButton());
    await user.click(await screen.findByRole("button", { name: "Clear filters" }));
    expect(await screen.findByText("4 tags")).toBeDefined();
    expect(screen.queryByRole("list", { name: "Active filters" })).toBeNull();
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
  });

  it("最後のチップを外すと「Filter」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Unused only");

    await user.click(
      screen.getByRole("button", { name: 'Remove the filter "Unused only"' }),
    );
    expect(await screen.findByText("4 tags")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
  });

  it("見出しは「Tags」で、右に件数と「New tag」を置く。絞ると「N of M tags」で、ページの区切りは出さない", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 150 }, (_, index) =>
      tag({ id: index + 1, name: `Tag ${String(index).padStart(3, "0")}` }),
    );
    // Supply row heights so virtualization renders only a realistic viewport in jsdom.
    onTestFinished(stubRowHeights());
    install();
    renderPage();
    await screen.findByText("150 tags");

    const heading = screen.getByRole("heading", { level: 1, name: "Tags" });
    const header = heading.closest("header")!;
    const status = within(header).getByRole("status");
    expect(server.pageRequests.at(-1)?.get("limit")).toBe("100");
    expect(status.textContent).toBe("150 tags");
    expect(within(header).getByRole("button", { name: "New tag" })).toBeDefined();
    expect(document.body.textContent).not.toMatch(/loaded/);

    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag 01");
    await waitFor(() => expect(status.textContent).toBe("10 of 150 tags"));
  });

  it("列の見出しは先頭のチェック・「Name」・右寄せの「Videos」で、表の見出しの行にある", async () => {
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const selectAll = screen.getByRole("checkbox", { name: "Select all 4 loaded tags" });
    const columns = selectAll.closest("thead")!;
    expect(
      within(columns)
        .getAllByRole("columnheader")
        .map((cell) => cell.textContent),
    ).toEqual(["", "Name", "Videos"]);
    const videos = within(columns).getByRole("columnheader", { name: "Videos" });
    expect(videos.className).toContain("text-right");
    expect(
      columns.compareDocumentPosition(rowOf("Alpha")) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("見出しの「Kinds」と「Rejected names」は種類と却下した名前のページへのリンクで、押すとそのページを開く", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    expect(kindsLink().getAttribute("href")).toBe("/tags/kinds");
    expect(rejectedLink().getAttribute("href")).toBe("/tags/rejected");
    expect(screen.queryByRole("tablist")).toBeNull();

    await user.click(rejectedLink());
    expect(await screen.findByRole("heading", { name: "Rejected names" })).toBeDefined();
    expect(probe.pathname).toBe("/tags/rejected");
    expect(
      await screen.findAllByRole("button", { name: /^Allow ".*" again$/ }),
    ).toHaveLength(2);

    await user.click(screen.getByRole("link", { name: "Tags" }));
    await screen.findByTitle("Alpha");
    await user.click(kindsLink());
    expect(await screen.findByRole("heading", { name: "Kinds" })).toBeDefined();
    expect(probe.pathname).toBe("/tags/kinds");
  });

  it("検索語・絞り込み・並び順は URL に載り、その URL で開き直すと同じ条件で読む", async () => {
    const user = userEvent.setup();
    install();
    const view = renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Tentative only");
    await chooseSortKind(user, "Video count");
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "a");
    await waitFor(() => expect(probe.search).toContain("q=a"));
    const params = new URLSearchParams(probe.search);
    expect(params.get("tentative")).toBe("1");
    expect(params.get("sort")).toBe("countDesc");
    view.unmount();

    server.pageRequests = [];
    renderPage({ url: `/tags${probe.search}` });
    await waitFor(() => expect(server.pageRequests.length).toBeGreaterThan(0));
    const sent = server.pageRequests.at(-1)!;
    expect(sent.get("q")).toBe("a");
    expect(sent.get("tentative")).toBe("true");
    expect(sent.get("sort")).toBe("countDesc");

    expect(
      (screen.getByRole("searchbox", { name: "Search tags" }) as HTMLInputElement).value,
    ).toBe("a");
    expect(screen.getByRole("button", { name: "Sort by: Video count" })).toBeDefined();
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (1 applied)");
    expect(
      screen.getByRole("button", { name: 'Remove the filter "Tentative only"' }),
    ).toBeDefined();
  });
});
