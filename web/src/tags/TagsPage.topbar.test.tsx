import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import {
  tag,
  server,
  hold,
  install,
  probe,
  historyBack,
  renderPage,
  filterButton,
  toggleFilter,
  rejectedTab,
  tagsTab,
  setUpTagsPageServer,
} from "../testing/tagsPage";

setUpTagsPageServer();

describe("TagsPage トップバー・見出し・タブ（specs/036-tag-admin-scale/ui-design.md）", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  /** chooseSortKind はトップバーの並び順のメニューから種類を選ぶ。 */
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
    // 「Name」に向きは無いので、向きのボタンは出さない。
    expect(within(topBar).queryByRole("button", { name: /Press for/ })).toBeNull();
    // 本文の中に操作の行は無い。
    expect(screen.getAllByRole("searchbox")).toHaveLength(1);

    // 「/」で検索欄へ移る。
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
    // 何も効いていない間は「Clear filters」を出さない。
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();

    await user.click(tentative);
    await user.click(unused);
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (2 applied)");
    // 効いている数は soft の Badge で添える。
    expect(filterButton().querySelector('[data-slot="badge"]')?.className).toContain(
      "bg-primary-soft",
    );
    expect(filterButton().textContent).toContain("2");
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
    await user.keyboard("{Escape}");

    // 見出しの下に、効いている絞り込みのチップが並ぶ。
    const chips = screen.getByRole("list", { name: "Active filters" });
    expect(
      within(chips)
        .getAllByRole("button")
        .map((chip) => chip.textContent),
    ).toEqual(["Tentative only", "Unused only"]);

    // チップの × でその絞り込みだけを外し、残るチップへフォーカスが移る。
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

    // 「Clear filters」で残りを外す。
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
    install();
    renderPage();
    await screen.findByText("150 tags");

    const heading = screen.getByRole("heading", { level: 1, name: "Tags" });
    const header = heading.closest("header")!;
    const status = within(header).getByRole("status");
    // 150 件のうち 100 件だけを読んだ状態でも、全部の数だけを出す。
    expect(server.pageRequests.at(-1)?.get("limit")).toBe("100");
    expect(status.textContent).toBe("150 tags");
    expect(within(header).getByRole("button", { name: "New tag" })).toBeDefined();
    expect(document.body.textContent).not.toMatch(/loaded/);

    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag 01");
    await waitFor(() => expect(status.textContent).toBe("10 of 150 tags"));
    // 150 件を描き、打鍵ごとに引き直すので、負荷の高い `task check` の中では既定の 5 秒を
    // 超えることがある。表明は弱めず、時間だけを仕事の量に合わせる。
  }, 20_000);

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
    // 列の見出しは最初の行より前にある。
    expect(
      columns.compareDocumentPosition(rowOf("Alpha")) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("タブで「Rejected names」へ移ると、ツールバーと見出しの件数・「New tag」を外し、本文に却下した名前を出す。URL に tab が載る", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Alpha");
    await waitFor(() => expect(rejectedTab().textContent).toBe("Rejected names2"));
    expect(tagsTab().textContent).toBe("Tags4");
    expect(screen.getByRole("tablist", { name: "Tag lists" })).toBeDefined();

    await user.click(rejectedTab());
    expect(rejectedTab().getAttribute("aria-selected")).toBe("true");
    expect(tagsTab().getAttribute("aria-selected")).toBe("false");
    expect(probe.search).toContain("tab=rejected");
    const panel = screen.getByRole("tabpanel", { name: /Rejected names/ });
    expect(
      within(panel).getAllByRole("button", { name: /^Allow ".*" again$/ }),
    ).toHaveLength(2);
    expect(within(panel).getAllByRole("row")[0]!.textContent).toContain("Allow again");
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(within(screen.getByTestId("topbar")).queryByRole("button")).toBeNull();
    expect(screen.queryByRole("button", { name: "New tag" })).toBeNull();
    expect(
      screen.queryByRole("link", { name: "Open the library filtered by Alpha" }),
    ).toBeNull();

    // 左右の矢印でタブの間を動く。
    rejectedTab().focus();
    await user.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(tagsTab());
    // 選ぶのはフォーカスの後の描画なので、重い環境でも待って確かめる。
    await waitFor(() => expect(tagsTab().getAttribute("aria-selected")).toBe("true"));
    expect(await screen.findByTitle("Alpha")).toBeDefined();
    expect(probe.search).not.toContain("tab=");
  });

  it("作成の送信中に戻るで「Rejected names」へ移っても「Tags」に留まり、失敗すれば下書きと誤りを残して URL を「Tags」へ戻す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(rejectedTab());
    await screen.findByRole("tabpanel", { name: /Rejected names/ });
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "Alpha");
    hold.nextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    await historyBack();
    expect(probe.search).toContain("tab=rejected");
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("textbox", { name: "New tag name" })).toBe(input);

    hold.release?.();
    expect(await screen.findByText('A tag named "Alpha" already exists.')).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("Alpha");
    await waitFor(() => expect(probe.search).not.toContain("tab="));
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(screen.queryByRole("tabpanel", { name: /Rejected names/ })).toBeNull();
  });

  it("改名の送信中に戻るで「Rejected names」へ移っても「Tags」に留まり、成功すれば「Rejected names」へ移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(rejectedTab());
    await screen.findByRole("tabpanel", { name: /Rejected names/ });
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "Alpha"' });
    await user.clear(input);
    await user.type(input, "Trip");
    hold.nextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    await historyBack();
    expect(probe.search).toContain("tab=rejected");
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("textbox", { name: 'New name for "Alpha"' })).toBe(input);

    hold.release?.();
    expect(await screen.findByRole("tabpanel", { name: /Rejected names/ })).toBeDefined();
    expect(rejectedTab().getAttribute("aria-selected")).toBe("true");
    expect(probe.search).toContain("tab=rejected");
  });

  it("作成の送信中は矢印でタブを切り替えず、フォーカスも今のタブに残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "新規");
    hold.nextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(
        screen.getByRole("textbox", { name: "New tag name" }).getAttribute("aria-busy"),
      ).toBe("true"),
    );

    tagsTab().focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(tagsTab());
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    await user.click(rejectedTab());
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    expect(probe.search).not.toContain("tab=");

    hold.release?.();
    await waitFor(() =>
      expect(screen.queryByRole("textbox", { name: "New tag name" })).toBeNull(),
    );
    tagsTab().focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(rejectedTab());
    await waitFor(() => expect(rejectedTab().getAttribute("aria-selected")).toBe("true"));
  });

  it("タブを移ると選択を解く", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    expect(screen.getByRole("region", { name: "Selected tags" })).toBeDefined();
    await user.click(rejectedTab());
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
  });

  it("検索語・絞り込み・並び順・タブは URL に載り、その URL で開き直すと同じ条件で読む", async () => {
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

    // 開き直す（再読み込み）。
    server.pageRequests = [];
    renderPage({ url: `/tags${probe.search}&tab=rejected` });
    expect(await screen.findByRole("tabpanel", { name: /Rejected names/ })).toBeDefined();
    await waitFor(() => expect(server.pageRequests.length).toBeGreaterThan(0));
    const sent = server.pageRequests.at(-1)!;
    expect(sent.get("q")).toBe("a");
    expect(sent.get("tentative")).toBe("true");
    expect(sent.get("sort")).toBe("countDesc");

    await user.click(tagsTab());
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
