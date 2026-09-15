import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import MetaList from "./MetaList";

/**
 * C16 MetaList の不変条件（data-model.md 4.「不変条件」/ FR-016 /
 * contracts/components.md 5.）。
 *
 * 確かめるのは 3 つだけである。**6 項目の順序**（減らす・増やす・並べ替えるのは
 * spec のスコープ外）、**ラベルと値が `<dt>` / `<dd>` の対で読み上げに渡ること**
 * （見た目のために `<div>` の並びへ崩すと 004 が得ていたものを失う）、そして
 * **値に等幅フォントが無いこと**（FR-016 が名指しで禁じている）である。
 *
 * 色・余白・区切り線は確かめない。jsdom は CSS を解決しないので、確かめられる
 * のはクラス名が付いているという事実だけで、それは見た目の保証にならない
 * （R-511）。
 */

const done: Video = {
  id: 7,
  title: "長い動画",
  sizeBytes: 1024,
  addedAt: "2026-09-13T00:00:00Z",
  durationMs: 600_000,
  width: 1920,
  height: 1080,
  container: "mp4",
  videoCodec: "h264",
  audioCodec: "aac",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
};

/** labels は描かれた項目名を上から順に返す。 */
function labels(): string[] {
  return [...document.querySelectorAll("dt")].map((node) => node.textContent ?? "");
}

/** values は描かれた値を上から順に返す。 */
function values(): HTMLElement[] {
  return [...document.querySelectorAll("dd")];
}

describe("MetaList（C16 / data-model.md 4.）", () => {
  it("6 項目が data-model.md 4. の順序で並ぶ", () => {
    render(<MetaList video={done} />);

    expect(labels()).toEqual(["長さ", "解像度", "形式", "映像", "音声", "大きさ"]);
  });

  it("ラベルと値が dt / dd の対で出る", () => {
    render(<MetaList video={done} />);

    // 対なので数が揃う。片方だけ増減すると読み上げで対応が崩れる。
    const texts = values().map((node) => node.textContent ?? "");
    expect(texts).toHaveLength(labels().length);
    expect(texts).toEqual(["10:00", "1920 × 1080", "mp4", "h264", "aac", "1.0 KB"]);

    // dt と dd は dl の下にある（div で包んでも祖先には dl がいる）。
    for (const node of [...document.querySelectorAll("dt,dd")]) {
      expect(node.closest("dl")).not.toBeNull();
    }
  });

  it("値に等幅フォントの指定が無い（FR-016）", () => {
    // 解析待ち・失敗の言い分けも同じ体裁で出るので、3 つとも見る。
    for (const video of [
      done,
      { ...done, probeState: "pending" as const, durationMs: undefined },
      { ...done, probeState: "failed" as const, durationMs: undefined },
    ]) {
      document.body.innerHTML = "";
      render(<MetaList video={video} />);

      for (const node of values()) {
        expect(node.className).not.toContain("font-mono");
      }
    }
  });
});
