import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  tag,
  server,
  heldGetReleases,
  hold,
  install,
  renderPage,
  filterButton,
  toggleFilter,
  filterChecked,
  setUpTagsPageServer,
} from "../testing/tagsPage";
import { stubRowHeights } from "../testing/virtualRows";

setUpTagsPageServer();

describe("TagsPage 見えている行だけ描く", () => {
  /** Supply dimensions because jsdom has no viewport or element layout. */
  const rowHeight = 40;
  const viewportHeight = 200;
  const tagCount = 60;
  const bandHeight = 50;
  let restoreHeights = () => {};

  function name(index: number): string {
    return `Tag ${String(index).padStart(2, "0")}`;
  }

  function drawnIndexes(): number[] {
    return [...document.querySelectorAll<HTMLElement>("[data-index]")].map((row) =>
      Number(row.dataset.index),
    );
  }

  function wrapperOf(index: number): HTMLElement {
    return document.querySelector<HTMLElement>(`[data-index="${String(index)}"]`)!;
  }

  beforeEach(() => {
    server.tags = Array.from({ length: tagCount }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: true }),
    );
    restoreHeights = stubRowHeights({
      rowHeight,
      viewportHeight,
      otherHeight: (element) => (element.classList.contains("sticky") ? bandHeight : 0),
    });
  });

  afterEach(() => {
    restoreHeights();
  });

  async function tabPastRange(user: ReturnType<typeof userEvent.setup>) {
    const last = Math.max(...drawnIndexes());
    expect(last).toBeLessThan(tagCount - 1);
    const more = within(wrapperOf(last)).getByRole("button", { name: "More actions" });
    more.focus();
    await user.tab();
    return last + 1;
  }

  it("全件ではなく表示域と前後の行だけを描き、件数は全件を数える", async () => {
    install();
    renderPage();
    await screen.findByTitle(name(0));
    expect(screen.getByText(`${String(tagCount)} tags`)).toBeDefined();
    const drawn = drawnIndexes();
    expect(drawn.length).toBeGreaterThan(0);
    expect(drawn.length).toBeLessThan(tagCount);
    expect(screen.queryByTitle(name(tagCount - 1))).toBeNull();
  });

  it("描いている範囲の端の Tab は次の行へ進み、Shift+Tab は前の行の最後の操作へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    const next = await tabPastRange(user);
    expect(document.activeElement).toBe(
      screen.getByRole("checkbox", { name: `Select "${name(next)}"` }),
    );

    within(wrapperOf(next)).getByRole("button", { name: "More actions" }).focus();
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("checkbox", { name: `Select "${name(next + 1)}"` }),
    );
    expect(document.querySelector(`[data-index="${String(next)}"]`)).toBeNull();

    await user.tab({ shift: true });
    expect(document.activeElement).toBe(
      within(wrapperOf(next)).getByRole("button", { name: "More actions" }),
    );
  });

  it("フォーカスが一覧の外へ出ると、表示域の外の行を描き続けるのをやめる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    const next = await tabPastRange(user);

    await user.tab();
    expect(document.activeElement?.closest("[data-index]")).toBe(wrapperOf(next));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.querySelector(`[data-index="${String(next)}"]`)).not.toBeNull();

    await user.click(screen.getByRole("searchbox", { name: "Search tags" }));
    await waitFor(() =>
      expect(document.querySelector(`[data-index="${String(next)}"]`)).toBeNull(),
    );
  });

  it("作ったタグが描いている範囲の外に並ぶときも、その行を描いて名前へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "Tag 99");
    await user.keyboard("{Enter}");

    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Open the library filtered by Tag 99" }),
      ),
    );
  });

  it("並び順で改名中の行が描いている範囲の外へ移っても、その行を描き続け打っている途中の名前を失わない", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item, index) => ({
      ...item,
      videoCount: index === 0 ? 0 : 1,
    }));
    install();
    renderPage();
    await screen.findByTitle(name(0));

    const row = screen.getByTitle(name(0)).closest("[data-tag-id]")!;
    await user.click(within(row as HTMLElement).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", {
      name: `New name for "${name(0)}"`,
    });
    await user.clear(input);
    await user.type(input, "下書き");

    await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
    expect(await screen.findByTitle(name(1))).toBeDefined();
    await waitFor(() => expect(wrapperOf(tagCount - 1)).not.toBeNull());
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(drawnIndexes()).toContain(tagCount - 1);
    expect(drawnIndexes()).not.toContain(tagCount - 2);
    const still = screen.getByRole("textbox", { name: `New name for "${name(0)}"` });
    expect(still).toBe(input);
    expect((still as HTMLInputElement).value).toBe("下書き");
  });

  describe("スクロールした位置から", () => {
    const scrolled = 1000;

    function scrollWindow() {
      // Give the document height so scrolling is not clamped.
      Object.defineProperty(document.documentElement, "scrollHeight", {
        configurable: true,
        value: 100_000,
      });
      Object.defineProperty(window, "scrollY", { configurable: true, value: scrolled });
      act(() => {
        window.dispatchEvent(new Event("scroll"));
      });
    }

    afterEach(() => {
      Object.defineProperty(window, "scrollY", { configurable: true, value: 0 });
      Reflect.deleteProperty(document.documentElement, "scrollHeight");
    });

    it("描いていない前の行へ戻るときは、留めた帯の高さを差し引いてスクロールする", async () => {
      const user = userEvent.setup();
      install();
      renderPage();
      await screen.findByTitle(name(0));
      scrollWindow();
      await waitFor(() => expect(Math.min(...drawnIndexes())).toBeGreaterThan(0));

      const first = Math.min(...drawnIndexes());
      screen.getByRole("checkbox", { name: `Select "${name(first)}"` }).focus();
      const scrollTo = vi.mocked(window.scrollTo);
      scrollTo.mockClear();
      await user.tab({ shift: true });

      expect(document.activeElement).toBe(
        within(wrapperOf(first - 1)).getByRole("button", { name: "More actions" }),
      );
      // Subtract the sticky header height so the target row remains visible.
      const start = (first - 1) * rowHeight;
      expect(start).toBeGreaterThan(bandHeight);
      expect(scrollTo).toHaveBeenCalledWith(
        expect.objectContaining({ top: start - bandHeight }),
      );
    });

    it("一覧の先頭が帯の下に隠れていれば、「新しいタグ」は先頭まで戻してから作成の行を出す", async () => {
      const user = userEvent.setup();
      const listTop = -300;
      const rect = vi
        .spyOn(Element.prototype, "getBoundingClientRect")
        .mockReturnValue({ top: listTop } as DOMRect);
      try {
        install();
        renderPage();
        await screen.findByTitle(name(0));
        scrollWindow();
        const scrollTo = vi.mocked(window.scrollTo);
        scrollTo.mockClear();

        await user.click(screen.getByRole("button", { name: "New tag" }));

        expect(scrollTo).toHaveBeenCalledWith({ top: scrolled + listTop - bandHeight });
        await waitFor(() =>
          expect(document.activeElement).toBe(
            screen.getByRole("textbox", { name: "New tag name" }),
          ),
        );
      } finally {
        rect.mockRestore();
      }
    });
  });

  it("全件の最後の行からの Tab は既定のまま一覧の外へ進む", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.slice(0, 3);
    install();
    renderPage();
    await screen.findByTitle(name(2));
    const more = within(wrapperOf(2)).getByRole("button", { name: "More actions" });
    more.focus();
    await user.tab();
    expect(document.activeElement).not.toBe(more);
    expect(document.activeElement?.closest("[data-index]")).toBeNull();
  });

  it("削除で行が消えると、描いていなかった次の行を描いてその「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) => ({ ...item, tentative: false }));
    install();
    renderPage();
    await screen.findByTitle(name(0));
    const index = await tabPastRange(user);
    expect(document.querySelector(`[data-index="${String(index + 1)}"]`)).toBeNull();

    await user.click(
      within(wrapperOf(index)).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(screen.queryByTitle(name(index))).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle(name(index + 1)).closest<HTMLElement>("[data-tag-id]")!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「Tentative only」中の確定で行が外れると、描いていなかった次の行の「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await toggleFilter(user, "Tentative only");
    const index = await tabPastRange(user);
    expect(document.querySelector(`[data-index="${String(index + 1)}"]`)).toBeNull();

    await user.click(within(wrapperOf(index)).getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.queryByTitle(name(index))).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle(name(index + 1)).closest<HTMLElement>("[data-tag-id]")!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });
});

describe("TagsPage 並び順と0本の絞り込み", () => {
  function rowNames(): string[] {
    return [...document.querySelectorAll("[data-index]")]
      .sort(
        (a, b) =>
          Number((a as HTMLElement).dataset.index) -
          Number((b as HTMLElement).dataset.index),
      )
      .map(
        (row) => row.querySelector("[data-tag-id] [title]")?.getAttribute("title") ?? "",
      );
  }

  async function chooseSort(
    user: ReturnType<typeof userEvent.setup>,
    current: string,
    next: string,
  ) {
    await user.click(screen.getByRole("button", { name: `Sort by: ${current}` }));
    await user.click(await screen.findByRole("menuitemradio", { name: next }));
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", videoCount: 5, createdAt: "2025-01-03T00:00:00Z" }),
      tag({ id: 2, name: "Beta", createdAt: "2025-01-05T00:00:00Z" }),
      tag({ id: 3, name: "Cat", videoCount: 5, createdAt: "2025-01-01T00:00:00Z" }),
      tag({ id: 4, name: "Delta", tentative: true, createdAt: "2025-01-02T00:00:00Z" }),
      tag({
        id: 5,
        name: "Echo",
        tentative: true,
        videoCount: 1,
        createdAt: "2025-01-04T00:00:00Z",
      }),
    ];
  });

  it("既定は名前の順で、「Name」のときは向きの切り替えを出さない", async () => {
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    expect(rowNames()).toEqual(["Alpha", "Beta", "Cat", "Delta", "Echo"]);
    expect(screen.getByRole("button", { name: "Sort by: Name" })).toBeDefined();
    expect(screen.queryByRole("button", { name: /Press for/ })).toBeNull();
  });

  it("「Video count」は多い順で最多が先頭・0本が末尾、同じ本数は名前の順（受け入れ条件4）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await chooseSort(user, "Name", "Video count");
    await waitFor(() =>
      expect(rowNames()).toEqual(["Alpha", "Cat", "Echo", "Beta", "Delta"]),
    );

    const toggle = screen.getByRole("button", {
      name: "Descending (most videos first). Press for ascending",
    });
    await user.click(toggle);
    await waitFor(() =>
      expect(rowNames()).toEqual(["Beta", "Delta", "Echo", "Alpha", "Cat"]),
    );
    expect(
      screen.getByRole("button", {
        name: "Ascending (fewest videos first). Press for descending",
      }),
    ).toBeDefined();
  });

  it("「Date created」は新しい順で、作成したタグが先頭に入る（受け入れ条件5）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await chooseSort(user, "Name", "Date created");
    await waitFor(() =>
      expect(rowNames()).toEqual(["Beta", "Echo", "Alpha", "Delta", "Cat"]),
    );

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "Zulu");
    await user.keyboard("{Enter}");
    await screen.findByTitle("Zulu");
    await waitFor(() => expect(rowNames()[0]).toBe("Zulu"));
  });

  it("並び順は開き直しても残り、壊れた保存は名前の順になる（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    const first = renderPage();
    await screen.findByTitle("Alpha");
    await chooseSort(user, "Name", "Video count");
    await waitFor(() => expect(rowNames()[0]).toBe("Alpha"));
    await user.click(
      screen.getByRole("button", {
        name: "Descending (most videos first). Press for ascending",
      }),
    );
    first.unmount();

    const second = renderPage();
    await screen.findByTitle("Alpha");
    expect(screen.getByRole("button", { name: "Sort by: Video count" })).toBeDefined();
    expect(rowNames()).toEqual(["Beta", "Delta", "Echo", "Alpha", "Cat"]);
    second.unmount();

    window.localStorage.setItem("vv.tags.v1", "{not json");
    renderPage();
    await screen.findByTitle("Alpha");
    expect(screen.getByRole("button", { name: "Sort by: Name" })).toBeDefined();
    expect(rowNames()).toEqual(["Alpha", "Beta", "Cat", "Delta", "Echo"]);
  });

  it("「Unused only」は0本の行だけを出し、「Tentative only」・検索・並び順と重なる（受け入れ条件7）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    expect(await filterChecked(user, "Unused only")).toBe(false);
    await toggleFilter(user, "Unused only");
    expect(await filterChecked(user, "Unused only")).toBe(true);
    expect(await screen.findByText("2 of 5 tags")).toBeDefined();
    expect(rowNames()).toEqual(["Beta", "Delta"]);
    for (const name of rowNames()) {
      const row = screen.getByTitle(name).closest("[data-tag-id]")!;
      expect(row.textContent).toContain("0 videos");
    }

    await chooseSort(user, "Name", "Date created");
    await waitFor(() => expect(rowNames()).toEqual(["Beta", "Delta"]));
    await user.click(
      screen.getByRole("button", {
        name: "Descending (newest first). Press for ascending",
      }),
    );
    await waitFor(() => expect(rowNames()).toEqual(["Delta", "Beta"]));

    await toggleFilter(user, "Tentative only");
    expect(await screen.findByText("1 of 5 tags")).toBeDefined();
    expect(rowNames()).toEqual(["Delta"]);

    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "zz");
    expect(await screen.findByText('No unused tentative tags match "zz"')).toBeDefined();
    expect(screen.getByText("0 of 5 tags")).toBeDefined();
  });

  it("0本のタグが無いときは「No unused tags」を出し、「Show all tags」で外して「Filter」へ戻る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) => ({
      ...item,
      videoCount: item.videoCount + 1,
    }));
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Unused only");
    expect(await screen.findByText("No unused tags")).toBeDefined();
    expect(screen.getByText("Every tag is on at least one video.")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
    expect(await screen.findByText("5 tags")).toBeDefined();
  });

  it("「Unused only」と「Tentative only」の両方で一致が無ければ両方を外して「Filter」へ戻る", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item) =>
      item.tentative ? { ...item, videoCount: 2 } : item,
    );
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Unused only");
    await toggleFilter(user, "Tentative only");
    expect(await screen.findByText("No unused tentative tags")).toBeDefined();
    expect(screen.queryByText("Every tag is on at least one video.")).toBeNull();
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (2 applied)");

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
    expect(await filterChecked(user, "Tentative only")).toBe(false);
    expect(await filterChecked(user, "Unused only")).toBe(false);
  });

  it("「Unused only」と検索で一致が無ければ、両方を外して検索の入力へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Unused only");
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "Alpha");
    expect(await screen.findByText('No unused tags match "Alpha"')).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(search));
    expect((search as HTMLInputElement).value).toBe("");
    expect(screen.getByText("5 tags")).toBeDefined();
  });

  it("改名中の行は並び順・絞り込みを変えても残り、打っている途中の名前を失わない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const row = screen.getByTitle("Alpha").closest("[data-tag-id]")!;
    await user.click(within(row as HTMLElement).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "Alpha"' });
    await user.clear(input);
    await user.type(input, "下書き");

    await chooseSort(user, "Name", "Video count");
    await toggleFilter(user, "Unused only");
    expect(await screen.findByText("2 of 5 tags")).toBeDefined();
    const still = screen.getByRole("textbox", { name: 'New name for "Alpha"' });
    expect(still).toBe(input);
    expect((still as HTMLInputElement).value).toBe("下書き");
    expect(document.querySelectorAll("[data-index]")).toHaveLength(3);
  });

  it("読み込み中は並び順と「Filter」が押せない", async () => {
    install();
    hold.getsFrom = 1;
    renderPage();

    const user = userEvent.setup();
    expect((filterButton() as HTMLButtonElement).disabled).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Sort by: Name" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    // jsdom does not inherit disabled from fieldset; inspect the fieldset itself.
    await user.click(screen.getByRole("button", { name: "Sort" }));
    const group = await screen.findByRole("group", { name: "Sort by" });
    expect((group as HTMLFieldSetElement).disabled).toBe(true);
    for (const done of heldGetReleases) done();
  });

  it("狭い幅のまとめから並び順を選べる（ライブラリの CompactSortControls と同じ形）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: "Sort" }));
    const compact = await screen.findByRole("dialog");
    expect(
      within(compact).queryByRole("radiogroup", { name: "Sort direction" }),
    ).toBeNull();
    await user.click(within(compact).getByRole("radio", { name: "Video count" }));
    await waitFor(() => expect(rowNames()[0]).toBe("Alpha"));
    await user.click(within(compact).getByRole("radio", { name: "Fewest videos first" }));
    await waitFor(() => expect(rowNames()[0]).toBe("Beta"));
  });

  it("疑似ロケールで、並び順と「Unused only」の文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    const { container } = renderPage();
    await screen.findByTitle("Alpha");
    await user.click(screen.getByRole("button", { name: /Filter/ }));
    await user.click(await screen.findByRole("checkbox", { name: /Unused only/ }));
    await user.keyboard("{Escape}");
    await screen.findByTitle("Beta");
    expectCatalogTextOnly(container, ["Alpha", "Beta", "Cat", "Delta", "Echo"]);
  });
});
