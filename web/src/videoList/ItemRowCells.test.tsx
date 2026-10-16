import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import { Table, TableBody, TableRow } from "../ui/shadcn/table";
import {
  ItemThumbnailCell,
  type OrderedItem,
  orderedItemDurationMs,
  orderedItemTitle,
  rowFacts,
} from "./ItemRowCells";

// 順に並ぶ項目の行の共通の値（specs/052-smart-folders/ui-design.md「Playlist screen row」）。
// 画面の行はプレイリストとグループのページのテストが確かめ、ここでは項目の種類ごとの値を見る。

const folder: OrderedItem = {
  kind: "smartFolder",
  smartFolder: {
    id: 9,
    name: "タグ: 猫",
    videoCount: 0,
    sizeBytes: 0,
    previews: [],
    tags: [],
  },
};

describe("ItemRowCells", () => {
  it("スマートフォルダの項目はグループの項目と同じ値で、印と空の印だけが違う", () => {
    expect(rowFacts(folder)).toMatchObject({
      groupCount: "0 videos",
      smartFolder: true,
      empty: true,
      ratio: null,
    });
    expect(orderedItemTitle(folder)).toBe("タグ: 猫");
    expect(orderedItemDurationMs(folder)).toBeUndefined();
  });

  it("中身の無い項目（契約上は起きない）は名前も長さも空にする", () => {
    expect(orderedItemTitle({ kind: "video" })).toBe("");
    expect(orderedItemTitle({ kind: "smartFolder" })).toBe("");
    expect(orderedItemDurationMs({ kind: "group" })).toBeUndefined();
    expect(rowFacts({ kind: "smartFolder" })).toMatchObject({ groupCount: null });
  });

  it("見ている途中の動画の進み具合は動画の読み上げ名で出す", () => {
    const video = {
      id: 1,
      title: "a",
      durationMs: 60_000,
      progress: {
        positionMs: 30_000,
        completed: false,
        updatedAt: "2026-09-01T00:00:00Z",
      },
    } as Video;
    const item: OrderedItem = { kind: "video", video };
    render(
      <Table>
        <TableBody>
          <TableRow>
            <ItemThumbnailCell item={item} facts={rowFacts(item)} />
          </TableRow>
        </TableBody>
      </Table>,
    );
    expect(screen.getByRole("progressbar", { name: "Watched portion" })).toBeDefined();
  });
});
