import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import PropertyStrip from "./PropertyStrip";
import { formatCodec, splitPath } from "./properties";

const video: Video = {
  id: 7,
  title: "テスト動画",
  sizeBytes: 84_331_821,
  addedAt: "2026-09-20T03:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  container: "mkv",
  videoCodec: "h264",
  audioCodec: "aac",
};

function pairs(): [string, string][] {
  const list = document.querySelector("dl");
  if (!(list instanceof HTMLElement)) throw new Error("dl がありません");
  const terms = within(list).getAllByRole("term");
  return terms.map((term) => [
    term.textContent ?? "",
    term.nextElementSibling?.textContent ?? "",
  ]);
}

describe("PropertyStrip", () => {
  it("英語のラベルを決まった順に並べ、値を大文字の表記にそろえる", () => {
    render(<PropertyStrip video={video} />);
    const entries = pairs();
    expect(entries.map(([label]) => label)).toEqual([
      "RESOLUTION",
      "CONTAINER",
      "VIDEO",
      "AUDIO",
      "SIZE",
      "ADDED",
      "LAST PLAYED",
    ]);
    expect(Object.fromEntries(entries)).toMatchObject({
      RESOLUTION: "1920×1080",
      CONTAINER: "MKV",
      VIDEO: "H.264",
      AUDIO: "AAC",
      SIZE: "80.4 MB",
      ADDED: "2026/09/20",
      "LAST PLAYED": "—",
    });
    for (const term of screen.getAllByRole("term")) {
      expect(term.getAttribute("lang")).toBe("en");
    }
  });

  it("再生したことがあれば LAST PLAYED に絶対日時を出す", () => {
    render(
      <PropertyStrip
        video={{
          ...video,
          progress: {
            positionMs: 1,
            completed: false,
            updatedAt: "2026-09-21T05:06:00Z",
          },
        }}
      />,
    );
    expect(Object.fromEntries(pairs())["LAST PLAYED"]).toMatch(
      /^2026\/09\/21 \d\d:\d\d$/,
    );
  });

  it("読み取り前の技術情報は「読み取り中」、音声が無ければ —", () => {
    const view = render(
      <PropertyStrip
        video={{
          ...video,
          probeState: "pending",
          width: undefined,
          height: undefined,
          container: undefined,
          videoCodec: undefined,
          audioCodec: undefined,
        }}
      />,
    );
    expect(Object.fromEntries(pairs())).toMatchObject({
      RESOLUTION: "読み取り中",
      CONTAINER: "読み取り中",
      VIDEO: "読み取り中",
      AUDIO: "読み取り中",
    });
    view.unmount();

    render(<PropertyStrip video={{ ...video, audioCodec: undefined }} />);
    expect(Object.fromEntries(pairs()).AUDIO).toBe("—");
  });

  it("読み取りに失敗した動画では、技術情報を MEDIA の 1 項目にまとめる", () => {
    render(
      <PropertyStrip
        video={{ ...video, probeState: "failed", probeError: "moov atom" }}
      />,
    );
    expect(pairs()).toEqual([
      ["MEDIA", "読み取れませんでした"],
      ["SIZE", "80.4 MB"],
      ["ADDED", "2026/09/20"],
      ["LAST PLAYED", "—"],
    ]);
    expect(screen.getAllByText("読み取れませんでした")).toHaveLength(1);
  });

  it("廃止した項目（ID・解析状態・ブラウザ再生・バイト数・画質・再生位置）を出さない", () => {
    render(
      <PropertyStrip
        video={{
          ...video,
          progress: {
            positionMs: 60_000,
            completed: false,
            updatedAt: "2026-09-21T05:06:00Z",
          },
        }}
      />,
    );
    const labels = pairs().map(([label]) => label);
    expect(labels).not.toContain("ID");
    const text = document.body.textContent ?? "";
    for (const removed of [
      "解析",
      "ブラウザ再生",
      "バイト",
      "1080p",
      "再生位置",
      "1:00",
    ]) {
      expect(text).not.toContain(removed);
    }
  });
});

describe("property helpers", () => {
  it("コーデックを大文字の表記にする", () => {
    expect(formatCodec("h264")).toBe("H.264");
    expect(formatCodec("hevc")).toBe("H.265");
    expect(formatCodec("vp9")).toBe("VP9");
    expect(formatCodec("aac")).toBe("AAC");
  });

  it("パスをフォルダとファイル名に分ける", () => {
    expect(splitPath("/media/a/b.mp4")).toEqual({ folder: "/media/a/", name: "b.mp4" });
    expect(splitPath("C:\\v\\x.mkv")).toEqual({ folder: "C:\\v\\", name: "x.mkv" });
  });
});
