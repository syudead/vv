import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Density } from "../preferences/viewPreferences";
import DensitySlider, { toDensity, toIndex } from "./DensitySlider";

/**
 * 密度の読み替えの不変条件（FR-013 / data-model.md 3.）。
 *
 * スライダーが持つのは**位置**（0〜2）で、保存されるのは 004 のままの
 * `Density` の文字列である。往復が恒等でないと、密度を変えて再読み込みした
 * だけで別の密度に化ける ── 利用者から見れば設定が保存されていないのと同じで、
 * FR-013（操作の結果は従来と変わらない）に反する。
 *
 * この検査は目で見ても気づきにくい。位置がずれていても画面は正しく描かれ、
 * 化けるのは「変えて → 戻ってくる」往復を踏んだときだけだからである。
 */

/** densities は Density のすべての値である。増えたら型検査で落ちる。 */
const densities: Density[] = ["dense", "standard", "relaxed"];

describe("密度の位置の読み替え（data-model.md 3.）", () => {
  it("3 段階すべてで往復が恒等である", () => {
    for (const density of densities) {
      expect(toDensity(toIndex(density)), `${density} が往復で化ける`).toBe(density);
    }
  });

  it("位置が 3 段階を過不足なく指す", () => {
    // 往復が恒等でも、2 つの密度が同じ位置を指していれば片方は選べない。
    expect(new Set(densities.map(toIndex)).size).toBe(densities.length);
  });

  it("範囲外・非数値の位置は標準に落ちる", () => {
    for (const index of [-1, 3, 1.5, Number.NaN]) {
      expect(toDensity(index), `${String(index)} の落とし先が違う`).toBe("standard");
    }
  });
});

describe("密度切替のキーボード操作", () => {
  it("選択中だけがTab対象で、左右キーで隣へ移る", () => {
    const changes: Density[] = [];
    render(<DensitySlider value="standard" onChange={(value) => changes.push(value)} />);

    const standard = screen.getByRole("button", { name: "標準" });
    const relaxed = screen.getByRole("button", { name: "ゆったり" });
    expect(standard.getAttribute("tabindex")).toBe("0");
    expect(relaxed.getAttribute("tabindex")).toBe("-1");

    standard.focus();
    fireEvent.keyDown(standard, { key: "ArrowRight" });
    expect(changes).toEqual(["relaxed"]);
    expect(document.activeElement).toBe(relaxed);
  });
});
