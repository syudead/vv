import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import {
  renderPage,
  seed,
  server,
  setUpSmartFolderServer,
} from "../testing/smartFolderPage";

// スマートフォルダのページの所有者の「More」の「Add to home」「Remove from home」
// （specs/057-home/ui-design.md「Smart folder page and Settings」）。

setUpSmartFolderServer();

beforeEach(() => {
  __resetTagsForTest();
  __resetTagKindsForTest();
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.releasePointerCapture ??= () => undefined;
  Element.prototype.setPointerCapture ??= () => undefined;
});

async function openMore() {
  const user = userEvent.setup();
  await screen.findByRole("heading", { level: 1, name: "タグ: 猫" });
  await user.click(screen.getByRole("button", { name: "More" }));
  return user;
}

describe("スマートフォルダのページの Add to home と Remove from home", () => {
  it("ホームに無いフォルダは Add to home で最後の棚の後に足し、メニューが Remove from home になる", async () => {
    seed();
    server.homeShelves = [5];
    for (const detail of server.folders.values()) detail.onHome = false;
    server.folders.get(5)!.onHome = true;
    renderPage("/smart-folders/1");

    const user = await openMore();
    await user.click(await screen.findByRole("menuitem", { name: "Add to home" }));

    expect(await screen.findByText('Added "タグ: 猫" to your home.')).toBeTruthy();
    expect(server.writes).toEqual([
      { method: "PUT", path: "/api/home/shelves", body: { folderIds: [5, 1] } },
    ]);
    await waitFor(() => expect(server.reads.get(1)).toBe(2));
    await user.click(screen.getByRole("button", { name: "More" }));
    expect(
      await screen.findByRole("menuitem", { name: "Remove from home" }),
    ).toBeTruthy();
  });

  it("Remove from home は棚から外し、Undo で元の位置に戻す", async () => {
    seed();
    server.homeShelves = [1, 5];
    for (const detail of server.folders.values()) detail.onHome = true;
    renderPage("/smart-folders/1");

    const user = await openMore();
    await user.click(await screen.findByRole("menuitem", { name: "Remove from home" }));

    expect(await screen.findByText('Removed "タグ: 猫" from your home.')).toBeTruthy();
    expect(server.homeShelves).toEqual([5]);
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(server.homeShelves).toEqual([1, 5]));
  });

  it("onHome の無い応答（閲覧者）には出さない", async () => {
    seed();
    renderPage("/smart-folders/1");

    await openMore();

    expect(await screen.findByRole("menuitem", { name: "Edit…" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Add to home" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Remove from home" })).toBeNull();
  });
});
