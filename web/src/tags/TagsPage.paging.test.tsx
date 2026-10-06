import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { subscribeTags } from "../api/tags";
import {
  tag,
  server,
  heldGetReleases,
  hold,
  install,
  renderPage,
  toggleFilter,
  tagsTab,
  setUpTagsPageServer,
} from "../testing/tagsPage";

setUpTagsPageServer();

describe("TagsPage サーバーのページで読む（specs/036-tag-admin-scale/research.md R-1・R-11・R-12）", () => {
  /** jsdom には表示域の高さと要素の高さが無いので、試験用の高さを置く。 */
  const rowHeight = 40;
  const viewportHeight = 200;
  let originalInnerHeight = 0;
  let originalOffsetHeight: PropertyDescriptor | undefined;

  /** name は並べたときに番号の順になる名前（名前の自然順）。 */
  function name(index: number): string {
    return `Tag ${String(index)}`;
  }

  function loadedNames(): string[] {
    return [...document.querySelectorAll<HTMLElement>("[data-index]")]
      .sort((a, b) => Number(a.dataset.index) - Number(b.dataset.index))
      .map(
        (row) => row.querySelector("[data-tag-id] [title]")?.getAttribute("title") ?? "",
      );
  }

  function count(): string {
    return screen
      .getAllByRole("status")
      .find((node) => node.closest('[data-slot="page-header-count"]') !== null)!
      .textContent!;
  }

  function rowOf(tagName: string): HTMLElement {
    return screen.getByTitle(tagName).closest("[data-tag-id]")!;
  }

  /** scrollTo は文書を top までスクロールしたことにし、仮想化に知らせる。 */
  function scrollTo(top: number) {
    Object.defineProperty(document.documentElement, "scrollHeight", {
      configurable: true,
      value: 1_000_000,
    });
    Object.defineProperty(window, "scrollY", { configurable: true, value: top });
    window.dispatchEvent(new Event("scroll"));
  }

  /** scrollToEnd は読み込んだ行の末尾近くまでスクロールし、続きのきっかけを作る。 */
  async function scrollToEnd(loaded: number) {
    scrollTo(loaded * 60);
    await waitFor(() =>
      expect(
        document.querySelector(`[data-index="${String(loaded - 1)}"]`),
      ).not.toBeNull(),
    );
  }

  function cursorRequests(): URLSearchParams[] {
    return server.pageRequests.filter((params) => params.has("cursor"));
  }

  beforeEach(() => {
    server.tags = Array.from({ length: 150 }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: index % 2 === 0 }),
    );
    originalInnerHeight = window.innerHeight;
    window.innerHeight = viewportHeight;
    originalOffsetHeight = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "offsetHeight",
    );
    Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
      configurable: true,
      get(this: HTMLElement) {
        return this.hasAttribute("data-index") ? rowHeight : 0;
      },
    });
  });

  afterEach(() => {
    window.innerHeight = originalInnerHeight;
    if (originalOffsetHeight !== undefined) {
      Object.defineProperty(HTMLElement.prototype, "offsetHeight", originalOffsetHeight);
    }
    Object.defineProperty(window, "scrollY", { configurable: true, value: 0 });
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });

  it("開くと GET /api/tags を limit=100&sort=name で 1 回だけ送り、全件は送らない", async () => {
    install();
    renderPage();
    await screen.findByTitle(name(0));

    expect(server.pageRequests.map((params) => params.toString())).toEqual([
      "sort=name&limit=100",
    ]);
    expect(server.fullGetCalls).toBe(0);
    // 件数は全部の数で、続きがあっても読み込んだ数（ページの区切り）は出さない。
    expect(count()).toBe("150 tags");
    expect(document.body.textContent).not.toContain("loaded");
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(server.getCalls).toBe(1);
  });

  it("検索は q で先頭から読み直し、応答の行だけが並ぶ（受け入れ条件9）", async () => {
    const user = userEvent.setup();
    server.tags.push(tag({ id: 999, name: "zz action" }));
    install();
    renderPage();
    await screen.findByTitle(name(0));
    expect(screen.queryByTitle("zz action")).toBeNull();

    await user.type(
      screen.getByRole("searchbox", { name: "Search tags" }),
      "ＡＣＴＩＯＮ",
    );

    await screen.findByTitle("zz action");
    // 打鍵ごとに引き直すので、途中の語（「ＡＣＴＩＯ」）の応答でも同じ行が並ぶ。最後の語の
    // 要求が出るまで待つ。
    await waitFor(() =>
      expect(server.pageRequests.at(-1)?.get("q")).toBe("ＡＣＴＩＯＮ"),
    );
    await waitFor(() => expect(loadedNames()).toEqual(["zz action"]));
    expect(server.pageRequests.at(-1)?.has("cursor")).toBe(false);
    expect(count()).toBe("1 of 151 tags");
    // 入力は q の上限の 100 文字で止まる。
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.clear(search);
    await user.click(search);
    await user.paste("a".repeat(101));
    expect((search as HTMLInputElement).value).toBe("a".repeat(100));
  });

  it("「Unused only」で unused=true を送り、件数の行が total of totalAll になる（受け入れ条件8）", async () => {
    const user = userEvent.setup();
    server.tags = server.tags.map((item, index) => ({
      ...item,
      videoCount: index < 140 ? 1 : 0,
    }));
    install();
    renderPage();
    await screen.findByTitle(name(0));

    await toggleFilter(user, "Unused only");
    await screen.findByTitle(name(140));
    expect(server.pageRequests.at(-1)?.get("unused")).toBe("true");
    // 読み込んでいなかったタグも数えた数で、末尾まで読んだので「loaded」は無い。
    expect(count()).toBe("10 of 150 tags");
    expect(loadedNames()).toHaveLength(10);
  });

  it("末尾に近づくと cursor 付きの要求を 1 回送り、行を末尾に足して重複する id を捨てる", async () => {
    install();
    renderPage();
    await screen.findByTitle(name(0));
    // 読み込んだあとで、読み込んだ行の 1 つが別のタブで改名され、境より後ろに並ぶ
    // ようになった（続きのページにも同じ id が出る）。
    server.tags = server.tags.map((item) =>
      item.id === 6 ? { ...item, name: "Tag 120a" } : item,
    );

    await scrollToEnd(100);
    await waitFor(() => expect(count()).toBe("150 tags"));
    expect(cursorRequests()).toHaveLength(1);
    expect(cursorRequests()[0]?.get("limit")).toBe("100");
    expect(cursorRequests()[0]?.get("sort")).toBe("name");

    scrollTo(150 * 60);
    await waitFor(() =>
      expect(document.querySelector('[data-index="148"]')).not.toBeNull(),
    );
    // 150 行のまま（id 6 は二重に出ない）。続きはもう無いので要求しない。
    expect(document.querySelector('[data-index="149"]')).not.toBeNull();
    expect(document.querySelector('[data-index="150"]')).toBeNull();
    expect(screen.queryByTitle("Tag 120a")).toBeNull();
    expect(cursorRequests()).toHaveLength(1);
  });

  it("続きの応答の totalAll が違えば「一覧が変わった」を出して続きを止め、「Reload」で先頭から読み直す（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await user.click(within(rowOf(name(0))).getByRole("checkbox"));

    // 別のタブでタグが増えた。
    server.tags.push(tag({ id: 500, name: "Tag 999" }));
    await scrollToEnd(100);

    expect(
      await screen.findByText(
        "Tags were added or removed elsewhere, so the rest of this list may be out of date.",
      ),
    ).toBeDefined();
    // 行と選択は残り、続きは足さず、それ以上要求しない。選んでいる間は見出しの件数の
    // 代わりにタブの件数を見る。
    expect(tagsTab().textContent).toBe("Tags150");
    expect(
      within(screen.getByRole("region", { name: "Selected tags" })).getByText(
        "1 tag selected",
      ),
    ).toBeDefined();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(cursorRequests()).toHaveLength(1);

    const gets = server.getCalls;
    scrollTo(0);
    await user.click(screen.getByRole("button", { name: "Reload" }));
    await waitFor(() => expect(count()).toBe("151 tags"));
    expect(server.getCalls).toBe(gets + 1);
    expect(server.pageRequests.at(-1)?.has("cursor")).toBe(false);
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    expect(screen.queryByText(/Tags were added or removed elsewhere/)).toBeNull();
  });

  it("続きの読み込みに失敗すると読み込んだ行が残り、「Retry」で同じ cursor を送る（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    server.failNextGet = true;
    await scrollToEnd(100);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/^Couldn't load more: /);
    expect(screen.getByTitle(name(99))).toBeDefined();
    expect(count()).toBe("150 tags");

    await user.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(count()).toBe("150 tags"));
    const sent = cursorRequests();
    expect(sent).toHaveLength(2);
    expect(sent[1]?.get("cursor")).toBe(sent[0]?.get("cursor"));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("続きを待つ間に検索を変えると、古い条件の続きは一覧に混ざらない（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    // 続き（2 回目の要求）から止める。
    hold.getsFrom = 2;
    await scrollToEnd(100);
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));
    expect(screen.getByRole("status", { name: "Loading more tags…" })).toBeDefined();

    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag 14");
    await waitFor(() => expect(heldGetReleases.length).toBeGreaterThan(1));
    scrollTo(0);
    // 新しい条件の応答を先に、古い続きの応答をあとに解く。
    for (const release of heldGetReleases.slice(1)) release();
    heldGetReleases[0]!();
    await waitFor(() =>
      expect(loadedNames()).toEqual([
        "Tag 14",
        "Tag 140",
        "Tag 141",
        "Tag 142",
        "Tag 143",
        "Tag 144",
        "Tag 145",
        "Tag 146",
        "Tag 147",
        "Tag 148",
        "Tag 149",
      ]),
    );
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(loadedNames()).toHaveLength(11);
    expect(count()).toBe("11 of 150 tags");
    expect(screen.queryByText(/Couldn't load more/)).toBeNull();
  });

  it("先頭のチェックは読み込んだ行だけを選び、続きがあってもそれ以上は選ばない（要件10）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    const header = screen.getByRole("checkbox", { name: "Select all 100 loaded tags" });
    await user.click(header);
    const bar = screen.getByRole("region", { name: "Selected tags" });
    expect(within(bar).getByText("100 tags selected")).toBeDefined();
    expect(header.getAttribute("aria-checked")).toBe("true");

    // 続きが届くと読み込んだ行が増え、全部の状態は中間に戻る。
    await scrollToEnd(100);
    await waitFor(() =>
      expect(
        screen.getByRole("checkbox", { name: "Select all 150 loaded tags" }),
      ).toBeDefined(),
    );
    expect(within(bar).getByText("100 tags selected")).toBeDefined();
    expect(
      screen
        .getByRole("checkbox", { name: "Select all 150 loaded tags" })
        .getAttribute("aria-checked"),
    ).toBe("mixed");
  });

  it("名前の順で作成したタグは名前の自然順の鍵の位置に入る（受け入れ条件6）", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "B1" }),
      tag({ id: 2, name: "B2" }),
      tag({ id: 3, name: "B10" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("B10");
    expect(loadedNames()).toEqual(["B1", "B2", "B10"]);

    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(screen.getByRole("textbox", { name: "New tag name" }), "ｂ５");
    await user.keyboard("{Enter}");
    await screen.findByTitle("ｂ５");
    expect(loadedNames()).toEqual(["B1", "B2", "ｂ５", "B10"]);
    expect(count()).toBe("4 tags");
    expect(server.getCalls).toBe(1);
  });

  it("検索中に改名して一致しなくなった行は取り除かれ、total が減る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag 14");
    await waitFor(() => expect(count()).toBe("11 of 150 tags"));

    await user.click(within(rowOf("Tag 141")).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "Tag 141"' });
    await user.clear(input);
    await user.type(input, "Renamed");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(screen.queryByTitle("Renamed")).toBeNull());
    expect(screen.queryByTitle("Tag 141")).toBeNull();
    expect(count()).toBe("10 of 150 tags");
  });

  it("先頭のページの失敗で、一覧を持っていなければ失敗の表示と「Retry」になる（Edge Case）", async () => {
    const user = userEvent.setup();
    server.failNextGet = true;
    install();
    renderPage();
    expect(await screen.findByText("Couldn't load the tags")).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByTitle(name(0));
    expect(screen.queryByText("Couldn't load the tags")).toBeNull();
  });

  it("先頭のページの失敗で、一覧を持っていればその一覧を残して帯の中に知らせ、その間は続きを読まない（Edge Case）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));

    server.failNextGet = true;
    await toggleFilter(user, "Unused only");
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(
      /^Couldn't load tags: .*\. The list below may not match the current search, filters and sort\./,
    );
    // 前の行と件数を残す。
    expect(screen.getByTitle(name(0))).toBeDefined();
    expect(count()).toBe("150 tags");

    // 箱がある間は、末尾に近づいても前の条件のカーソルで続きを読まない。
    await scrollToEnd(100);
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(cursorRequests()).toHaveLength(0);

    await user.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(count()).toBe("150 of 150 tags"));
    expect(server.pageRequests.at(-1)?.get("unused")).toBe("true");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("「Tentative only」で読み込んだ行を全部確定すると、空の状態を出さずにその場で続きを読み、残りの仮のタグが仮のまま出る（受け入れ条件10）", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 250 }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: index % 2 === 0 }),
    );
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await toggleFilter(user, "Tentative only");
    await waitFor(() => expect(count()).toBe("125 of 250 tags"));

    await user.click(
      screen.getByRole("checkbox", { name: "Select all 100 loaded tags" }),
    );
    await user.click(
      within(screen.getByRole("region", { name: "Selected tags" })).getByRole("button", {
        name: "Confirm",
      }),
    );

    expect(await screen.findByText("Confirmed 100 tags")).toBeDefined();
    expect(server.batchCalls[0]?.ids).toHaveLength(100);
    await waitFor(() => expect(count()).toBe("25 of 250 tags"));
    expect(cursorRequests()).toHaveLength(1);
    expect(screen.queryByText("No tentative tags")).toBeNull();
    expect(loadedNames()[0]).toBe(name(200));
    expect(within(rowOf(name(200))).getByText("Tentative")).toBeDefined();
  });

  it("先頭のページを待つ間に削除すると、削除の前に送った応答は捨てて読み直し、消したタグが戻らない（R-12）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(1));

    // 「Unused only」の先頭のページ（2 回目の要求）から止める。全部のタグが 0 本。
    hold.getsFrom = 2;
    await toggleFilter(user, "Unused only");
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));

    await user.click(
      within(rowOf(name(1))).getByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", {
      name: `Delete "${name(1)}"`,
    });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByTitle(name(1))).toBeNull());

    // 削除の前に受けた先頭のページ（消したタグを含む）が届く。
    heldGetReleases[0]!();
    await waitFor(() => expect(heldGetReleases).toHaveLength(2));
    expect(server.pageRequests.at(-1)?.get("unused")).toBe("true");
    expect(server.pageRequests.at(-1)?.has("cursor")).toBe(false);
    expect(screen.queryByTitle(name(1))).toBeNull();

    heldGetReleases[1]!();
    await waitFor(() => expect(count()).toBe("149 of 149 tags"));
    expect(screen.queryByTitle(name(1))).toBeNull();
  });

  it("続きを待つ間に確定すると、確定の前に送った続きは捨てて同じ cursor で読み直し、件数が戻らない（R-12）", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 250 }, (_, index) =>
      tag({ id: index + 1, name: name(index), tentative: index % 2 === 0 }),
    );
    install();
    renderPage();
    await screen.findByTitle(name(0));
    await toggleFilter(user, "Tentative only");
    await waitFor(() => expect(count()).toBe("125 of 250 tags"));

    hold.getsFrom = server.getCalls + 1;
    await scrollToEnd(100);
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));
    scrollTo(0);
    await screen.findByTitle(name(0));

    await user.click(within(rowOf(name(0))).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(screen.queryByTitle(name(0))).toBeNull());
    expect(count()).toBe("124 of 250 tags");

    // 確定の前に受けた続き（total 125）が届く。
    heldGetReleases[0]!();
    await waitFor(() => expect(heldGetReleases).toHaveLength(2));
    const sent = cursorRequests();
    expect(sent).toHaveLength(2);
    expect(sent[1]?.get("cursor")).toBe(sent[0]?.get("cursor"));
    expect(count()).toBe("124 of 250 tags");

    heldGetReleases[1]!();
    await waitFor(() => expect(count()).toBe("124 of 250 tags"));
    expect(screen.queryByText(/Tags were added or removed elsewhere/)).toBeNull();
  });

  it("並び順を変えて読み込んだ範囲の外になった改名中の行を改名しても、件数は増えない", async () => {
    const user = userEvent.setup();
    // 本数の多い順では Tag 0（0 本）が最後に並び、先頭のページに入らない。
    server.tags = server.tags.map((item, index) => ({
      ...item,
      tentative: false,
      videoCount: index,
    }));
    install();
    renderPage();
    await screen.findByTitle(name(0));
    // 件数の行が total を出すよう、全部に合う検索で絞る。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag");
    await waitFor(() => expect(count()).toBe("150 of 150 tags"));

    await user.click(within(rowOf(name(0))).getByRole("button", { name: "Rename" }));
    await screen.findByRole("textbox", { name: `New name for "${name(0)}"` });
    // 並び順を変えた先頭のページの次（改名中の行が末尾にあるので続きを読む）は届かない
    // ままにし、改名中の行を読み込んでいない間に改名する。
    hold.getsFrom = server.getCalls + 2;
    await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
    await waitFor(() =>
      expect(server.pageRequests.at(-1)?.get("sort")).toBe("countDesc"),
    );
    await waitFor(() => expect(loadedNames()[0]).toBe(name(149)));
    expect(count()).toBe("150 of 150 tags");

    const input = screen.getByRole("textbox", { name: `New name for "${name(0)}"` });
    await user.clear(input);
    await user.type(input, "Tag renamed{Enter}");
    await waitFor(() =>
      expect(server.tags.find((item) => item.id === 1)?.name).toBe("Tag renamed"),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("textbox", { name: `New name for "${name(0)}"` }),
      ).toBeNull(),
    );
    expect(count()).toBe("150 of 150 tags");
  });

  it("シノニム登録に伴って読み込んでいないタグを統合すると、条件に合っていたそのタグを total からも引く", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(1));
    // 件数の行が total を出すよう、全部に合う検索で絞る。
    await user.type(screen.getByRole("searchbox", { name: "Search tags" }), "Tag");
    await waitFor(() => expect(count()).toBe("150 of 150 tags"));
    expect(screen.queryByTitle(name(140))).toBeNull();

    await user.click(within(rowOf(name(1))).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", {
      name: `Synonyms of "${name(1)}"`,
    });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Add synonym" }),
      `${name(140)}{Enter}`,
    );
    await within(dialog).findByText(/is a tag on/);
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    await waitFor(() =>
      expect(server.tags.some((item) => item.name === name(140))).toBe(false),
    );
    await within(dialog).findByText(name(140));
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(count()).toBe("149 of 149 tags"));
  });

  it("新しい検索の先頭のページを待つ間、空の状態は読んだ検索の語のまま出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle(name(0));
    const box = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(box, "zzz");
    expect(await screen.findByText('No tags match "zzz"')).toBeDefined();

    hold.getsFrom = server.getCalls + 1;
    await user.type(box, "q");
    await waitFor(() => expect(heldGetReleases).toHaveLength(1));
    expect(screen.getByText('No tags match "zzz"')).toBeDefined();
    expect(screen.queryByText('No tags match "zzzq"')).toBeNull();

    heldGetReleases[0]!();
    expect(await screen.findByText('No tags match "zzzq"')).toBeDefined();
  });

  it("読み込んでいない統合先へ統合すると、並び順の位置が範囲の中なら差し込み、共有の保持の取り直しが失敗しても統合元と統合先が残らず消えたりしない（R-12）", async () => {
    const user = userEvent.setup();
    // 本数の多い順で Tag 0（5 本）が先頭、残りは 1 本で名前の順。Tag 140 は読み込んでいない。
    server.tags = server.tags.map((item, index) => ({
      ...item,
      tentative: false,
      videoCount: index === 0 ? 5 : 1,
    }));
    install();
    // 共有の保持の購読者がいて、統合のあとの全件の取り直しは失敗する。
    server.failFullGets = true;
    const unsubscribe = subscribeTags(() => undefined);
    try {
      renderPage();
      await screen.findByTitle(name(0));
      await user.click(screen.getByRole("button", { name: "Sort by: Name" }));
      await user.click(await screen.findByRole("menuitemradio", { name: "Video count" }));
      await waitFor(() =>
        expect(server.pageRequests.at(-1)?.get("sort")).toBe("countDesc"),
      );
      await waitFor(() => expect(loadedNames()[1]).toBe(name(1)));
      expect(screen.queryByTitle(name(140))).toBeNull();

      await user.click(
        within(rowOf(name(0))).getByRole("button", { name: "More actions" }),
      );
      await user.click(
        await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
      );
      const dialog = await screen.findByRole("dialog", { name: `Merge "${name(0)}"` });
      await user.type(
        within(dialog).getByRole("combobox", { name: "Tag to merge into" }),
        name(140),
      );
      await user.click(
        await within(dialog).findByRole("option", { name: new RegExp(`^${name(140)}`) }),
      );
      await user.click(within(dialog).getByRole("button", { name: "Merge" }));

      expect(
        await screen.findByText(`Merged "${name(0)}" into "${name(140)}"`),
      ).toBeDefined();
      await waitFor(() => expect(server.fullGetCalls).toBeGreaterThan(0));
      // 統合先は 6 本で先頭に入り、統合元は消える。件数は 1 つ減る。
      await waitFor(() => expect(loadedNames()[0]).toBe(name(140)));
      expect(screen.queryByTitle(name(0))).toBeNull();
      expect(count()).toBe("149 tags");
      await waitFor(() =>
        expect(document.activeElement).toBe(
          screen.getByRole("link", { name: `Open the library filtered by ${name(140)}` }),
        ),
      );

      // 続きを末尾まで読んでも、統合先は二重に出ない。
      await scrollToEnd(100);
      await waitFor(() => expect(count()).toBe("149 tags"));
      scrollTo(0);
      await waitFor(() => expect(loadedNames()[0]).toBe(name(140)));
      expect(screen.getAllByTitle(name(140))).toHaveLength(1);
    } finally {
      unsubscribe();
    }
  });
});
