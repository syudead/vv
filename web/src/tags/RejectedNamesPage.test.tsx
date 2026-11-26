import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { install, renderPage, server, setUpTagsPageServer } from "../testing/tagScreens";

setUpTagsPageServer();

describe("RejectedNamesPage 却下した名前のページ", () => {
  const observers = new Set<{
    callback: IntersectionObserverCallback;
    target: Element | null;
  }>();

  function scrollToEnd() {
    act(() => {
      for (const observer of [...observers]) {
        if (observer.target === null) continue;
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

  function rows(list: HTMLElement): HTMLElement[] {
    return [...list.querySelectorAll<HTMLElement>("[data-rejected-name]")];
  }

  function allowButton(list: HTMLElement, name: string): HTMLElement {
    return within(within(list).getByTitle(name)).getByRole("button", {
      name: `Allow "${name}" again`,
    });
  }

  async function openPage(): Promise<HTMLElement> {
    install();
    renderPage({ url: "/tags/rejected" });
    const list = await screen.findByRole("region", { name: "All rejected names" });
    await waitFor(() => expect(rows(list).length).toBeGreaterThan(0));
    return list;
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
    server.rejectedNames = [...names];
  });

  it("開くと先頭の1ページだけを limit=100 で読み、戻るリンクと説明を出す", async () => {
    const list = await openPage();

    expect(rows(list)).toHaveLength(100);
    expect(server.rejectedGetRequests).toHaveLength(1);
    expect(server.rejectedGetRequests[0]?.get("limit")).toBe("100");
    expect(server.rejectedGetRequests[0]?.has("cursor")).toBe(false);
    expect(screen.getByRole("link", { name: "Tags" }).getAttribute("href")).toBe("/tags");
    expect(
      screen.getByText(
        "Automatic tagging won't create these names. Allow a name again to let it be created.",
      ),
    ).toBeDefined();
  });

  it(
    "末尾までスクロールすると cursor 付きで続きを読み、名前を足す。最後まで読めば見張らない",
    async () => {
      const list = await openPage();

      scrollToEnd();
      await waitFor(() => expect(rows(list)).toHaveLength(200));
      expect(server.rejectedGetRequests[1]?.get("cursor")).toBe("Name099");
      expect(server.rejectedGetRequests[1]?.get("limit")).toBe("100");

      // Wait until the effect observes the new last row.
      await waitFor(() => {
        scrollToEnd();
        expect(rows(list)).toHaveLength(250);
      });
      expect(within(list).getByTitle("Name249")).toBeDefined();
      expect(observers.size).toBe(0);
    },
    heavyTimeout,
  );

  it("「Allow again」で DELETE を送り、その名前がページから消えて次の名前へフォーカスが移る。最後の1つなら見出しへ移る", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old", "Stale"];
    const fetchMock = install();
    renderPage({ url: "/tags/rejected" });
    const list = await screen.findByRole("region", { name: "All rejected names" });
    await within(list).findByTitle("Old");

    await user.click(allowButton(list, "Old"));
    await waitFor(() => expect(within(list).queryByTitle("Old")).toBeNull());
    expect(
      fetchMock.mock.calls.some(
        ([input, init]) =>
          init?.method === "DELETE" &&
          String(input).startsWith("/api/tags/rejected-names?"),
      ),
    ).toBe(true);
    expect(server.rejectedNames).toEqual(["Stale"]);
    await waitFor(() => expect(document.activeElement).toBe(allowButton(list, "Stale")));

    await user.click(allowButton(list, "Stale"));
    expect(await screen.findByText("No rejected names")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("heading", { level: 1, name: "Rejected names" }),
      ),
    );
    expect(server.rejectedGetRequests).toHaveLength(1);
  });

  it("「Allow again」が失敗したら理由をページの中に出し、名前を残す", async () => {
    const user = userEvent.setup();
    server.rejectedNames = ["Old"];
    install();
    renderPage({ url: "/tags/rejected" });
    const list = await screen.findByRole("region", { name: "All rejected names" });
    await within(list).findByTitle("Old");

    server.nextError = { status: 500, body: { code: "internal", message: "failed" } };
    await user.click(allowButton(list, "Old"));

    expect(
      await within(list).findByText(
        'Couldn\'t remove "Old": Something went wrong on the server.',
      ),
    ).toBeDefined();
    expect(within(list).getByTitle("Old")).toBeDefined();
  });

  it("読み込みに失敗したら理由と Retry を出し、Retry で読み直す", async () => {
    const user = userEvent.setup();
    server.failRejectedGets = true;
    server.rejectedNames = ["Old"];
    install();
    renderPage({ url: "/tags/rejected" });

    expect(await screen.findByText("Couldn't load the rejected names")).toBeDefined();
    server.failRejectedGets = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("button", { name: 'Allow "Old" again' }),
    ).toBeDefined();
  });

  it("続きの読み込みに失敗しても読み込んだ名前を残し、Retry で同じカーソルから読み直す", async () => {
    const user = userEvent.setup();
    const list = await openPage();

    server.failRejectedMoreGets = true;
    scrollToEnd();
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
    "読み込んだ名前をすべて外しても続きが残っていれば、空の文言を出さず、見張りで続きを読む",
    async () => {
      server.rejectedNames = names.slice(0, 101);
      const list = await openPage();

      // Batch clicks in one act to avoid quadratic rerenders of intermediate states nobody observes.
      act(() => {
        for (const name of names.slice(0, 100)) {
          fireEvent.click(within(list).getByTitle(name).querySelector("button")!);
        }
      });
      await waitFor(() => expect(rows(list)).toHaveLength(0));
      expect(screen.queryByText("No rejected names")).toBeNull();

      await waitFor(() => {
        scrollToEnd();
        expect(within(list).getByTitle("Name100")).toBeDefined();
      });
    },
    heavyTimeout,
  );
});
