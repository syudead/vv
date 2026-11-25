import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { emitServerEvent } from "../api/fakeEventSource";
import { confirmTag } from "../api/tags";
import {
  historyBack,
  hold,
  install,
  kindsLink,
  probe,
  renderPage,
  server,
  setUpTagsPageServer,
  tag,
} from "../testing/tagScreens";

setUpTagsPageServer();

const genre = { id: 1, name: "Genre", cardFirst: false };

/** List pages, as opposed to the one-row tally reads and the change check's re-reads. */
function listPageRequests(): URLSearchParams[] {
  return server.pageRequests.filter((params) => params.get("limit") === "100");
}

function rereads(): URLSearchParams[] {
  return server.pageRequests.filter(
    (params) => params.get("limit") !== "100" && params.get("limit") !== "1",
  );
}

function card(name: string): HTMLElement {
  return screen.getByTitle(name).closest<HTMLElement>("[data-tag-id]")!;
}

/** Nine tags read three at a time, so every page loads and the last cards are past the first page. */
function nineTagsInPagesOfThree() {
  server.tags = Array.from({ length: 9 }, (_, index) =>
    tag({ id: index + 1, name: `Tag${String(index + 1)}` }),
  );
  server.pageLimitOverride = 3;
}

describe("TagsPage 条件と URL", () => {
  it("URL の条件で読み、種類を押すと URL に積み、戻ると前の条件で読み直す", async () => {
    const user = userEvent.setup();
    server.tags = [tag({ id: 9, name: "Robot", kind: genre, tentative: true })];
    install();
    renderPage({ url: "/tags?q=rob&tentative=1&kind=1&sort=countDesc" });

    await screen.findByTitle("Robot");
    const first = listPageRequests()[0]!;
    expect(Object.fromEntries(first)).toMatchObject({
      q: "rob",
      tentative: "true",
      kind: "1",
      sort: "countDesc",
    });
    const filter = screen.getByRole("radiogroup", { name: "Kind" });
    expect(
      within(filter)
        .getByRole("radio", { name: /^Genre/ })
        .getAttribute("aria-checked"),
    ).toBe("true");
    const tallies = screen.getByRole("group", { name: "What is left to tidy" });
    expect(
      within(tallies)
        .getByRole("button", { name: /Tentative/ })
        .getAttribute("aria-pressed"),
    ).toBe("true");

    await user.click(within(filter).getByRole("radio", { name: /^All/ }));
    await waitFor(() => expect(probe.search).toBe("?q=rob&tentative=1&sort=countDesc"));
    await waitFor(() => expect(listPageRequests().at(-1)!.has("kind")).toBe(false));

    await historyBack();
    expect(probe.search).toBe("?q=rob&tentative=1&kind=1&sort=countDesc");
    await waitFor(() => expect(listPageRequests().at(-1)!.get("kind")).toBe("1"));
  });

  it("仮・種類なし・未使用の数は語彙全体の数で、押すとその絞り込みになる", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Robot", kind: genre, videoCount: 2 }),
      tag({ id: 2, name: "Draft", tentative: true }),
      tag({ id: 3, name: "Plain", videoCount: 1 }),
    ];
    install();
    renderPage({ url: "/tags?q=rob" });
    const tallies = await screen.findByRole("group", { name: "What is left to tidy" });

    const noKind = within(tallies).getByRole("button", { name: /No kind/ });
    await waitFor(() => expect(noKind.textContent).toMatch(/^2/));
    expect(
      within(tallies).getByRole("button", { name: /Tentative/ }).textContent,
    ).toMatch(/^1/);
    expect(within(tallies).getByRole("button", { name: /Unused/ }).textContent).toMatch(
      /^1/,
    );

    await user.click(noKind);
    await waitFor(() => expect(probe.search).toBe("?q=rob&kind=none&sort=name"));
    expect(noKind.getAttribute("aria-pressed")).toBe("true");
    const filter = screen.getByRole("radiogroup", { name: "Kind" });
    expect(
      within(filter)
        .getByRole("radio", { name: /^No kind/ })
        .getAttribute("aria-checked"),
    ).toBe("true");
  });
});

describe("TagsPage 続きの読み込み", () => {
  it("描画する行が同じでも、切り替えた条件のページに続きがあれば読み込む", async () => {
    const user = userEvent.setup();
    server.tags = Array.from({ length: 6 }, (_, index) =>
      tag({
        id: index + 1,
        name: `Tag${String(index + 1)}`,
        kind: index < 3 ? genre : null,
      }),
    );
    server.pageLimitOverride = 3;
    install();
    renderPage({ url: "/tags?kind=1&sort=name" });
    await screen.findByTitle("Tag3");

    const filter = screen.getByRole("radiogroup", { name: "Kind" });
    await user.click(within(filter).getByRole("radio", { name: /^All/ }));

    expect(await screen.findByTitle("Tag6")).toBeDefined();
  });
});

describe("TagsPage キーボード", () => {
  it("Tab は描画されていない隣の行のカードへ一覧の順に進む", async () => {
    const user = userEvent.setup();
    // Rows measure as one card high, so only the rows near the top of the window are drawn.
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(function (
      this: HTMLElement,
    ) {
      return this.hasAttribute("data-index") ? 108 : 0;
    });
    server.tags = Array.from({ length: 40 }, (_, index) =>
      tag({ id: index + 1, name: `Tag${String(index + 1).padStart(2, "0")}` }),
    );
    install();
    renderPage({ url: "/tags?sort=name" });
    await screen.findByTitle("Tag01");
    const grid = screen.getByRole("region", { name: "Tags" });
    const drawn = [...grid.querySelectorAll<HTMLElement>("[data-index]")];
    const last = Number(drawn.at(-1)!.dataset.index);
    expect(last).toBeLessThan(39);
    const name = (row: number) => `Tag${String(row + 1).padStart(2, "0")}`;

    card(name(last)).focus();
    await user.tab();
    expect(document.activeElement).toBe(card(name(last + 1)));
  });
});

describe("TagsPage タグのページから戻る", () => {
  it("最初のページより先のカードから戻ると、読み込んだカードと位置をそれらのリクエストなしで戻す", async () => {
    const user = userEvent.setup();
    nineTagsInPagesOfThree();
    install();
    renderPage({ url: "/tags?sort=name" });
    await screen.findByTitle("Tag9");
    const pagesBefore = listPageRequests().length;
    expect(pagesBefore).toBe(3);

    vi.spyOn(window, "scrollY", "get").mockReturnValue(640);
    await user.click(card("Tag8"));
    expect(probe.pathname).toBe("/tags/8");

    await historyBack();
    expect(screen.getByTitle("Tag9")).toBeDefined();
    expect(listPageRequests()).toHaveLength(pagesBefore);
    expect(window.scrollTo).toHaveBeenCalledWith({ top: 640 });
  });

  it("タグのページで書き込んだあとに戻ると、同じ条件で先頭から読み直す", async () => {
    const user = userEvent.setup();
    nineTagsInPagesOfThree();
    install();
    renderPage({ url: "/tags?sort=name" });
    await screen.findByTitle("Tag9");
    const pagesBefore = listPageRequests().length;

    await user.click(card("Tag8"));
    await act(async () => {
      await confirmTag(8);
    });
    await historyBack();

    await waitFor(() => expect(listPageRequests().length).toBeGreaterThan(pagesBefore));
    const reread = listPageRequests()[pagesBefore]!;
    expect(reread.get("sort")).toBe("name");
    expect(reread.has("cursor")).toBe(false);
  });
});

describe("TagsPage 戻ったあとの読み直し", () => {
  it("戻ったあとに読み直した一覧は、別の画面から戻っても古いカードに戻らない", async () => {
    const user = userEvent.setup();
    nineTagsInPagesOfThree();
    install();
    renderPage({ url: "/tags?sort=name" });
    await screen.findByTitle("Tag9");
    await user.click(card("Tag8"));
    await historyBack();
    await screen.findByTitle("Tag9");

    server.tags[7]!.synonyms = ["Elsewhere"];
    await emitServerEvent("tags");
    await user.click(await screen.findByRole("button", { name: "Refresh" }));
    await waitFor(() =>
      expect(within(card("Tag8")).getByText("Elsewhere")).toBeDefined(),
    );

    await user.click(kindsLink());
    await historyBack();
    await waitFor(() =>
      expect(within(card("Tag8")).getByText("Elsewhere")).toBeDefined(),
    );
  });

  it("開発時に effect が繰り返されても、戻ったあとに件数と変更を確かめる", async () => {
    const user = userEvent.setup();
    nineTagsInPagesOfThree();
    install();
    renderPage({ url: "/tags?sort=name", strict: true });
    await screen.findByTitle("Tag9");
    await user.click(card("Tag8"));

    server.tags[7]!.synonyms = ["Elsewhere"];
    server.tags[0]!.tentative = true;
    await historyBack();

    expect(await screen.findByText("Tags changed elsewhere")).toBeDefined();
    const tallies = screen.getByRole("group", { name: "What is left to tidy" });
    await waitFor(() =>
      expect(
        within(tallies).getByRole("button", { name: /Tentative/ }).textContent,
      ).toMatch(/^1/),
    );
  });
});

describe("TagsPage 変更の通知", () => {
  it("最初のページより先のカードが読み直しで違えば通知を出し、カードは動かさない", async () => {
    const user = userEvent.setup();
    nineTagsInPagesOfThree();
    install();
    renderPage({ url: "/tags?sort=name" });
    await screen.findByTitle("Tag9");
    const pagesBefore = listPageRequests().length;

    server.tags[7]!.synonyms = ["Elsewhere"];
    await emitServerEvent("tags");

    expect(await screen.findByText("Tags changed elsewhere")).toBeDefined();
    expect(within(card("Tag8")).queryByText("Elsewhere")).toBeNull();
    expect(listPageRequests()).toHaveLength(pagesBefore);

    await user.click(screen.getByRole("button", { name: "Refresh" }));
    await waitFor(() =>
      expect(within(card("Tag8")).getByText("Elsewhere")).toBeDefined(),
    );
    expect(screen.queryByText("Tags changed elsewhere")).toBeNull();
  });

  it("Refresh は種類の一覧と件数も読み直し、消えた種類の絞り込みを外す", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "Robot", kind: genre }),
      tag({ id: 2, name: "Plain" }),
    ];
    install();
    renderPage({ url: "/tags?kind=1&sort=name" });
    await screen.findByTitle("Robot");
    const filter = screen.getByRole("radiogroup", { name: "Kind" });
    await waitFor(() =>
      expect(within(filter).getByRole("radio", { name: /^Genre/ }).textContent).toMatch(
        /1$/,
      ),
    );

    server.tagKinds = server.tagKinds.filter((item) => item.id !== 1);
    server.tags[0]!.kind = null;
    await emitServerEvent("tags");
    await user.click(await screen.findByRole("button", { name: "Refresh" }));

    await waitFor(() =>
      expect(within(filter).queryByRole("radio", { name: /^Genre/ })).toBeNull(),
    );
    await waitFor(() => expect(probe.search).toBe("?sort=name"));
  });

  it("読み直しが表示と同じなら何も出さない", async () => {
    nineTagsInPagesOfThree();
    install();
    renderPage({ url: "/tags?sort=name" });
    await screen.findByTitle("Tag9");

    await emitServerEvent("tags");
    await waitFor(() => expect(rereads().length).toBe(3));
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByText("Tags changed elsewhere")).toBeNull();
  });

  it("自分の書き込み中のイベントは、書き込みが済んでから確かめる", async () => {
    const user = userEvent.setup();
    install();
    renderPage({ url: "/tags" });
    await screen.findByTitle("Anime");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByRole("textbox", { name: "New tag name" }),
      "Anime",
    );
    hold.nextMutation = true;
    await user.click(within(dialog).getByRole("button", { name: "Create" }));
    await waitFor(() => expect(hold.release).not.toBeNull());

    server.tags[0]!.videoCount = 5;
    await emitServerEvent("tags");
    expect(rereads()).toHaveLength(0);

    await act(async () => {
      hold.release!();
      await Promise.resolve();
    });
    expect(
      await within(dialog).findByText('A tag named "Anime" already exists.'),
    ).toBeDefined();
    await waitFor(() => expect(rereads()).toHaveLength(1));
    expect(await screen.findByText("Tags changed elsewhere")).toBeDefined();
  });
});

describe("TagsPage 空の状態", () => {
  it("タグが 1 つもなければ、作る操作を出す", async () => {
    const user = userEvent.setup();
    server.tags = [];
    install();
    renderPage({ url: "/tags" });

    const empty = await screen.findByRole("heading", { name: "No tags yet" });
    await user.click(
      within(empty.closest<HTMLElement>('[data-state="empty"]')!).getByRole("button", {
        name: "New tag",
      }),
    );
    expect(await screen.findByRole("dialog", { name: "New tag" })).toBeDefined();
  });

  it("条件に合うタグがなければ、検索とすべての絞り込みを外す操作を出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage({ url: "/tags?q=zzz&unused=1&sort=name" });

    await screen.findByRole("heading", { name: 'No unused tags match "zzz"' });
    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    await waitFor(() => expect(probe.search).toBe("?sort=name"));
    expect(await screen.findByTitle("Anime")).toBeDefined();
  });
});

describe("TagsPage 新しいタグ", () => {
  it("作るとそのタグのページを開く", async () => {
    const user = userEvent.setup();
    install();
    renderPage({ url: "/tags" });
    await screen.findByTitle("Anime");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByRole("textbox", { name: "New tag name" }),
      "Robot",
    );
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    await waitFor(() => expect(probe.pathname).toBe("/tags/100"));
  });
});
