import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { video } from "../../testing/groupPage";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import { ResultCard } from "./ResultCard";

function Player() {
  const { pathname, search } = useLocation();
  return <p>{`再生画面 ${pathname}${search}`}</p>;
}

describe("ResultCard", () => {
  afterEach(cleanup);

  it("代表より長い版で一致した区間もレーンに出し、押すとその版を区間の始めから開く", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    render(
      <MemoryRouter initialEntries={["/"]}>
        <TooltipProvider>
          <Routes>
            <Route
              path="/"
              element={
                <ResultCard
                  item={{
                    video: video(5, { title: "動画 5", durationMs: 60_000 }),
                    match: {
                      videoId: 7,
                      rate: 1,
                      kind: "partial",
                      positionMs: 70_000,
                      startMs: 70_000,
                      endMs: 80_000,
                    },
                  }}
                  backTo="/"
                  entry="entry"
                  onOpen={onOpen}
                />
              }
            />
            <Route path="/videos/:id" element={<Player />} />
          </Routes>
        </TooltipProvider>
      </MemoryRouter>,
    );

    const lane = screen.getByRole("group", { name: "Matched section" });
    await user.click(within(lane).getByRole("button", { name: "Play from 1:10" }));

    expect(onOpen).toHaveBeenCalledOnce();
    expect(screen.getByText("再生画面 /videos/7?t=70")).toBeDefined();
  });
});
