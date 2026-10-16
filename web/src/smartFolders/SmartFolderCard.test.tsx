import { render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { emitServerEvent } from "../api/fakeEventSource";
import { AudienceProvider } from "../auth/audience";
import {
  folderCard,
  revealCards,
  server,
  setUpSmartFolderServer,
} from "../testing/smartFolderPage";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { SmartFolderCardsProvider } from "./cardValues";
import { SmartFolderCard } from "./SmartFolderCard";

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

describe("スマートフォルダのカード（specs/052-smart-folders/ui-design.md「Smart folder card and row」）", () => {
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
});
