import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { emitServerEvent } from "../api/fakeEventSource";
import { type Audience, AudienceProvider } from "../auth/audience";
import {
  folderCard,
  revealCards,
  server,
  setUpSmartFolderServer,
} from "../testing/smartFolderPage";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ToastProvider } from "../ui/Toast";
import { SmartFolderCardsProvider } from "./cardValues";
import { SmartFolderCard, SmartFolderRow } from "./SmartFolderCard";

setUpSmartFolderServer();

function renderCards() {
  render(
    <MemoryRouter>
      <TooltipProvider>
        <AudienceProvider audience="owner">
          <SmartFolderCardsProvider>
            <SmartFolderCard folder={{ id: 1, name: "猫" }} backTo="/" />
            <SmartFolderCard folder={{ id: 2, name: "犬" }} backTo="/" />
          </SmartFolderCardsProvider>
        </AudienceProvider>
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function card(name: string) {
  const link = screen.getByRole("link", { name: new RegExp(`^${name},`) });
  return link.closest("article") as HTMLElement;
}

describe("スマートフォルダのカード", () => {
  it("要約から名前と骨組みの面を出し、画面に入ったカードだけ値を読む", async () => {
    server.cards.set(1, folderCard(1, "猫", { videoCount: 12 }));
    server.cards.set(2, folderCard(2, "犬"));
    renderCards();

    expect(screen.getByRole("link", { name: "猫, smart folder" })).toBeTruthy();
    expect(within(card("猫")).getByRole("heading", { name: "猫" })).toBeTruthy();
    expect(card("猫").querySelector('[data-slot="skeleton"]')).not.toBeNull();
    expect(server.cardRequests).toEqual([]);

    await revealCards([1]);

    expect(
      await screen.findByRole("link", { name: "猫, smart folder of 12 videos" }),
    ).toBeTruthy();
    expect(within(card("猫")).getByText("12 videos")).toBeTruthy();
    expect(within(card("猫")).getByTitle("Smart folder")).toBeTruthy();
    expect(screen.getByRole("link", { name: "犬, smart folder" })).toBeTruthy();
    expect(server.cardRequests).toEqual([[1]]);
    expect(card("猫").querySelector("a")?.getAttribute("href")).toBe("/smart-folders/1");
  });

  it("smartFolder の知らせで、読んだカードの値だけを 1 回読み直す", async () => {
    server.cards.set(1, folderCard(1, "猫", { videoCount: 2 }));
    server.cards.set(2, folderCard(2, "犬"));
    renderCards();
    await revealCards([1]);
    await screen.findByRole("link", { name: "猫, smart folder of 2 videos" });

    server.cards.set(1, folderCard(1, "猫", { videoCount: 3 }));
    await emitServerEvent("smartFolder", { ids: [1, 2] });

    expect(
      await screen.findByRole("link", { name: "猫, smart folder of 3 videos" }),
    ).toBeTruthy();
    expect(server.cardRequests).toEqual([[1], [1]]);
  });

  it("答えに入らなかったフォルダのカードは消える", async () => {
    renderCards();

    await revealCards();

    await waitFor(() => expect(screen.queryAllByRole("article")).toHaveLength(0));
    expect(server.cardRequests).toEqual([[1, 2]]);
  });

  it("同じカードを重ねて読んだら、後から送った読み出しの答えだけを使う", async () => {
    server.holdCards = true;
    server.cards.set(1, folderCard(1, "猫", { videoCount: 2 }));
    renderCards();
    await revealCards([1]);

    server.cards.set(1, folderCard(1, "猫", { videoCount: 3 }));
    await emitServerEvent("smartFolder", { ids: [1] });
    expect(server.cardRequests).toEqual([[1], [1]]);

    const [first, second] = server.heldCards;
    await act(async () => {
      second?.();
      await Promise.resolve();
    });
    expect(
      await screen.findByRole("link", { name: "猫, smart folder of 3 videos" }),
    ).toBeTruthy();

    await act(async () => {
      first?.();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(
      screen.getByRole("link", { name: "猫, smart folder of 3 videos" }),
    ).toBeTruthy();
  });

  it("読めなかったカードは骨組みのままで、つなぎ直すと求めたカードをすべて読み直す", async () => {
    server.failCards = true;
    server.cards.set(1, folderCard(1, "猫", { videoCount: 4 }));
    server.cards.set(2, folderCard(2, "犬", { videoCount: 1 }));
    renderCards();
    await revealCards([1]);
    expect(server.cardRequests).toEqual([[1]]);
    expect(card("猫").querySelector('[data-slot="skeleton"]')).not.toBeNull();

    server.failCards = false;
    await emitServerEvent("open");
    expect(server.cardRequests).toEqual([[1]]);
    await emitServerEvent("open");

    expect(
      await screen.findByRole("link", { name: "猫, smart folder of 4 videos" }),
    ).toBeTruthy();
    expect(server.cardRequests).toEqual([[1], [1]]);
  });

  it("見張りの無い環境では、描いたカードの値をすぐ読む", async () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    server.cards.set(1, folderCard(1, "猫", { videoCount: 6 }));
    server.cards.set(2, folderCard(2, "犬", { videoCount: 1 }));
    renderCards();

    expect(
      await screen.findByRole("link", { name: "猫, smart folder of 6 videos" }),
    ).toBeTruthy();
    expect(server.cardRequests).toEqual([[1, 2]]);
  });

  it("値の持ち主の外では何も読まない", async () => {
    render(
      <MemoryRouter>
        <TooltipProvider>
          <AudienceProvider audience="owner">
            <SmartFolderCard folder={{ id: 1, name: "猫" }} backTo="/" />
          </AudienceProvider>
        </TooltipProvider>
      </MemoryRouter>,
    );
    await revealCards();

    expect(screen.getByRole("link", { name: "猫, smart folder" })).toBeTruthy();
    expect(server.cardRequests).toEqual([]);
  });

  it("同じフォルダのカードが 2 枚あっても 1 回だけ求め、画面を離れたら読んでいる答えを捨てる", async () => {
    server.holdCards = true;
    server.cards.set(1, folderCard(1, "猫", { videoCount: 2 }));
    const { unmount } = render(
      <MemoryRouter>
        <TooltipProvider>
          <AudienceProvider audience="owner">
            <SmartFolderCardsProvider>
              <SmartFolderCard folder={{ id: 1, name: "猫" }} backTo="/" />
              <SmartFolderCard folder={{ id: 1, name: "猫" }} backTo="/" />
            </SmartFolderCardsProvider>
          </AudienceProvider>
        </TooltipProvider>
      </MemoryRouter>,
    );
    await revealCards();
    expect(server.cardRequests).toEqual([[1]]);

    unmount();
    await act(async () => {
      server.heldCards[0]?.();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("リストの行は表紙と、空のフォルダの薄い名前を出し、答えに無ければ消える", async () => {
    server.cards.set(
      1,
      folderCard(1, "猫", {
        videoCount: 0,
        previews: [{ videoId: 10, thumbnailUrl: "/api/videos/10/thumbnail" }],
      }),
    );
    render(
      <MemoryRouter>
        <TooltipProvider>
          <AudienceProvider audience="owner">
            <SmartFolderCardsProvider>
              <table>
                <tbody>
                  <SmartFolderRow folder={{ id: 1, name: "猫" }} backTo="/" owner />
                  <SmartFolderRow
                    folder={{ id: 2, name: "犬" }}
                    backTo="/"
                    owner={false}
                  />
                </tbody>
              </table>
            </SmartFolderCardsProvider>
          </AudienceProvider>
        </TooltipProvider>
      </MemoryRouter>,
    );
    await revealCards();

    const link = await screen.findByRole("link", {
      name: "猫, smart folder of 0 videos",
    });
    expect(link.className).toContain("text-muted-foreground");
    expect(link.closest("tr")?.querySelector("img")?.getAttribute("src")).toBe(
      "/api/videos/10/thumbnail",
    );
    await waitFor(() => expect(screen.queryByRole("link", { name: /^犬/ })).toBeNull());
  });

  describe("行の印", () => {
    function renderMarked(audience: Audience = "owner", favorite = false) {
      render(
        <MemoryRouter>
          <TooltipProvider>
            <ToastProvider>
              <AudienceProvider audience={audience}>
                <SmartFolderCardsProvider>
                  <SmartFolderCard folder={{ id: 3, name: "鳥", favorite }} backTo="/" />
                  <table>
                    <tbody>
                      <SmartFolderRow
                        folder={{ id: 1, name: "猫", favorite }}
                        backTo="/"
                        owner={audience === "owner"}
                      />
                      <SmartFolderRow
                        folder={{ id: 2, name: "犬", favorite }}
                        backTo="/"
                        owner={audience === "owner"}
                      />
                    </tbody>
                  </table>
                </SmartFolderCardsProvider>
              </AudienceProvider>
            </ToastProvider>
          </TooltipProvider>
        </MemoryRouter>,
      );
    }

    function answerMarks() {
      server.writeAnswer = (method) =>
        method === "PUT"
          ? json({ appliedVideos: 0, appliedGroups: 0, appliedSmartFolders: 1 })
          : json({ added: 1, existing: 0 });
      const original = globalThis.fetch;
      vi.stubGlobal(
        "fetch",
        vi.fn<typeof fetch>((input, init) =>
          String(input).startsWith("/api/playlists?")
            ? Promise.resolve(
                json({
                  items: [
                    {
                      id: 5,
                      name: "Tonight",
                      itemCount: 0,
                      updatedAt: "2026-10-01T00:00:00Z",
                    },
                  ],
                  total: 1,
                  version: "1",
                }),
              )
            : original(input, init),
        ),
      );
    }

    function json(body: unknown): Response {
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }

    it("格子のカードには印を出さず、行に出す", () => {
      renderMarked("owner", true);
      expect(
        screen.queryByRole("button", { name: 'Favorite smart folder "鳥"' }),
      ).toBeNull();
      expect(
        screen.queryByRole("button", { name: 'Add smart folder "鳥" to a playlist' }),
      ).toBeNull();
      expect(
        screen.getByRole("button", { name: 'Favorite smart folder "猫"' }),
      ).toBeDefined();
    });

    it("ハートはフォルダの id を smartFolderIds で送り、成功したら塗りを変える", async () => {
      answerMarks();
      const user = userEvent.setup();
      renderMarked();
      const heart = screen.getByRole("button", { name: 'Favorite smart folder "猫"' });
      expect(heart.getAttribute("aria-pressed")).toBe("false");
      await user.click(heart);
      await waitFor(() =>
        expect(
          screen
            .getByRole("button", { name: 'Favorite smart folder "猫"' })
            .getAttribute("aria-pressed"),
        ).toBe("true"),
      );
      expect(server.writes).toEqual([
        {
          method: "PUT",
          path: "/api/favorites",
          body: { videoIds: [], groupIds: [], smartFolderIds: [1], favorite: true },
        },
      ]);
      await user.click(
        screen.getByRole("button", { name: 'Favorite smart folder "犬"' }),
      );
      await waitFor(() => expect(server.writes).toHaveLength(2));
      expect(server.writes[1]?.body).toEqual({
        videoIds: [],
        groupIds: [],
        smartFolderIds: [2],
        favorite: true,
      });
    });

    it("お気に入りのフォルダは塗りで始まり、読んだカードの値に従う", async () => {
      server.cards.set(1, folderCard(1, "猫", { favorite: false }));
      renderMarked("owner", true);
      expect(
        screen
          .getByRole("button", { name: 'Favorite smart folder "猫"' })
          .getAttribute("aria-pressed"),
      ).toBe("true");
      await revealCards([1]);
      await waitFor(() =>
        expect(
          screen
            .getByRole("button", { name: 'Favorite smart folder "猫"' })
            .getAttribute("aria-pressed"),
        ).toBe("false"),
      );
    });

    it("押した後にカードの値が読み直されたら、押した塗りを捨てて読んだ値に従う", async () => {
      answerMarks();
      server.cards.set(1, folderCard(1, "猫", { favorite: false }));
      const user = userEvent.setup();
      renderMarked();
      await revealCards([1]);
      await screen.findByRole("link", { name: /^猫, smart folder of/ });
      const pressed = () =>
        screen
          .getByRole("button", { name: 'Favorite smart folder "猫"' })
          .getAttribute("aria-pressed");
      await user.click(
        screen.getByRole("button", { name: 'Favorite smart folder "猫"' }),
      );
      await waitFor(() => expect(pressed()).toBe("true"));

      server.cards.set(1, folderCard(1, "猫", { favorite: true }));
      await emitServerEvent("smartFolder", { ids: [1] });
      await waitFor(() => expect(server.cardRequests).toHaveLength(2));
      expect(pressed()).toBe("true");

      // A new read restores the original server value; discard the old optimistic override even when values match.
      server.cards.set(1, folderCard(1, "猫", { favorite: false }));
      await emitServerEvent("smartFolder", { ids: [1] });
      await waitFor(() => expect(server.cardRequests).toHaveLength(3));
      await waitFor(() => expect(pressed()).toBe("false"));

      await user.click(
        screen.getByRole("button", { name: 'Favorite smart folder "猫"' }),
      );
      await waitFor(() => expect(server.writes).toHaveLength(2));
      expect(server.writes[1]?.body).toEqual({
        videoIds: [],
        groupIds: [],
        smartFolderIds: [1],
        favorite: true,
      });
    });

    it("「Add to playlist」はフォルダを 1 つの項目として smartFolderIds で送る", async () => {
      answerMarks();
      const user = userEvent.setup();
      renderMarked();
      await user.click(
        screen.getByRole("button", { name: 'Add smart folder "猫" to a playlist' }),
      );
      await user.click(await screen.findByRole("option", { name: /Tonight/ }));
      await waitFor(() =>
        expect(server.writes).toEqual([
          {
            method: "POST",
            path: "/api/playlists/5/items",
            body: { videoIds: [], groupIds: [], smartFolderIds: [1] },
          },
        ]),
      );
      expect(await screen.findByText('Added 1 item to "Tonight"')).toBeTruthy();
    });

    it("ゲストには印を出さない", () => {
      renderMarked("guest");
      expect(screen.queryByRole("button", { name: /^Favorite smart folder/ })).toBeNull();
      expect(screen.queryByRole("button", { name: /^Add smart folder/ })).toBeNull();
    });
  });
});
