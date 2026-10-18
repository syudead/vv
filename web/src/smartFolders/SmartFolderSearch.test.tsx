import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import * as api from "../api/smartFolders";
import {
  renderPage,
  seed,
  server,
  setUpSmartFolderServer,
} from "../testing/smartFolderPage";

setUpSmartFolderServer();

it("URLの検索が0件でも検索欄と元の条件を残し、クリアすると元の範囲へ戻る", async () => {
  const scroll = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
  seed();
  const original = api.readSmartFolderPage;
  const read = vi
    .spyOn(api, "readSmartFolderPage")
    .mockImplementation(async (id, signal, limit, query) => {
      const detail = await original(id, signal, limit, query);
      return query ? { ...detail, items: [], nextCursor: undefined } : detail;
    });
  const user = userEvent.setup();
  renderPage("/smart-folders/1?q=missing");
  expect(await screen.findByText("No videos match these conditions")).toBeTruthy();
  const search = screen.getByRole<HTMLInputElement>("searchbox", {
    name: "Search videos",
  });
  expect(search.value).toBe("missing");
  expect(screen.getByRole("button", { name: "How to search" })).toBeTruthy();
  expect(screen.getByRole("link", { name: 'Play "タグ: 猫"' })).toBeTruthy();
  expect(screen.getByText("Tag: 猫")).toBeTruthy();
  expect(read).toHaveBeenLastCalledWith(1, expect.any(AbortSignal), undefined, "missing");
  await user.click(screen.getByRole("button", { name: "Clear search" }));
  expect(await screen.findByRole("heading", { name: "Cat 1" })).toBeTruthy();
  expect(search.value).toBe("");
  expect(document.activeElement).toBe(search);
  expect(read).toHaveBeenLastCalledWith(1, expect.any(AbortSignal), undefined, "");
  expect(server.writes).toEqual([]);
  expect(scroll).toHaveBeenCalled();
});
