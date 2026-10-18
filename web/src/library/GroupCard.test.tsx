import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { expect, it, vi } from "vitest";

import { AudienceProvider } from "../auth/audience";
import { group } from "../testing/groupPage";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { GroupCard } from "./GroupCard";

function Location() {
  const location = useLocation();
  return (
    <output>
      {location.pathname + location.search} {JSON.stringify(location.state)}
    </output>
  );
}

it("opens the collection at rest and on More, and plays only the mouse-promoted video", () => {
  const collection = group(7, "Trip", {
    videoCount: 5,
    previews: [1, 2, 3, 4].map((videoId) => ({
      videoId,
      thumbnailUrl: `/thumb/${String(videoId)}`,
    })),
  });
  const onSelect = vi.fn();
  const card = (selectionMode = false) => (
    <MemoryRouter>
      <AudienceProvider audience="guest">
        <TooltipProvider>
          <GroupCard
            group={collection}
            backTo="/?q=trip"
            to="/groups/7?trail=3"
            selected={false}
            selectionMode={selectionMode}
            onSelect={onSelect}
          />
          <Location />
        </TooltipProvider>
      </AudienceProvider>
    </MemoryRouter>
  );
  const { rerender } = render(card());
  const link = screen.getByRole("link", { name: /Trip, group/ });
  const art = link.querySelector<HTMLElement>("[data-folder-art]")!;
  art.getBoundingClientRect = () => new DOMRect(0, 0, 200, 100);
  const hover = (x: number) =>
    fireEvent.pointerMove(art, { pointerType: "mouse", clientX: x });
  expect(link.getAttribute("href")).toBe("/groups/7?trail=3");
  hover(10);
  expect(link.getAttribute("href")).toBe("/videos/1?group=7");
  hover(190);
  expect(art.querySelector("[data-folder-more][data-folder-front]")).not.toBeNull();
  expect(link.getAttribute("href")).toBe("/groups/7?trail=3");
  hover(10);
  fireEvent.pointerLeave(art);
  expect(link.getAttribute("href")).toBe("/groups/7?trail=3");
  hover(10);
  fireEvent.pointerDown(link, { pointerType: "touch" });
  expect(link.getAttribute("href")).toBe("/groups/7?trail=3");
  fireEvent.pointerLeave(art);
  hover(90);
  fireEvent.keyDown(link, { key: "Enter" });
  expect(link.getAttribute("href")).toBe("/groups/7?trail=3");
  hover(90);
  expect(link.getAttribute("href")).toBe("/videos/3?group=7");

  rerender(card(true));
  hover(10);
  fireEvent.click(link);
  expect(onSelect).toHaveBeenCalledWith(collection, true);
  expect(screen.getByRole("status").textContent).toBe("/ null");
  rerender(card());
  hover(50);
  fireEvent.click(link);
  expect(screen.getByRole("status").textContent).toBe(
    '/videos/2?group=7 {"from":"/?q=trip","autoplay":true}',
  );
});
