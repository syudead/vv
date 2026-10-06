import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  tag,
  server,
  install,
  renderPage,
  filterButton,
  toggleFilter,
  setUpTagsPageServer,
} from "../testing/tagsPage";

setUpTagsPageServer();

describe("TagsPage まとめての操作", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  function selectAll(): HTMLElement {
    return screen.getByRole("checkbox", {
      name: /^Select (all [\d,]+ loaded tags|the 1 loaded tag)$/,
    });
  }

  function bar(): HTMLElement {
    return screen.getByRole("region", { name: "Selected tags" });
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
      tag({ id: 3, name: "Cat", videoCount: 4 }),
      tag({ id: 4, name: "Gamma", synonyms: ["ガンマ"], videoCount: 5 }),
    ];
  });

  it("「Tentative only」で読み込んだものをすべて選んで確定すると、全 id を 1 回送り、仮の目印と選択が消え、一覧は取り直さない（受け入れ条件10）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await toggleFilter(user, "Tentative only");
    await waitFor(() => expect(screen.queryByTitle("Cat")).toBeNull());
    expect(server.pageRequests.at(-1)?.get("tentative")).toBe("true");
    const gets = server.getCalls;
    expect(selectAll().getAttribute("aria-label")).toBe("Select all 2 loaded tags");
    await user.click(selectAll());
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
    expect(screen.getByRole("checkbox", { name: "Clear selection" })).toBeDefined();

    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));

    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull(),
    );
    expect(server.batchCalls).toEqual([{ action: "confirm", ids: [1, 2] }]);
    expect(await screen.findByText("Confirmed 2 tags")).toBeDefined();
    // 確定は読み込んだ行の中で反映し、ページも全件も取り直さない（R-12）。
    expect(server.getCalls).toBe(gets);
    expect(server.fullGetCalls).toBe(0);
    expect(screen.getByText("0 of 4 tags")).toBeDefined();
    // 確定した行は「Tentative only」から外れ、空になれば「Filter」へ移る。
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
    await toggleFilter(user, "Tentative only");
    await screen.findByTitle("Alpha");
    expect(screen.queryByText("Tentative")).toBeNull();
  });

  it("仮と確定を混ぜて確定すると、既に確定していた分を数えて伝え、それを選んだまま残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(selectAll());
    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));

    expect(
      await screen.findByText("Confirmed 2 tags. 2 were already confirmed."),
    ).toBeDefined();
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
    // 残った選択は確定したタグだけなので「Confirm」「Reject…」は出さず、フォーカスは
    // 「Merge into one tag…」へ。
    expect(within(bar()).queryByRole("button", { name: "Confirm" })).toBeNull();
    expect(within(bar()).queryByRole("button", { name: "Reject…" })).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(bar()).getByRole("button", { name: "Merge into one tag…" }),
      ),
    );
  });

  it("仮と確定を混ぜて削除すると、確認に確定したタグの数と動画の本数が出て、外した数をトーストで伝える（受け入れ条件11）", async () => {
    const user = userEvent.setup();
    server.impactVideoCount = 7;
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(selectAll());
    await user.click(within(bar()).getByRole("button", { name: "Delete…" }));

    const dialog = await screen.findByRole("alertdialog", {
      name: "Delete selected tags",
    });
    expect(server.impactCalls).toEqual([{ action: "delete", ids: [1, 2, 3, 4] }]);
    expect(
      await within(dialog).findByText(
        "2 of the 4 selected tags are confirmed. They will be removed from 7 videos. This can't be undone. The 2 tentative tags are left as they are. Reject them instead.",
      ),
    ).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByText("Deleted 2 tags. 2 tentative tags were skipped."),
    ).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "delete", ids: [1, 2, 3, 4] }]);
    expect(screen.queryByTitle("Cat")).toBeNull();
    expect(screen.queryByTitle("Gamma")).toBeNull();
    // 働かなかった仮のタグは選んだまま。
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
  });

  it("確認の数が届くまで実行できず、数えられなければ理由と再試行を出す", async () => {
    const user = userEvent.setup();
    server.failImpact = true;
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    await user.click(within(bar()).getByRole("button", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: "Reject selected tags",
    });
    const reject = within(dialog).getByRole("button", { name: "Reject" });
    expect((reject as HTMLButtonElement).disabled).toBe(true);

    expect(
      (await within(dialog).findByRole("alert")).textContent?.startsWith(
        "Couldn't count the affected videos:",
      ),
    ).toBe(true);
    expect((reject as HTMLButtonElement).disabled).toBe(true);

    server.failImpact = false;
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    expect(
      await within(dialog).findByText(
        "The selected tag will be removed from 2 videos, and automatic tagging won't create its name again. You can allow a name again from Rejected names.",
      ),
    ).toBeDefined();
    await user.click(reject);
    expect(await screen.findByText("Rejected 1 tag")).toBeDefined();
    expect(screen.queryByTitle("Alpha")).toBeNull();
  });

  it("対象の一部がもう無ければ一覧を取り直し、失敗すれば選択を残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    await user.click(within(rowOf("Beta")).getByRole("checkbox"));

    // 失敗は何も変えず、選択は残る。
    server.failNextBatch = true;
    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));
    expect(await screen.findByText(/Something went wrong/)).toBeDefined();
    expect(within(bar()).getByText("2 tags selected")).toBeDefined();
    expect(within(rowOf("Alpha")).getByText("Tentative")).toBeDefined();

    // 別のタブで Beta が消えていた。
    server.tags = server.tags.filter((item) => item.id !== 2);
    const gets = server.getCalls;
    await user.click(within(bar()).getByRole("button", { name: "Confirm" }));
    expect(
      await screen.findByText(
        "Some of the tags no longer existed, so the list was reloaded",
      ),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    expect(server.getCalls).toBeGreaterThan(gets);
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
  });

  it("並び順を変えると sort=countDesc で読み直されて選択が空になり、検索を変えても空になる（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(selectAll());
    expect(within(bar()).getByText("4 tags selected")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
    await waitFor(() =>
      expect(server.pageRequests.at(-1)?.get("sort")).toBe("countDesc"),
    );
    expect(server.pageRequests.at(-1)?.get("limit")).toBe("100");
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    // 読み直した行が同じ id を持っていても選び直さない。
    await waitFor(() =>
      expect(screen.getAllByRole("link").map((link) => link.textContent)).toEqual([
        "Gamma",
        "Cat",
        "Alpha",
        "Beta",
      ]),
    );
    expect(selectAll().getAttribute("aria-checked")).toBe("false");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    expect(within(bar()).getByText("1 tag selected")).toBeDefined();
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "a");
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull(),
    );
  });

  it("選んでいる行の改名を始めると選択から外れ、改名中の行のチェックは押せない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Rename" }));
    await screen.findByRole("textbox", { name: 'New name for "Alpha"' });
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    const check = screen.getByRole("checkbox", { name: 'Select "Alpha"' });
    expect((check as HTMLButtonElement).disabled).toBe(true);
  });

  it("選んでいる間はページの下端に選択バーを出し、働かない操作は出さない。×で解くとバーが消え、先頭のチェックへフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    expect(screen.getByRole("heading", { name: "Tags" })).toBeDefined();

    // 確定したタグだけを選ぶと「Confirm」「Reject…」は出さない（薄くもしない）。
    await user.click(within(rowOf("Cat")).getByRole("checkbox"));
    expect(within(bar()).getByText("1 tag selected")).toBeDefined();
    expect(
      within(bar())
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Clear selection", "Merge into one tag…", "Delete…"]);

    // 仮のタグも選ぶと「Confirm」「Reject…」が出る。
    await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
    expect(
      within(bar())
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual([
      "Clear selection",
      "Confirm",
      "Merge into one tag…",
      "Reject…",
      "Delete…",
    ]);
    // 選択バーはデザインシステムの SelectionBar で、ページの下端に貼り付く。
    expect(bar().querySelector('[data-slot="selection-bar"]')).not.toBeNull();

    await user.click(within(bar()).getByRole("button", { name: "Clear selection" }));
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Tags" })).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(selectAll()));
  });

  it("選んだ行は表の選択の面（primary-soft）で塗る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Cat")).getByRole("checkbox"));
    // 表の選択した行（data-state="selected"）は primary-soft の面になる。
    expect(rowOf("Cat").getAttribute("data-state")).toBe("selected");
    expect(rowOf("Cat").className).toContain("data-[state=selected]:bg-primary-soft");
    expect(rowOf("Gamma").getAttribute("data-state")).toBeNull();
  });

  it("タッチ・狭い幅の「Actions」は文字を持つ項目を並べ、「Confirm」は行の「確定する」と同じ結果になる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Actions" }));
    const items = (await screen.findAllByRole("menuitem")).map(
      (item) => item.textContent,
    );
    expect(items).toEqual([
      "Confirm",
      "Rename",
      "Synonyms",
      "Merge into another tag…",
      "Reject…",
    ]);
    await user.click(screen.getByRole("menuitem", { name: "Confirm" }));

    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    expect(server.confirmCalls).toBe(1);
    await waitFor(() =>
      expect(within(rowOf("Alpha")).queryByText("Tentative")).toBeNull(),
    );

    await user.click(within(rowOf("Cat")).getByRole("button", { name: "Actions" }));
    expect(
      (await screen.findAllByRole("menuitem")).map((item) => item.textContent),
    ).toEqual(["Rename", "Synonyms", "Merge into another tag…", "Delete…"]);
  });

  it("疑似ロケールで、選択・選択バー・まとめての確認の文言がカタログから出る", async () => {
    const user = userEvent.setup();
    enablePseudoLocale();
    install();
    const { container } = renderPage();
    await screen.findByTitle("Alpha");

    // Alpha（仮のタグ）を選ぶ。選択の行のボタンは ×・Confirm・Merge・Reject の順。
    await user.click(
      container.querySelector<HTMLElement>('[data-tag-id] [role="checkbox"]')!,
    );
    expectCatalogTextOnly(document.body, ["Alpha", "Beta", "Cat", "Gamma", "ガンマ"]);
    await user.click(
      within(
        screen
          .getAllByRole("region")
          // 通知（Sonner）の section も region なので外す。
          .find((region) => region.tagName !== "SECTION")!,
      ).getAllByRole("button")[3]!,
    );
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() => expect(server.impactCalls).toHaveLength(1));
    await within(dialog).findByText(/videos|video/i);
    expectCatalogTextOnly(document.body, ["Alpha", "Beta", "Cat", "Gamma", "ガンマ"]);
  });
});

describe("TagsPage まとめての操作の上限", () => {
  let originalOffsetHeight: PropertyDescriptor | undefined;

  beforeEach(() => {
    originalOffsetHeight = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "offsetHeight",
    );
    Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
      configurable: true,
      get(this: HTMLElement) {
        return this.hasAttribute("data-index") ? 40 : 0;
      },
    });
  });

  afterEach(() => {
    if (originalOffsetHeight !== undefined) {
      Object.defineProperty(HTMLElement.prototype, "offsetHeight", originalOffsetHeight);
    }
  });

  it("読み込んだ行が上限を超えると先頭のチェックだけが押せず、1 行ずつ選んだ数行のまとめての操作は押せる", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 20002 }, (_, index) =>
      tag({ id: index + 1, name: `T${String(index).padStart(5, "0")}`, tentative: true }),
    );
    // 続きを何度も読んだあとと同じく、上限を超える行を読み込んだ状態を 1 回の応答で作る。
    server.pageLimitOverride = 20001;
    install();
    renderPage();
    await screen.findByTitle("T00000");

    const reason =
      "Too many tags are loaded to select them all at once (limit 20,000). Narrow the list with search or a filter.";
    const header = screen.getByRole("checkbox", {
      name: "Select all 20,001 loaded tags",
    });
    expect((header as HTMLButtonElement).disabled).toBe(true);
    expect(header.parentElement?.getAttribute("title")).toBe(reason);
    const describedBy = header.getAttribute("aria-describedby");
    expect(describedBy).not.toBeNull();
    expect(document.getElementById(describedBy!)?.textContent).toBe(reason);

    await user.click(screen.getByRole("checkbox", { name: 'Select "T00000"' }));
    await user.click(screen.getByRole("checkbox", { name: 'Select "T00001"' }));
    const region = screen.getByRole("region", { name: "Selected tags" });
    for (const name of ["Confirm", "Merge into one tag…", "Reject…"]) {
      const button = within(region).getByRole("button", { name });
      expect((button as HTMLButtonElement).disabled).toBe(false);
    }
    await user.click(within(region).getByRole("button", { name: "Confirm" }));
    expect(await screen.findByText("Confirmed 2 tags")).toBeDefined();
    expect(server.batchCalls).toEqual([{ action: "confirm", ids: [1, 2] }]);
  }, 60000);
});
