import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ProbeErrorCode, Video } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ReadFailure } from "./StatusOverlays";

const failed: Video = {
  id: 7,
  title: "壊れた動画",
  public: false,
  sizeBytes: 100,
  addedAt: "2026-09-01T00:00:00Z",
  playable: false,
  probeState: "failed",
  thumbnailState: "failed",
  previewState: "failed",
  location: { path: "/media/壊れた動画.mp4", openable: true },
  tags: [],
};

const reprobe = () => Promise.resolve();

describe("ReadFailure", () => {
  const explanations: [ProbeErrorCode, string][] = [
    ["file_unavailable", "The video file couldn't be read."],
    ["probe_unavailable", "ffprobe couldn't be started, so the video couldn't be read."],
    ["probe_failed", "The file is damaged or isn't a supported video."],
    ["invalid_metadata", "The video's information couldn't be understood."],
    ["internal", "Something went wrong while reading the video."],
  ];

  it.each(explanations)(
    "probeErrorCode %s は英語の説明を出し、自由文の probeError は出さない",
    (code, text) => {
      render(
        <ReadFailure
          video={{
            ...failed,
            probeErrorCode: code,
            probeError: "ffprobe: moov atom not found (exit status 1)",
          }}
          onReprobe={reprobe}
        />,
      );
      const alert = screen.getByRole("alert");
      expect(within(alert).getByText(text)).toBeDefined();
      expect(alert.textContent).not.toContain("moov atom");
    },
  );

  it("コードの無い過去の失敗は、日本語の probeError を出さずに英語の概要を出す", () => {
    render(
      <ReadFailure
        video={{ ...failed, probeError: "ffprobe の実行に失敗しました: 壊れています" }}
        onReprobe={reprobe}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(
      within(alert).getByText("Couldn't read this video's information."),
    ).toBeDefined();
    expect(alert.textContent).not.toMatch(/[\u3040-\u30ff\u4e00-\u9fff]/);
    expect(within(alert).getByRole("button", { name: "Read again" })).toBeDefined();
  });

  it("ゲストの画面（操作なし）でも、コードの無い失敗は英語の概要を出す", () => {
    render(<ReadFailure video={{ ...failed, location: undefined }} />);
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toBe(
      "Couldn't read this videoCouldn't read this video's information.",
    );
  });

  it("疑似ロケールで、説明と操作がカタログから出る", () => {
    enablePseudoLocale();
    render(
      <ReadFailure
        video={{ ...failed, probeErrorCode: "probe_failed", probeError: "x" }}
        onReprobe={reprobe}
      />,
    );
    expectCatalogTextOnly(document.body);
  });
});
