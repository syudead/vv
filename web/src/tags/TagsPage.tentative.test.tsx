import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  tag,
  server,
  rejectedGetReleases,
  confirmReleases,
  hold,
  install,
  renderPage,
  filterButton,
  toggleFilter,
  filterChecked,
  rejectedTab,
  tagsTab,
  openRejectedTab,
  setUpTagsPageServer,
} from "../testing/tagsPage";

setUpTagsPageServer();

describe("TagsPage 仮のタグ", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest<HTMLElement>("[data-tag-id]")!;
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
      tag({ id: 3, name: "Cat", tentative: true }),
      tag({ id: 4, name: "Gamma", synonyms: ["ガンマ"], videoCount: 5 }),
    ];
  });

  it("仮のタグの行は目印と「確定する」を持ち、確定したタグの行は持たない（受け入れ条件5）", async () => {
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const alpha = rowOf("Alpha");
    expect(within(alpha).getByText("Tentative")).toBeDefined();
    expect(within(alpha).getByRole("button", { name: "Confirm" })).toBeDefined();

    const gamma = rowOf("Gamma");
    expect(within(gamma).queryByText("Tentative")).toBeNull();
    expect(within(gamma).queryByRole("button", { name: "Confirm" })).toBeNull();
    // 確定した行の操作は今と同じ3つ（マウスの端末）と、タッチ・狭い幅でそれを
    // まとめる「Actions」（CSS でどちらか一方だけを描く）。
    expect(
      within(gamma)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["Actions", "Rename", "Synonyms", "More actions"]);
  });

  it("「Tentative only」で仮のタグだけが並び、件数は全タグを分母にする（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Gamma");

    expect(await filterChecked(user, "Tentative only")).toBe(false);
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
    await toggleFilter(user, "Tentative only");

    expect(await filterChecked(user, "Tentative only")).toBe(true);
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (1 applied)");
    expect(await screen.findByText("3 of 4 tags")).toBeDefined();
    expect(screen.queryByTitle("Gamma")).toBeNull();
    expect(screen.getByTitle("Alpha")).toBeDefined();

    // 検索と重ねられる。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "be");
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
    expect(screen.getByTitle("Beta")).toBeDefined();
    expect(screen.queryByTitle("Alpha")).toBeNull();

    await toggleFilter(user, "Tentative only");
    expect(await screen.findByText("1 of 4 tags")).toBeDefined();
  });

  it("確定するとその行が確定したタグになり、同じ行の「改名」へフォーカスが移る（受け入れ条件7）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));

    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    await waitFor(() =>
      expect(
        within(rowOf("Alpha")).queryByRole("button", { name: "Confirm" }),
      ).toBeNull(),
    );
    expect(within(rowOf("Alpha")).queryByText("Tentative")).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Alpha")).getByRole("button", { name: "Rename" }),
      ),
    );
    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    expect(await screen.findByRole("menuitem", { name: "Delete…" })).toBeDefined();
  });

  it("確定の送信中は二度押しを無視し、同じ行の「改名」「その他の操作」がdisabledになる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    hold.confirms = true;
    const confirm = within(rowOf("Alpha")).getByRole("button", { name: "Confirm" });
    await user.click(confirm);
    await user.click(confirm);
    await user.keyboard("{Enter}");

    expect(server.confirmCalls).toBe(1);
    expect(confirm.getAttribute("aria-busy")).toBe("true");
    expect(confirm.getAttribute("aria-disabled")).toBe("true");
    expect(confirm.hasAttribute("disabled")).toBe(false);
    const alpha = rowOf("Alpha");
    expect(
      within(alpha).getByRole("button", { name: "Rename" }).hasAttribute("disabled"),
    ).toBe(true);
    expect(
      within(alpha)
        .getByRole("button", { name: "More actions" })
        .hasAttribute("disabled"),
    ).toBe(true);
    // ほかの行はそのまま。
    expect(
      within(rowOf("Beta"))
        .getByRole("button", { name: "Rename" })
        .hasAttribute("disabled"),
    ).toBe(false);

    confirmReleases.forEach((resolve) => resolve());
    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    expect(server.confirmCalls).toBe(1);
  });

  it("「Tentative only」中に確定すると、その行が外れて次の行の「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.queryByTitle("Alpha")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Beta")).getByRole("button", { name: "Rename" }),
      ),
    );
    expect(screen.getByText("2 of 4 tags")).toBeDefined();
  });

  it("仮のタグの行のメニューは「削除…」の代わりに「却下する…」を持ち、確定したタグの行は今のまま（受け入れ条件12）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    const items = await screen.findAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual([
      "Merge into another tag…",
      "Reject…",
    ]);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());

    await user.click(
      within(rowOf("Gamma")).getByRole("button", { name: "More actions" }),
    );
    const confirmedItems = await screen.findAllByRole("menuitem");
    expect(confirmedItems.map((item) => item.textContent)).toEqual([
      "Merge into another tag…",
      "Delete…",
    ]);
  });

  it("却下すると行が消えて却下した名前の一覧に出る（受け入れ条件8）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("0"));

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });
    expect(
      within(dialog).getByText(
        'This tag will be removed from 2 videos, and automatic tagging won\'t create "Alpha" again. You can allow the name again from Rejected names.',
      ),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    await waitFor(() => expect(screen.queryByTitle("Alpha")).toBeNull());
    expect(await screen.findByText('Rejected "Alpha"')).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Beta")).getByRole("button", { name: "Rename" }),
      ),
    );
    await waitFor(() => expect(entry.textContent).toContain("1"));

    const list = await openRejectedTab(user);
    expect(within(list).getByTitle("Alpha")).toBeDefined();
  });

  it("0本の仮のタグの却下の確認は、どの動画にも付いていないと出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Beta");

    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Beta"' });
    expect(
      within(dialog).getByText(
        "This tag isn't on any videos. Automatic tagging won't create \"Beta\" again. You can allow the name again from Rejected names.",
      ),
    ).toBeDefined();

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Beta")).getByRole("button", { name: "More actions" }),
      ),
    );
  });

  it("却下の前に別のタブで確定されていると、窓を閉じて一覧を取り直す（Edge Case「操作の競合」）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });

    // 窓を開いたあと、別のタブで先に確定されたことにする。
    server.tags = server.tags.map((item) =>
      item.id === 1 ? { ...item, tentative: false } : item,
    );
    const getsBefore = server.getCalls;
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(
      await screen.findByText("This tag was already confirmed, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(server.getCalls).toBeGreaterThan(getsBefore);
    await waitFor(() =>
      expect(
        within(rowOf("Alpha")).queryByRole("button", { name: "Confirm" }),
      ).toBeNull(),
    );
    expect(server.rejectedNames).toEqual([]);
  });

  it("却下の一般の失敗は窓の中のalertで伝え、窓は開いたまま", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });
    server.nextError = { status: 500, body: { code: "internal", message: "x" } };
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    expect(screen.getByRole("alertdialog", { name: 'Reject "Alpha"' })).toBeDefined();
  });

  it("却下した名前を「Allow again」で一覧から外せる。最後の1つならタブへフォーカスが移る（受け入れ条件13）", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("2"));
    const list = await openRejectedTab(user);
    expect(entry.getAttribute("aria-selected")).toBe("true");
    expect(
      screen.getByText(
        "Automatic tagging won't create these names. Allow a name again to let it be created.",
      ),
    ).toBeDefined();

    await user.click(within(list).getByRole("button", { name: 'Allow "Old" again' }));
    await waitFor(() => expect(within(list).queryByTitle("Old")).toBeNull());
    expect(server.rejectedNames).toEqual(["Stale"]);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(list).getByRole("button", { name: 'Allow "Stale" again' }),
      ),
    );

    await user.click(within(list).getByRole("button", { name: 'Allow "Stale" again' }));
    expect(await screen.findByText("No rejected names")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(rejectedTab()));
    expect(rejectedTab().textContent).toContain("0");
  });

  it("却下した名前は見出しの下のタブで、選ぶと本文に一覧と取り外しが出る（受け入れ条件12）", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    const firstRow = screen.getByTitle("Alpha").closest("[data-tag-id]")!;
    expect(
      entry.compareDocumentPosition(firstRow) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // タブは見出しと同じ帯の中にある。窓は開かない。
    expect(entry.closest(".sticky")).toBe(
      screen.getByRole("heading", { name: "Tags" }).closest(".sticky"),
    );
    expect(entry.getAttribute("aria-haspopup")).toBeNull();
    expect(tagsTab().getAttribute("aria-selected")).toBe("true");
    await waitFor(() => expect(entry.textContent).toContain("1"));

    const list = await openRejectedTab(user);
    expect(screen.queryByRole("dialog")).toBeNull();
    // タグの行は「Tags」のタブに戻るまで描かない。
    expect(
      screen.queryByRole("link", { name: "Open the library filtered by Alpha" }),
    ).toBeNull();
    expect(
      screen.getByText(
        "Automatic tagging won't create these names. Allow a name again to let it be created.",
      ),
    ).toBeDefined();
    await user.click(within(list).getByRole("button", { name: 'Allow "Old" again' }));
    expect(await screen.findByText("No rejected names")).toBeDefined();
    expect(server.rejectedNames).toEqual([]);

    await user.click(tagsTab());
    expect(await screen.findByTitle("Alpha")).toBeDefined();
  });

  it("タグの一覧を取る前もタブは出し、件数は届いてから添える。タグが無いときも出す", async () => {
    server.tags = [];
    install();
    renderPage();
    expect(rejectedTab()).toBeDefined();
    expect(await screen.findByText("No tags yet")).toBeDefined();
    await waitFor(() => expect(rejectedTab().textContent).toContain("0"));
    expect(tagsTab().textContent).toContain("0");
  });

  it("却下した名前を取れなかったら理由を出し、再試行できる", async () => {
    const user = userEvent.setup();
    server.failRejectedGets = true;
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    await user.click(rejectedTab());
    expect(await screen.findByText("Couldn't load the rejected names")).toBeDefined();

    server.failRejectedGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("button", { name: 'Allow "Old" again' }),
    ).toBeDefined();
  });

  it("取り外しの前から待っていた取り直しの古い応答で、外した名前が戻らない", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    // 却下で取り直しが始まり、その応答は取り外しの前の並びを持ったまま止まる。
    hold.rejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    expect(rejectedGetReleases).toHaveLength(1);
    hold.rejectedGets = false;

    const list = await openRejectedTab(user);

    await user.click(within(list).getByRole("button", { name: 'Allow "Old" again' }));
    await waitFor(() => expect(within(list).queryByTitle("Old")).toBeNull());

    rejectedGetReleases.forEach((resolve) => resolve());
    // 取り外しのあとの取り直しが却下した Beta を並べ、古い応答の Old は戻らない。
    expect(
      await within(list).findByRole("button", { name: 'Allow "Beta" again' }),
    ).toBeDefined();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(within(list).queryByTitle("Old")).toBeNull();
  });

  it("一覧を持ったあとの取り直しに失敗しても、古い並びを残さず理由と再試行を出す", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await user.click(rejectedTab());
    expect(
      await screen.findByRole("button", { name: 'Allow "Old" again' }),
    ).toBeDefined();
    await user.click(tagsTab());
    await screen.findByTitle("Beta");

    server.failRejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());

    await user.click(rejectedTab());
    expect(await screen.findByText("Couldn't load the rejected names")).toBeDefined();
    expect(screen.queryByRole("button", { name: 'Allow "Old" again' })).toBeNull();

    server.failRejectedGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("button", { name: 'Allow "Beta" again' }),
    ).toBeDefined();
  });

  it("「Tentative only」中に作成を取り消すと、「新しいタグ」へフォーカスが戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");

    const newTag = screen.getByRole("button", { name: "New tag" });
    await user.click(newTag);
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(newTag));
  });

  it("確定の送信中に「Tentative only」を外すと、確定した行はそのまま出て、その行の「改名」へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");

    hold.confirms = true;
    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));
    await toggleFilter(user, "Tentative only");
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");

    confirmReleases.forEach((resolve) => resolve());
    expect(await screen.findByText('Confirmed "Alpha"')).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Alpha")).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「Tentative only」中に並行した確定が逆の順で終わっても、残った次の行へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");

    hold.confirms = true;
    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Confirm" }));
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "Confirm" }));
    expect(confirmReleases).toHaveLength(2);

    // 後から押した Beta が先に終わり、そのあとで Alpha が終わる。
    const [releaseAlpha, releaseBeta] = confirmReleases;
    releaseBeta!();
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    releaseAlpha!();
    await waitFor(() => expect(screen.queryByTitle("Alpha")).toBeNull());

    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Cat")).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「新しいタグ」で却下した名前を作ると、却下した名前の一覧から消える（受け入れ条件14）", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("1"));

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "Old");
    await user.keyboard("{Enter}");
    await screen.findByTitle("Old");

    await waitFor(() => expect(entry.textContent).toContain("0"));
    await user.click(entry);
    expect(await screen.findByText("No rejected names")).toBeDefined();
    expect(screen.queryByRole("button", { name: 'Allow "Old" again' })).toBeNull();
  });

  it("「Tentative only」中の仮のタグどうしの統合は、「新しいタグ」へ落ちず次の行へフォーカスが移る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "Alpha"' });
    await user.type(
      within(dialog).getByRole("combobox", { name: "Tag to merge into" }),
      "Beta",
    );
    await user.click(await within(dialog).findByRole("option", { name: /Beta/ }));
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(await screen.findByText('Merged "Alpha" into "Beta"')).toBeDefined();
    // 統合元（Alpha）は消え、統合先（Beta）は確定になって絞り込みから外れる。
    await waitFor(() => expect(screen.queryByTitle("Beta")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Cat")).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("「Tentative only」中に唯一のタグを却下すると、「Filter」が押せるまま、そこにフォーカスがある", async () => {
    const user = userEvent.setup();
    server.tags = [tag({ id: 1, name: "Alpha", tentative: true })];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");
    const filter = filterButton();

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Reject "Alpha"' });
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    expect(await screen.findByText("No tentative tags")).toBeDefined();
    expect(filter.hasAttribute("disabled")).toBe(false);
    await waitFor(() => expect(document.activeElement).toBe(filter));

    // 「Show all tags」で外すとタグが 0 なので、フォーカスは「新しいタグ」へ。
    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    expect(await screen.findByText("No tags yet")).toBeDefined();
    expect(filter.hasAttribute("disabled")).toBe(true);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getAllByRole("button", { name: "New tag" })[0],
      ),
    );
  });

  it("「Tentative only」中に唯一の仮のタグへシノニムを足すと、入力にフォーカスが残り、閉じると「Filter」へ移る", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true }),
      tag({ id: 4, name: "Gamma" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");
    const toggle = filterButton();

    await user.click(within(rowOf("Alpha")).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Alpha"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "アルファ");
    await user.keyboard("{Enter}");

    expect(await within(dialog).findByText("アルファ")).toBeDefined();
    expect(document.activeElement).toBe(input);

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByTitle("Alpha")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(toggle));
  });

  it("「Tentative only」と検索で一致が無いときは、両方を外して検索の入力へ戻れる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    await toggleFilter(user, "Tentative only");
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "Gamma");

    expect(await screen.findByText('No tentative tags match "Gamma"')).toBeDefined();
    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(document.activeElement).toBe(search));
    expect(filterButton().getAttribute("aria-label")).toBe("Filter");
    expect(await screen.findByText("4 tags")).toBeDefined();
  });

  it("疑似ロケールで、仮のタグの行・絞り込み・却下の窓・却下した名前の文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const userData = ["Alpha", "Beta", "Cat", "Gamma", "ガンマ", "Old"];
    expectCatalogTextOnly(document.body, userData);

    await user.click(screen.getByRole("tab", { name: /Rejected names/ }));
    await screen.findByRole("table", { name: /Rejected names/ });
    expectCatalogTextOnly(document.body, userData);
    await user.click(screen.getByRole("tab", { name: /Tags/ }));
    await screen.findByTitle("Alpha");

    await user.click(screen.getByRole("button", { name: /Filter/ }));
    expectCatalogTextOnly(document.body, userData);
    await user.click(await screen.findByRole("checkbox", { name: /Tentative only/ }));
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTitle("Gamma")).toBeNull());
    expectCatalogTextOnly(document.body, userData);

    await user.click(
      within(rowOf("Alpha")).getByRole("button", { name: /More actions/ }),
    );
    await screen.findByRole("menu");
    expectCatalogTextOnly(document.body, userData);
    await user.click(screen.getByRole("menuitem", { name: /Reject/ }));
    await screen.findByRole("alertdialog");
    expectCatalogTextOnly(document.body, userData);
  });
});
