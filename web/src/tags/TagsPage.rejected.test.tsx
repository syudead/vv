import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  tag,
  server,
  rejectedGetReleases,
  rejectedDeleteReleases,
  hold,
  install,
  renderPage,
  rejectedTab,
  tagsTab,
  openRejectedTab,
  setUpTagsPageServer,
} from "../testing/tagsPage";

setUpTagsPageServer();

describe("TagsPage 却下した名前のページ", () => {
  const observers = new Set<{
    callback: IntersectionObserverCallback;
    target: Element | null;
  }>();

  function scrollRejectedToEnd(skip: ReadonlySet<object> = new Set()) {
    act(() => {
      for (const observer of [...observers]) {
        if (observer.target === null || skip.has(observer)) continue;
        observer.callback(
          [
            {
              isIntersecting: true,
              target: observer.target,
            } as IntersectionObserverEntry,
          ],
          {} as IntersectionObserver,
        );
      }
    });
  }

  // Large rendered-row cases need extra time under two-core CI coverage.
  const heavyTimeout = 15_000;

  const names = Array.from(
    { length: 250 },
    (_, index) => `Name${String(index).padStart(3, "0")}`,
  );

  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  /** Avoid repeated global role queries across hundreds of rows; the first case verifies row semantics. */
  function rows(list: HTMLElement): HTMLElement[] {
    return [...list.querySelectorAll<HTMLElement>("tbody > tr")];
  }

  function allowButton(list: HTMLElement, name: string): HTMLElement {
    return within(within(list).getByTitle(name)).getByRole("button", {
      name: `Allow "${name}" again`,
    });
  }

  function bar(): HTMLElement {
    return screen.getByRole("region", { name: "Selected tags" });
  }

  beforeEach(() => {
    observers.clear();
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        private entry: { callback: IntersectionObserverCallback; target: Element | null };
        constructor(callback: IntersectionObserverCallback) {
          this.entry = { callback, target: null };
        }
        observe(target: Element) {
          this.entry.target = target;
          observers.add(this.entry);
        }
        disconnect() {
          observers.delete(this.entry);
        }
      },
    );
    server.tags = [
      tag({ id: 1, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 2, name: "Beta", tentative: true }),
    ];
    server.rejectedNames = [...names];
  });

  const openRejected = openRejectedTab;

  async function backToTags(user: ReturnType<typeof userEvent.setup>) {
    await user.click(tagsTab());
    await screen.findByTitle("Alpha");
  }

  it("開くと先頭の1ページだけを limit=100 で読み、タブに total を出す。タブを開くために読み直さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("250"));
    expect(server.rejectedGetRequests).toHaveLength(1);
    expect(server.rejectedGetRequests[0]?.get("limit")).toBe("100");
    expect(server.rejectedGetRequests[0]?.has("cursor")).toBe(false);

    const list = await openRejected(user);
    expect(within(list).getAllByRole("row")).toHaveLength(100);
    expect(server.rejectedGetRequests).toHaveLength(1);
  });

  it(
    "タブの並びを末尾までスクロールすると cursor 付きで続きを読み、名前を足す。タブを離れても続きは残る",
    async () => {
      const user = userEvent.setup();
      install();
      renderPage();
      await screen.findByTitle("Alpha");
      const list = await openRejected(user);

      scrollRejectedToEnd();
      await waitFor(() => expect(rows(list)).toHaveLength(200));
      expect(server.rejectedGetRequests[1]?.get("cursor")).toBe("Name099");
      expect(server.rejectedGetRequests[1]?.get("limit")).toBe("100");
      expect(list.getAttribute("aria-busy")).toBeNull();

      // Wait until the effect reobserves the sentinel after rendering the new page.
      await waitFor(() => {
        scrollRejectedToEnd();
        expect(rows(list)).toHaveLength(250);
      });
      expect(within(list).getByTitle("Name249")).toBeDefined();
      expect(observers.size).toBe(0);
      const requests = server.rejectedGetRequests.length;

      await backToTags(user);
      const reopened = await openRejected(user);
      expect(rows(reopened)).toHaveLength(250);
      expect(server.rejectedGetRequests).toHaveLength(requests);
    },
    heavyTimeout,
  );

  it("×で外すと DELETE を送り、その名前が消えて入口の件数が1減る。一覧は取り直さない", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    renderPage();
    await screen.findByTitle("Alpha");
    const entry = rejectedTab();
    await waitFor(() => expect(entry.textContent).toContain("250"));
    const list = await openRejected(user);

    await user.click(allowButton(list, "Name000"));
    await waitFor(() => expect(within(list).queryByTitle("Name000")).toBeNull());
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) =>
          init?.method === "DELETE" &&
          String(input).startsWith("/api/tags/rejected-names?"),
      ),
    ).toBe(true);
    expect(entry.textContent).toContain("249");
    expect(rows(list)).toHaveLength(99);
    expect(server.rejectedGetRequests).toHaveLength(1);
  });

  it(
    "まとめての却下のあと、先頭の1ページだけを取り直し、窓の並びが先頭のページに戻る",
    async () => {
      const user = userEvent.setup();
      install();
      renderPage();
      await screen.findByTitle("Alpha");
      const list = await openRejected(user);
      scrollRejectedToEnd();
      await waitFor(() => expect(rows(list)).toHaveLength(200));
      await backToTags(user);

      const before = server.rejectedGetRequests.length;
      await user.click(within(rowOf("Alpha")).getByRole("checkbox"));
      await user.click(within(bar()).getByRole("button", { name: "Reject…" }));
      const dialog = await screen.findByRole("alertdialog", {
        name: "Reject selected tags",
      });
      const reject = within(dialog).getByRole("button", { name: "Reject" });
      await waitFor(() => expect((reject as HTMLButtonElement).disabled).toBe(false));
      await user.click(reject);
      expect(await screen.findByText("Rejected 1 tag")).toBeDefined();

      const entry = rejectedTab();
      await waitFor(() => expect(entry.textContent).toContain("251"));
      const reloads = server.rejectedGetRequests.slice(before);
      expect(reloads).toHaveLength(1);
      expect(reloads[0]?.has("cursor")).toBe(false);
      expect(reloads[0]?.get("limit")).toBe("100");

      const reopened = await openRejected(user);
      expect(rows(reopened)).toHaveLength(100);
      expect(within(reopened).getByTitle("Alpha")).toBeDefined();
    },
    heavyTimeout,
  );

  it("続きの読み込みに失敗しても読み込んだ名前を残し、Retry で同じカーソルから読み直す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);

    server.failRejectedMoreGets = true;
    scrollRejectedToEnd();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Couldn't load more rejected namesRetry");
    expect(rows(list)).toHaveLength(100);
    expect(observers.size).toBe(0);

    server.failRejectedMoreGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(rows(list)).toHaveLength(200));
    const more = server.rejectedGetRequests.filter((params) => params.has("cursor"));
    expect(more.map((params) => params.get("cursor"))).toEqual(["Name099", "Name099"]);
    expect(screen.queryByText("Couldn't load more rejected names")).toBeNull();
  });
  it(
    "読み込んだ名前をすべて外しても続きが残っていれば、空の文言ではなく続きの失敗と Retry を残す",
    async () => {
      const user = userEvent.setup();
      server.rejectedNames = names.slice(0, 101);
      install();
      renderPage();
      await screen.findByTitle("Alpha");
      const list = await openRejected(user);

      server.failRejectedMoreGets = true;
      scrollRejectedToEnd();
      expect((await screen.findByRole("alert")).textContent).toBe(
        "Couldn't load more rejected namesRetry",
      );
      // Batch clicks in one act to avoid quadratic rerenders of intermediate states nobody observes.
      act(() => {
        for (const name of names.slice(0, 100)) {
          fireEvent.click(within(list).getByTitle(name).querySelector("button")!);
        }
      });
      await waitFor(() => expect(rows(list)).toHaveLength(0));
      expect(screen.queryByText("No rejected names")).toBeNull();

      server.failRejectedMoreGets = false;
      await user.click(screen.getByRole("button", { name: "Retry" }));
      expect(await within(list).findByTitle("Name100")).toBeDefined();
    },
    heavyTimeout,
  );

  it(
    "読み込んだ名前をすべて外しても続きが残っていれば、番兵で続きを読む",
    async () => {
      const user = userEvent.setup();
      server.rejectedNames = names.slice(0, 101);
      install();
      renderPage();
      await screen.findByTitle("Alpha");
      const list = await openRejected(user);

      // Batch clicks in one act to avoid quadratic rerenders of intermediate states nobody observes.
      act(() => {
        for (const name of names.slice(0, 100)) {
          fireEvent.click(within(list).getByTitle(name).querySelector("button")!);
        }
      });
      await waitFor(() => expect(rows(list)).toHaveLength(0));
      expect(screen.queryByText("No rejected names")).toBeNull();

      scrollRejectedToEnd();
      expect(await within(list).findByTitle("Name100")).toBeDefined();
    },
    heavyTimeout,
  );

  it("先頭のページの取り直しの間に番兵が見えても、取り直しのあとで見張り直して続きを読む", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");
    const list = await openRejected(user);
    await backToTags(user);

    hold.rejectedGets = true;
    await user.click(within(rowOf("Beta")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    await waitFor(() => expect(rejectedGetReleases).toHaveLength(1));
    hold.rejectedGets = false;

    const entry = rejectedTab();
    const reopened = await openRejected(user);
    scrollRejectedToEnd();
    expect(
      server.rejectedGetRequests.filter((params) => params.has("cursor")),
    ).toHaveLength(0);
    expect(list.isConnected).toBe(false);

    // Unchanged row counts leave the visible sentinel without another event unless it is reobserved.
    const watching = new Set(observers);
    act(() => rejectedGetReleases.forEach((resolve) => resolve()));
    await waitFor(() => expect(entry.textContent).toContain("251"));
    await waitFor(() =>
      expect([...observers].some((observer) => !watching.has(observer))).toBe(true),
    );
    scrollRejectedToEnd(watching);
    await waitFor(() => expect(rows(reopened)).toHaveLength(200));
  });

  it(
    "取り直しと重なった続きの名前の取り外しのあと、取り直して入口の件数を合わせる",
    async () => {
      const user = userEvent.setup();
      install();
      renderPage();
      await screen.findByTitle("Alpha");
      const entry = rejectedTab();
      const list = await openRejected(user);
      scrollRejectedToEnd();
      await waitFor(() => expect(rows(list)).toHaveLength(200));
      await backToTags(user);

      hold.rejectedGets = true;
      await user.click(
        within(rowOf("Beta")).getByRole("button", { name: "More actions" }),
      );
      await user.click(await screen.findByRole("menuitem", { name: "Reject…" }));
      await user.click(await screen.findByRole("button", { name: "Reject" }));
      await waitFor(() => expect(rejectedGetReleases).toHaveLength(1));
      hold.rejectedGets = false;

      const reopened = await openRejected(user);
      hold.rejectedDeletes = true;
      await user.click(allowButton(reopened, "Name150"));
      await waitFor(() => expect(rejectedDeleteReleases).toHaveLength(1));
      hold.rejectedDeletes = false;

      act(() => rejectedGetReleases.forEach((resolve) => resolve()));
      await waitFor(() => expect(entry.textContent).toContain("251"));
      const before = server.rejectedGetRequests.length;

      act(() => rejectedDeleteReleases.forEach((resolve) => resolve()));
      await waitFor(() => expect(entry.textContent).toContain("250"));
      expect(server.rejectedGetRequests.length).toBe(before + 1);
      expect(server.rejectedGetRequests.at(-1)?.has("cursor")).toBe(false);
    },
    heavyTimeout,
  );
});
